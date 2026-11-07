package zmodem

import (
	"bytes"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"time"

	"oshell-desktop/internal/zmodem/log"
)

const (
	// xprzmodem.library (Term 4.8, NComm) hard-codes a 1024-byte zrdata
	// buffer. Larger subpackets cause it to abort with "Data packet too long".
	subpacketSize = 1024

	// Eight 1024-byte subpackets per ACK = 8 KB between real round-trips.
	// Short enough that slow serial clients with small FIFO buffers don't
	// overflow while waiting for the wire to catch up.
	subpacketsPerAck = 8

	activityTimeout    = 60 * time.Second
	canCancelThreshold = 5
)

// cancelEcho is what the sender writes on the wire when the receiver has
// cancelled the session: 8× CAN followed by 10× backspace. Matches lrzsz.
var cancelEcho = []byte{
	0x18, 0x18, 0x18, 0x18, 0x18, 0x18, 0x18, 0x18,
	0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08,
}

// ErrCancelled is returned when the receiver aborts via ≥5 consecutive CANs.
var ErrCancelled = errors.New("ZMODEM transfer cancelled by receiver")

// ErrSkipped means an individual file was declined, not that the batch failed.
var ErrSkipped = errors.New("ZMODEM transfer skipped by receiver")

var ErrRemoteFile = errors.New("ZMODEM receiver reported a file error")
var ErrAborted = fmt.Errorf("ZMODEM receiver aborted batch: %w", ErrCancelled)

func peerRejection(frame byte) error {
	switch frame {
	case FrameZSKIP:
		return ErrSkipped
	case FrameZABORT:
		return ErrAborted
	default:
		return ErrRemoteFile
	}
}

func isPeerRejection(err error) bool {
	return errors.Is(err, ErrSkipped) || errors.Is(err, ErrAborted) || errors.Is(err, ErrRemoteFile)
}

// rzTrigger is the preamble terminals like NComm / Term 4.8 require before
// recognizing a ZMODEM session. Modern receivers (lrzsz) tolerate it harmlessly.
var rzTrigger = []byte("rz\r")

// nowFn is swapped by tests to pin timestamps for deterministic ZFILE mtime.
var nowFn = func() time.Time { return time.Now() }

// SetNow replaces the clock used to stamp ZFILE mtime. Returns a restore func.
// Tests-only hook; no production caller should touch this.
func SetNow(f func() time.Time) func() {
	prev := nowFn
	nowFn = f
	return func() { nowFn = prev }
}

// SendBuffer transfers `data` to the ZMODEM receiver on the other end of conn,
// advertising `filename` in the ZFILE frame. Honors the receiver's ZRPOS
// resume offset and emits subpackets paced at 8 KB per ACK.
//
// The function takes full ownership of conn for the duration of the transfer:
// reads and writes must be routed through the returned error only. The caller
// is responsible for restoring any pre-attached read handlers afterwards.
func SendBuffer(conn net.Conn, data []byte, filename string) error {

	s := newSender(conn, data, filename)
	err := s.run()
	if err == nil || errors.Is(err, ErrCancelled) || errors.Is(err, ErrSkipped) {
		return err
	}
	// Strip Go's "io:" prefix on closed-pipe errors; surface cleanly to the
	// user-visible log line.
	logger.Error(fmt.Sprintf("ZMODEM transfer failed: %s", strings.TrimPrefix(err.Error(), "io: ")))
	return err
}

// --- Internals --------------------------------------------------------------

type sender struct {
	codec      codec
	source     io.ReaderAt
	size       int64
	onProgress func(int64, int64)

	conn     net.Conn
	data     []byte
	filename string

	reader *reader

	// Receiver-advertised byte window; zero means no explicit limit.
	receiveWindow uint32

	startedAt time.Time
}

func newSender(conn net.Conn, data []byte, filename string) *sender {
	s := &sender{conn: conn, data: data, source: bytes.NewReader(data), size: int64(len(data)), filename: filename, startedAt: nowFn()}
	s.reader = newReader(conn)
	return s
}

func (s *sender) run() error {
	defer s.reader.close()

	logger.Info(fmt.Sprintf("Starting ZMODEM transfer: %s (%d bytes)", s.filename, int(s.size)))

	// Preamble + ZRQINIT.
	if _, err := s.conn.Write(rzTrigger); err != nil {
		return err
	}
	if _, err := s.conn.Write(BuildZhexHeader(FrameZRQINIT, 0)); err != nil {
		return err
	}

	// Negotiate: wait for ZRINIT from the receiver.
	if err := s.waitForZrinit(); err != nil {
		return err
	}

	// Term 4.8 (Amiga xprzmodem.library) sends ZRINIT as soon as its
	// auto-download kicks in, but the receive window is still animating
	// open — firing ZFILE immediately causes the first bytes to be eaten,
	// producing a visible burst of CRC errors before the transfer recovers
	// on retransmit. A brief pause lets the window finish initializing.
	time.Sleep(250 * time.Millisecond)

	if err := s.sendCurrentFile(true); err != nil {
		return err
	}

	// ZFIN handshake: send ZFIN, wait for receiver's ZFIN, write the "OO"
	// (Over and Out) terminator that closes the session.
	if _, err := s.conn.Write(BuildZhexHeader(FrameZFIN, 0)); err != nil {
		return err
	}
	if err := s.waitForZfin(); err != nil {
		return err
	}
	if _, err := s.conn.Write([]byte{'O', 'O'}); err != nil {
		return err
	}

	logger.Info(fmt.Sprintf("ZMODEM transfer completed in %.1fs", time.Since(s.startedAt).Seconds()))
	return nil
}

// waitForZrinit accepts the same checked header encodings as the receiver.
// Negotiate only a complete CRC-valid ZRINIT, including its receive window.
func (s *sender) waitForZrinit() error {
	_, count, err := s.awaitOneOfWithCount(FrameZRINIT)
	if err == nil {
		s.codec.allControls = s.codec.allControls || byte(count>>24)&EscCtl != 0
		s.receiveWindow = count & 0xffff
	}
	return err
}

func (s *sender) waitForZfin() error {
	_, _, err := s.awaitOneOfWithCount(FrameZFIN)
	return err
}

// waitForZrpos waits for a ZRPOS header (can be ZHEX or ZBIN), parses the
// 4-byte offset, and returns it. If the receiver instead sends ZSKIP or
// ZABORT (the two "I don't want this file" signals Term 4.8 issues from
// its Skip file / Skip batch buttons), returns ErrSkipped. ZFERR is a
// fatal file error on the receiver side — surface it the same way.
//
// Also handles the ZFILE-got-garbled recovery path: if Term 4.8's receive
// window eats the first ZFILE, the Amiga side either NAKs the header or
// just keeps re-emitting its ZRINIT until we give it a fresh ZFILE. Both
// signals cause us to resend the ZFILE (capped) so the handshake recovers
// in a round-trip instead of hanging until the receiver's own retry
// cycle happens to produce something we'd otherwise listen for.
func (s *sender) waitForZrpos() (uint32, error) {
	const maxZfileResends = 8
	resends := 0
	for {
		frame, count, err := s.awaitOneOfWithCount(
			FrameZRPOS, FrameZSKIP, FrameZABORT, FrameZFERR,
			FrameZNAK, FrameZRINIT, FrameZCRC,
		)
		if err != nil {
			return 0, err
		}
		switch frame {
		case FrameZRPOS:
			return count, nil
		case FrameZSKIP:
			logger.Info("ZMODEM receiver sent ZSKIP — skipping file")
			return 0, ErrSkipped
		case FrameZABORT:
			logger.Info("ZMODEM receiver sent ZABORT — ending session")
			return 0, ErrAborted
		case FrameZFERR:
			logger.Info("ZMODEM receiver sent ZFERR — aborting transfer")
			return 0, ErrRemoteFile
		case FrameZCRC:
			// lrzsz verifies the existing prefix before allowing resume.
			length := s.size
			if count != 0 && int64(count) < length {
				length = int64(count)
			}
			hash := crc32.NewIEEE()
			n, err := io.Copy(hash, io.NewSectionReader(s.source, 0, length))
			if err != nil {
				return 0, err
			}
			if n != length {
				return 0, io.ErrUnexpectedEOF
			}
			if _, err := s.conn.Write(s.codec.header16(FrameZCRC, hash.Sum32())); err != nil {
				return 0, err
			}
		case FrameZNAK, FrameZRINIT:
			if frame == FrameZRINIT {
				// Like lrzsz zsendfile: a delayed init can precede a valid
				// ZRPOS. Allow that next header to arrive before resending.
				pending, err := s.nextHeaderPending()
				if err != nil {
					return 0, err
				}
				if pending {
					continue
				}
			}
			if resends >= maxZfileResends {
				return 0, fmt.Errorf("ZMODEM ZFILE resend limit exceeded")
			}
			resends++
			logger.Info(fmt.Sprintf("ZMODEM receiver asked to restart handshake (frame %d) — resending ZFILE (attempt %d)", frame, resends+1))
			if _, err := s.conn.Write(s.buildZfile()); err != nil {
				return 0, err
			}
		}
	}
}

func (s *sender) nextHeaderPending() (bool, error) {
	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return false, nil
		}
		b, err := s.reader.readByte(remaining)
		if errors.Is(err, errActivityTimeout) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if b == ZPAD {
			s.reader.unread([]byte{b})
			return true, nil
		}
	}
}

// gracefulClose emits a best-effort ZFIN + OO so the receiver sees a clean
// session terminator. Used when the receiver has signalled ZSKIP/ZABORT —
// the transfer is over, but we still owe the peer a tidy goodbye so it
// doesn't sit in its own cleanup timeout. Any write error is swallowed;
// we're already on the failure path.
func (s *sender) gracefulClose() {
	_, _ = s.conn.Write(BuildZhexHeader(FrameZFIN, 0))
	_, _ = s.conn.Write([]byte{'O', 'O'})
}

// buildZfile emits the ZBIN16 ZFILE header + lrzsz-format fileinfo subpacket.
// CRC16 (not CRC32) is load-bearing: Term 4.8 advertises CANFC32 but hangs
// when it actually receives a CRC32 ZFILE. See amigalove.com writeup.
func (s *sender) buildZfile() []byte {
	// Header: ZBIN ZFILE with ZF3..ZF0 = 00 00 00 01 (ZCBIN binary).
	out := []byte{ZPAD, ZDLE, ZBIN}
	header := []byte{FrameZFILE, 0x00, 0x00, 0x00, 0x01}
	out = s.codec.escape(out, header)
	crc := CRC16(header)
	out = s.codec.escape(out, []byte{byte(crc >> 8), byte(crc)})

	// Subpacket: "<name>\0<size> <mtime_octal> 100644 0 1 <size>\0"
	info := make([]byte, 0, 128+len(s.filename))
	info = append(info, []byte(s.filename)...)
	info = append(info, 0)
	meta := fmt.Sprintf("%d %s 100644 0 1 %d",
		int(s.size),
		strconv.FormatInt(nowFn().Unix(), 8),
		int(s.size))
	info = append(info, []byte(meta)...)
	info = append(info, 0)
	out = s.codec.escape(out, info)

	// ZDLE + ZCRCW + CRC16 of (info + ZCRCW).
	out = append(out, ZDLE, ZCRCW)
	crcInput := make([]byte, len(info)+1)
	copy(crcInput, info)
	crcInput[len(info)] = ZCRCW
	subCrc := CRC16(crcInput)
	out = s.codec.escape(out, []byte{byte(subCrc >> 8), byte(subCrc)})
	return out
}

// streamData emits ZDATA bursts from `startOffset` to end-of-file, flushing
// every `subpacketsPerAck` subpackets with a ZCRCW that waits for a ZACK.
// On ZRPOS (receiver-requested retransmit) or ZNAK (bad header), the stream
// rewinds and resends from the requested offset.
func (s *sender) streamData(startOffset uint32) error {
	if int64(startOffset) > s.size {
		return fmt.Errorf("ZMODEM: resume offset %d exceeds file size %d", startOffset, s.size)
	}
	offset := startOffset
	burstLimit := uint32(subpacketSize * subpacketsPerAck)
	if s.receiveWindow != 0 && s.receiveWindow < burstLimit {
		burstLimit = s.receiveWindow
	}
	for int(offset) < int(s.size) {
		burstStart := offset
		// Assemble one ZDATA burst (up to subpacketsPerAck subpackets or EOF).
		frame := s.codec.header16(FrameZDATA, burstStart)

		for i := 0; i < subpacketsPerAck; i++ {
			remaining := int(s.size) - int(offset)
			if remaining <= 0 {
				break
			}
			size := min(remaining, subpacketSize, int(burstLimit-(offset-burstStart)))
			chunk := make([]byte, size)
			n, e := s.source.ReadAt(chunk, int64(offset))
			if n != size {
				return io.ErrUnexpectedEOF
			}
			if e != nil && e != io.EOF {
				return e
			}

			kind := byte(ZCRCG)
			last := int(offset)+len(chunk) == int(s.size) || i == subpacketsPerAck-1 || offset-burstStart+uint32(len(chunk)) == burstLimit
			if last {
				kind = ZCRCW
			}
			frame = s.codec.escape(frame, chunk)
			frame = append(frame, ZDLE, kind)
			crcInput := make([]byte, len(chunk)+1)
			copy(crcInput, chunk)
			crcInput[len(chunk)] = kind
			crc := CRC16(crcInput)
			frame = s.codec.escape(frame, []byte{byte(crc >> 8), byte(crc)})
			offset += uint32(len(chunk))
			if last {
				break
			}
		}
		if _, err := s.conn.Write(frame); err != nil {
			return err
		}
		// A ZCRCW terminator always requests a response from the receiver:
		// ZACK on success, ZRPOS(offset) when a subpacket CRC failed, or
		// ZNAK when the ZDATA header itself was garbled. Waiting also keeps
		// old receivers (Term 4.8 / NComm) in step — they NAK a premature
		// ZEOF if the last subpacket isn't acknowledged first.
		var resp burstResponse
		for {
			var err error
			resp, err = s.awaitBurstResponse()
			if err != nil {
				return err
			}
			// A delayed ACK for an earlier burst cannot acknowledge this one.
			if resp.frame != FrameZACK || resp.offset >= offset {
				break
			}
		}
		switch resp.frame {
		case FrameZACK:
			if resp.offset != offset {
				return fmt.Errorf("ZMODEM: invalid acknowledgement offset %d (want %d)", resp.offset, offset)
			}
			if s.onProgress != nil {
				s.onProgress(int64(offset), s.size)
			}
		case FrameZRPOS:
			if int64(resp.offset) > s.size {
				return fmt.Errorf("ZMODEM: resume offset %d exceeds file size %d", resp.offset, s.size)
			}
			// Receiver detected a CRC error and wants us to rewind. Flush
			// any stale ZRPOS retries it may have already queued so the
			// next awaitBurstResponse sees a fresh response to our retry.
			logger.Info(fmt.Sprintf("ZMODEM receiver requested retransmit from offset %d", resp.offset))
			s.reader.drain()
			offset = resp.offset
		case FrameZNAK:
			// Header was unreadable; receiver wants the same burst again.
			logger.Info("ZMODEM receiver NAK — resending burst")
			s.reader.drain()
			offset = burstStart
		case FrameZSKIP:
			logger.Info("ZMODEM receiver sent ZSKIP — skipping file")
			return ErrSkipped
		case FrameZABORT:
			logger.Info("ZMODEM receiver sent ZABORT — ending session")
			return ErrAborted
		case FrameZFERR:
			logger.Info("ZMODEM receiver sent ZFERR — aborting transfer")
			return ErrRemoteFile
		}
	}
	return nil
}

// burstResponse is a receiver reply to a ZCRCW-terminated data burst.
type burstResponse struct {
	frame  byte
	offset uint32
}

// awaitBurstResponse blocks until the receiver sends a ZACK, ZRPOS, ZNAK,
// ZSKIP, ZABORT, or ZFERR header. Accepts both ZHEX and ZBIN encodings.
func (s *sender) awaitBurstResponse() (burstResponse, error) {
	frame, count, err := s.awaitOneOfWithCount(
		FrameZACK, FrameZRPOS, FrameZNAK, FrameZSKIP, FrameZABORT, FrameZFERR,
	)
	if err != nil {
		return burstResponse{}, err
	}
	return burstResponse{frame: frame, offset: count}, nil
}

// awaitOneOfWithCount shares the receiver's CRC-checked header decoder.
// This handles flow control inside headers, legal padding, binary CRC16/32,
// and ZRUB0/ZRUB1 escapes identically in both transfer directions.
func (s *sender) awaitOneOfWithCount(wants ...byte) (byte, uint32, error) {
	r := &receiver{conn: s.conn, reader: s.reader}
	for {
		frame, count, err := r.readHeader()
		if err != nil {
			return 0, 0, err
		}
		for _, want := range wants {
			if frame == want {
				return frame, count, nil
			}
		}
		if frame == FrameZABORT || frame == FrameZFERR || frame == FrameZSKIP {
			return 0, 0, peerRejection(frame)
		}
	}
}

// --- reader ----------------------------------------------------------------

// reader wraps a net.Conn with:
//   - byte-by-byte reads with per-call deadlines (activity timeout)
//   - async CAN-run detection: 5 consecutive CAN bytes cancel the session
//   - listener bookkeeping so the caller can restore the conn cleanly

type reader struct {
	closing   bool
	conn      net.Conn
	mu        sync.Mutex
	buf       []byte
	cond      *sync.Cond
	done      chan struct{}
	closeOnce sync.Once
	cancelled atomic.Bool
	err       error
}

var errActivityTimeout = errors.New("ZMODEM activity timeout - no progress")

func newReader(conn net.Conn) *reader {
	r := &reader{conn: conn, done: make(chan struct{})}
	r.cond = sync.NewCond(&r.mu)
	go r.loop()
	return r
}

// Bound queued bytes and backpressure the connection without dropping frames.
func (r *reader) loop() {
	defer close(r.done)
	buf := make([]byte, 1024)
	consecutiveCan := 0
	for {
		r.mu.Lock()
		for len(r.buf) >= 256<<10 && !r.closing {
			r.cond.Wait()
		}
		stopped := r.closing
		r.mu.Unlock()
		if stopped {
			return
		}
		n, err := r.conn.Read(buf)
		if n > 0 {
			for _, b := range buf[:n] {
				if b == CAN {
					consecutiveCan++
					if consecutiveCan >= canCancelThreshold {
						r.cancelled.Store(true)
					}
				} else {
					consecutiveCan = 0
				}
			}
			r.mu.Lock()
			r.buf = append(r.buf, buf[:n]...)
			r.cond.Broadcast()
			r.mu.Unlock()
		}
		if err != nil {
			r.mu.Lock()
			if r.err == nil {
				r.err = err
			}
			r.cond.Broadcast()
			r.mu.Unlock()
			return
		}
	}
}

// CAN control byte (duplicate of xmodem.CAN; kept local to avoid import cycle).
const CAN = 0x18

// readByte blocks until a byte is available, the deadline elapses, the
// receiver cancels, or the socket fails. Returns ErrCancelled on cancel.
func (r *reader) readByte(timeout time.Duration) (byte, error) {
	deadline := time.Now().Add(timeout)
	r.mu.Lock()
	defer r.mu.Unlock()
	for len(r.buf) == 0 {
		if r.cancelled.Load() {
			go r.emitCancelEcho()
			return 0, ErrCancelled
		}
		if r.err != nil {
			return 0, r.err
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return 0, errActivityTimeout
		}
		// Wait with a guard goroutine that broadcasts on timeout expiry.
		timer := time.AfterFunc(remaining, func() {
			r.mu.Lock()
			r.cond.Broadcast()
			r.mu.Unlock()
		})
		r.cond.Wait()
		timer.Stop()
	}
	// If more than one CAN arrived while we were serving another byte, still
	// surface the cancellation first.
	if r.cancelled.Load() {
		go r.emitCancelEcho()
		return 0, ErrCancelled
	}
	b := r.buf[0]
	r.buf = r.buf[1:]
	r.cond.Signal()
	return b, nil
}

// drain discards any bytes already read from the wire. Used after handling a
// ZRPOS/ZNAK so that follow-up retries the receiver queued while it was
// waiting for us don't get misread as a response to our next burst.
func (r *reader) drain() {
	r.mu.Lock()
	r.buf = r.buf[:0]
	r.cond.Broadcast()
	r.mu.Unlock()
}

func (r *reader) emitCancelEcho() {
	_, _ = r.conn.Write(cancelEcho)
	logger.Info("ZMODEM transfer cancelled by receiver")
}

func (r *reader) close() {
	r.closeOnce.Do(func() {
		r.mu.Lock()
		r.closing = true
		r.cond.Broadcast()
		r.mu.Unlock()
		// Nudge the read loop out of its blocking Read.
		_ = r.conn.SetReadDeadline(time.Now())
		// Restore deadline after the loop exits — nothing else will read conn.
		<-r.done
		_ = r.conn.SetReadDeadline(time.Time{})
		r.mu.Lock()
		tail := append([]byte(nil), r.buf...)
		r.buf = nil
		r.mu.Unlock()
		if c, ok := r.conn.(interface{ ReturnBuffered([]byte) }); ok && len(tail) > 0 {
			c.ReturnBuffered(tail)
		}
	})
}

func (r *reader) unread(b []byte) {
	r.mu.Lock()
	r.buf = append(append([]byte(nil), b...), r.buf...)
	r.cond.Broadcast()
	r.mu.Unlock()
}

func (s *sender) sendCurrentFile(finalizeOnSkip bool) error {
	// Send ZFILE (CRC16, lrzsz fileinfo).
	if _, err := s.conn.Write(s.buildZfile()); err != nil {
		return err
	}

	// Receiver responds with ZRPOS(offset), or ZSKIP/ZABORT to decline.
	offset, err := s.waitForZrpos()
	if err != nil {
		if isPeerRejection(err) {
			if finalizeOnSkip {
				s.gracefulClose()
			}
			return err
		}
		return err
	}

	for {
		// The peer may reject ZEOF and request a final retransmission.
		if err := s.streamData(offset); err != nil {
			if isPeerRejection(err) && finalizeOnSkip {
				s.gracefulClose()
			}
			return err
		}
		if _, err := s.conn.Write(s.codec.header16(FrameZEOF, uint32(s.size))); err != nil {
			return err
		}
		frame, count, err := s.awaitOneOfWithCount(FrameZRINIT, FrameZRPOS, FrameZSKIP, FrameZABORT, FrameZFERR)
		if err != nil {
			return err
		}
		switch frame {
		case FrameZRINIT:
			return nil
		case FrameZRPOS:
			offset = count
		default:
			if finalizeOnSkip {
				s.gracefulClose()
			}
			return peerRejection(frame)
		}
	}
}

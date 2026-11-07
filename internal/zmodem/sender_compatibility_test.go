package zmodem

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSenderReplyEncodings(t *testing.T) {
	for _, format := range []string{"hex", "binary16", "binary32"} {
		for _, noisy := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/flow=%v", format, noisy), func(t *testing.T) {
				encode := func(kind byte, value uint32) []byte {
					h := BuildZhexHeader(kind, value)
					if format == "binary16" {
						h = BuildZbinHeader(kind, value)
					}
					if format == "binary32" {
						h = BuildZbin32Header(kind, value)
					}
					if !noisy {
						return h
					}
					// Legal padding and flow control, including inside ZDLE/CRC.
					h = append([]byte{ZPAD}, bytes.TrimLeft(h, "*")...)
					var wire []byte
					for _, b := range h {
						wire = append(wire, 0x11, b, 0x93)
					}
					return wire
				}
				wire := encode(FrameZRINIT, uint32(CanFDX|CanOVIO|EscCtl)<<24|257)
				wire = append(wire, encode(FrameZRPOS, 0xff7f)...)
				wire = append(wire, encode(FrameZFIN, 0)...)
				c := &wireFixture{Reader: bytes.NewReader(wire)}
				s := newSender(c, nil, "file.bin")
				defer s.reader.close()
				if err := s.waitForZrinit(); err != nil {
					t.Fatal(err)
				}
				if !s.codec.allControls {
					t.Fatal("lost ESCCTL negotiation")
				}
				if got, err := s.waitForZrpos(); err != nil || got != 0xff7f {
					t.Fatalf("resume=%x: %v", got, err)
				}
				if err := s.waitForZfin(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestSenderQueuedInitDoesNotDuplicateFile(t *testing.T) {
	wire := append(BuildZhexHeader(FrameZRINIT, 0x23000000), BuildZhexHeader(FrameZRPOS, 0)...)
	c := &wireFixture{Reader: bytes.NewReader(wire)}
	s := newSender(c, []byte("abc"), "file.bin")
	defer s.reader.close()
	if _, err := s.waitForZrpos(); err != nil {
		t.Fatal(err)
	}
	if c.written.Len() != 0 {
		t.Fatal("resent ZFILE despite queued ZRPOS")
	}
}

func TestSenderAnswersFileCRC(t *testing.T) {
	data := []byte("123456789")
	// ZCRC=13: a prefix request, then a whole-file request before ZRPOS.
	wire := append(BuildZhexHeader(13, 4), BuildZhexHeader(13, 0)...)
	wire = append(wire, BuildZhexHeader(FrameZRPOS, 4)...)
	c := &wireFixture{Reader: bytes.NewReader(wire)}
	s := newSender(c, data, "file.bin")
	defer s.reader.close()
	if got, err := s.waitForZrpos(); err != nil || got != 4 {
		t.Fatalf("resume=%d: %v", got, err)
	}
	reply := &wireFixture{Reader: bytes.NewReader(c.written.Bytes())}
	r := &receiver{conn: reply, reader: newReader(reply)}
	defer r.reader.close()
	// Independent known IEEE CRC32 values, not values from our encoder.
	for _, want := range []uint32{0x9be3e0a3, 0xcbf43926} {
		kind, crc, err := r.readHeader()
		if err != nil || kind != 13 || crc != want {
			t.Fatalf("CRC reply=%d/%08x: %v", kind, crc, err)
		}
	}
}

func TestSenderCRCRejectsTruncatedSource(t *testing.T) {
	c := &wireFixture{Reader: bytes.NewReader(BuildZhexHeader(FrameZCRC, 0))}
	s := newSender(c, []byte("short"), "file.bin")
	s.size = 100
	defer s.reader.close()
	if _, err := s.waitForZrpos(); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("truncated source: %v", err)
	}
	if c.written.Len() != 0 {
		t.Fatal("replied with CRC of a truncated source")
	}
}

// Reads are owned by the protocol reader; SendFiles joins it before returning.
type recordedPeer struct {
	net.Conn
	incoming []byte
}

func (c *recordedPeer) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	c.incoming = append(c.incoming, p[:n]...)
	return n, err
}

func TestLrzszCRCResumeAndCompare(t *testing.T) {
	for _, resume := range []bool{false, true} {
		for _, equal := range []bool{false, true} {
			t.Run(fmt.Sprintf("resume=%v/equal=%v", resume, equal), func(t *testing.T) {
				dir := t.TempDir()
				data := make([]byte, 128<<10)
				for i := range data {
					data[i] = byte(i*37 + (i >> 3))
				}
				existing := append([]byte(nil), data...)
				args := []string{"-b", "-e", "--crc-check"}
				if resume {
					existing = existing[:8197]
					args = []string{"-b", "-e", "-r", "-r"}
				}
				if !equal {
					existing[123] ^= 1
				}
				if err := os.WriteFile(filepath.Join(dir, "data.bin"), existing, 0600); err != nil {
					t.Fatal(err)
				}
				conn, wait := lrzszPeer(t, dir, "rz", args...)
				peer := &recordedPeer{Conn: conn}
				err := SendFiles(peer, []SendFile{{Name: "data.bin", Size: int64(len(data)), Reader: bytes.NewReader(data)}}, nil)
				if err != nil && !errors.Is(err, ErrSkipped) {
					conn.Close()
					wait()
				}
				if !resume && equal {
					// Some 64-bit lrzsz builds compare an unmasked unsigned-long
					// CRC against the 32-bit wire value and retransmit even equal
					// files (also observed with native sz). Both decisions are safe.
					if err != nil && !errors.Is(err, ErrSkipped) {
						t.Fatalf("equal file comparison: %v", err)
					}
				} else if err != nil {
					t.Fatal(err)
				}
				wait()
				got, err := os.ReadFile(filepath.Join(dir, "data.bin"))
				if err != nil || !bytes.Equal(got, data) {
					t.Fatalf("received file differs: %v", err)
				}
				crcRequested, resumed := false, false
				for i := 0; i+18 <= len(peer.incoming); i++ {
					if !bytes.Equal(peer.incoming[i:i+4], []byte{ZPAD, ZPAD, ZDLE, ZHEX}) {
						continue
					}
					kind, flags, ok := DecodeHexHeader(peer.incoming, i+4)
					if !ok {
						continue
					}
					count := uint32(flags[0]) | uint32(flags[1])<<8 | uint32(flags[2])<<16 | uint32(flags[3])<<24
					if kind == FrameZCRC || kind == FrameZRPOS {
						t.Logf("receiver frame=%d count=%d", kind, count)
					}
					if kind == FrameZCRC {
						crcRequested = true
					}
					if kind == FrameZRPOS && count > 0 {
						resumed = true
					}
				}
				if !crcRequested {
					t.Fatal("real receiver did not exercise ZCRC")
				}
				if resumed && (!resume || !equal) {
					t.Fatalf("unsafe resume decision: %v", resumed)
				}
				if resume && equal && !resumed {
					t.Log("receiver chose full retransmission after CRC; content still verified")
				}
			})
		}
	}
}

func TestSenderHonorsReceiverWindow(t *testing.T) {
	for _, window := range []uint32{257, 2048} {
		t.Run(fmt.Sprint(window), func(t *testing.T) {
			local, peer := net.Pipe()
			defer local.Close()
			defer peer.Close()
			peer.SetDeadline(time.Now().Add(5 * time.Second))
			payload := bytes.Repeat([]byte("binary\x00\xff\x7f"), 1200)
			done := make(chan error, 1)
			go func() {
				done <- SendFiles(local, []SendFile{{Name: "window.bin", Size: int64(len(payload)), Reader: bytes.NewReader(payload)}}, nil)
			}()
			r := &receiver{conn: peer, reader: newReader(peer)}
			defer r.reader.close()
			peer.Write(BuildZhexHeader(FrameZRINIT, uint32(CanFDX|CanOVIO)<<24|window))
			kind, _, err := r.readHeader()
			if err != nil || kind != FrameZFILE {
				t.Fatalf("file header %d: %v", kind, err)
			}
			if _, _, err := r.readSubpacket(); err != nil {
				t.Fatal(err)
			}
			peer.Write(BuildZhexHeader(FrameZRPOS, 0))
			var received []byte
			for len(received) < len(payload) {
				kind, offset, err := r.readHeader()
				if err != nil || kind != FrameZDATA || int(offset) != len(received) {
					t.Fatalf("data header %d/%d: %v", kind, offset, err)
				}
				burst := 0
				for {
					data, end, err := r.readSubpacket()
					if err != nil {
						t.Fatal(err)
					}
					burst += len(data)
					if burst > int(window) {
						t.Fatalf("burst %d exceeds receiver window %d", burst, window)
					}
					received = append(received, data...)
					if end == ZCRCW {
						break
					}
				}
				peer.Write(BuildZhexHeader(FrameZACK, uint32(len(received))))
			}
			if !bytes.Equal(received, payload) {
				t.Fatal("corrupt received file")
			}
			if kind, _, err := r.readHeader(); err != nil || kind != FrameZEOF {
				t.Fatalf("EOF %d: %v", kind, err)
			}
			peer.Write(BuildZhexHeader(FrameZRINIT, 0x23000000))
			if kind, _, err := r.readHeader(); err != nil || kind != FrameZFIN {
				t.Fatalf("FIN %d: %v", kind, err)
			}
			peer.Write(BuildZhexHeader(FrameZFIN, 0))
			r.readOOBestEffort()
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}

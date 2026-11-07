package zmodem

import (
	"bytes"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

// A synchronous fixture keeps the protocol assertions independent of our sender.
type wireFixture struct {
	*bytes.Reader
	written bytes.Buffer
}

func (c *wireFixture) Write(p []byte) (int, error)    { return c.written.Write(p) }
func (*wireFixture) Close() error                     { return nil }
func (*wireFixture) LocalAddr() net.Addr              { return nil }
func (*wireFixture) RemoteAddr() net.Addr             { return nil }
func (*wireFixture) SetDeadline(time.Time) error      { return nil }
func (*wireFixture) SetReadDeadline(time.Time) error  { return nil }
func (*wireFixture) SetWriteDeadline(time.Time) error { return nil }

func TestReceiveAdvertisesStandardZRINIT(t *testing.T) {
	c := &wireFixture{Reader: bytes.NewReader(nil)}
	r := &receiver{conn: c}
	if err := r.sendZRINIT(); err != nil {
		t.Fatal(err)
	}
	kind, flags, ok := DecodeHexHeader(c.written.Bytes(), 4)
	if !ok || kind != FrameZRINIT || flags != [4]byte{0, 0x20, 0, 0x23} {
		t.Fatalf("ZRINIT wire flags = %x; need buffer 8192 and capabilities in ZF0", flags)
	}
}

func TestSenderDoesNotReuseConsumedZRINIT(t *testing.T) {
	wire := append(BuildZhexHeader(FrameZRINIT, 0x23000000), []byte("unrelated")...)
	c := &wireFixture{Reader: bytes.NewReader(wire)}
	s := newSender(c, nil, "")
	defer s.reader.close()
	if err := s.waitForZrinit(); err != nil {
		t.Fatal(err)
	}
	if err := s.waitForZrinit(); !errors.Is(err, io.EOF) {
		t.Fatalf("consumed ZRINIT matched again: %v", err)
	}
}

func fixtureSubpacket(data []byte, crc32 bool) []byte {
	out := AppendEscaped(nil, data)
	out = append(out, ZDLE, ZCRCW)
	input := append(append([]byte(nil), data...), ZCRCW)
	if crc32 {
		crc := CRC32(input)
		return AppendEscaped(out, []byte{byte(crc), byte(crc >> 8), byte(crc >> 16), byte(crc >> 24)})
	}
	crc := CRC16(input)
	return AppendEscaped(out, []byte{byte(crc >> 8), byte(crc)})
}

func TestReceiveChangesCRCWithEachHeader(t *testing.T) {
	var wire []byte
	for _, crc32 := range []bool{true, false, true, false} {
		header := BuildZbinHeader(FrameZDATA, 0)
		if crc32 {
			header = BuildZbin32Header(FrameZDATA, 0)
		}
		wire = append(wire, header...)
		wire = append(wire, fixtureSubpacket([]byte("payload"), crc32)...)
	}
	c := &wireFixture{Reader: bytes.NewReader(wire)}
	r := &receiver{conn: c, reader: newReader(c)}
	defer r.reader.close()
	for i := 0; i < 4; i++ {
		if _, _, err := r.readHeader(); err != nil {
			t.Fatal(err)
		}
		if data, _, err := r.readSubpacket(); err != nil || string(data) != "payload" {
			t.Fatalf("packet %d: %q %v", i, data, err)
		}
	}
}

func TestReceiveHexZSINITSubpacket(t *testing.T) {
	for _, newline := range []byte{'\n', 0x8a} {
		header := BuildZhexHeader(FrameZSINIT, 0x40000000)
		header[19] = newline
		wire := append(header, fixtureSubpacket([]byte{0}, false)...)
		c := &wireFixture{Reader: bytes.NewReader(wire)}
		r := &receiver{conn: c, reader: newReader(c), crc32Mode: true}
		if _, _, err := r.readHeader(); err != nil {
			t.Fatal(err)
		}
		data, _, err := r.readSubpacket()
		r.reader.close()
		if err != nil || !bytes.Equal(data, []byte{0}) {
			t.Fatalf("ZSINIT trailer %x leaked into attention string: %x %v", newline, data, err)
		}
	}
}

func TestReceiveDisconnectDoesNotRequestRetransmission(t *testing.T) {
	c := &wireFixture{Reader: bytes.NewReader([]byte("incomplete"))}
	r := &receiver{conn: c, reader: newReader(c)}
	defer r.reader.close()
	_, err := r.readDataBurst()
	if !errors.Is(err, io.EOF) || c.written.Len() != 0 {
		t.Fatalf("disconnect treated as CRC error: %v, wrote %x", err, c.written.Bytes())
	}
}

func TestReceiveRepliesUseHexForSenderResynchronization(t *testing.T) {
	r := &receiver{crc32Mode: true}
	for _, kind := range []byte{FrameZACK, FrameZRPOS, FrameZSKIP} {
		if got := r.buildHeader(kind, 42); !bytes.Equal(got, BuildZhexHeader(kind, 42)) {
			t.Fatalf("reply %d is not a double-pad hex header: %x", kind, got)
		}
	}
}

func TestReceiveCRCErrorKeepsAlreadyBufferedRetransmission(t *testing.T) {
	bad := fixtureSubpacket([]byte("bad"), false)
	bad[len(bad)-1] ^= 1
	wire := append(BuildZbinHeader(FrameZDATA, 0), bad...)
	wire = append(wire, BuildZbinHeader(FrameZDATA, 0)...)
	wire = append(wire, fixtureSubpacket([]byte("good"), false)...)
	wire = append(wire, BuildZbinHeader(FrameZEOF, 4)...)
	c := &wireFixture{Reader: bytes.NewReader(wire)}
	var got bytes.Buffer
	r := &receiver{conn: c, reader: newReader(c), writer: &got}
	defer r.reader.close()
	if err := r.receiveData(); err != nil || got.String() != "good" {
		t.Fatalf("retransmission lost/corrupted: %q %v", got.String(), err)
	}
	if !bytes.Contains(c.written.Bytes(), BuildZhexHeader(FrameZRPOS, 0)) {
		t.Fatal("missing retransmission request")
	}
}

func TestSenderRetriesWhenReceiverRejectsEOF(t *testing.T) {
	local, remote := net.Pipe()
	defer local.Close()
	defer remote.Close()
	remote.SetDeadline(time.Now().Add(3 * time.Second))
	done := make(chan error, 1)
	progress := make(chan FileProgress, 8)
	go func() {
		done <- SendFiles(local, []SendFile{{Name: "retry.bin", Size: 3, Reader: bytes.NewReader([]byte("abc"))}}, func(p FileProgress) { progress <- p })
	}()
	r := &receiver{conn: remote, reader: newReader(remote)}
	defer r.reader.close()
	header := func(want byte) {
		t.Helper()
		kind, _, err := r.readHeader()
		if err != nil || kind != want {
			t.Fatalf("header want=%d got=%d: %v", want, kind, err)
		}
	}
	// An already-running rz initiates the upload. The first sender response
	// must be ZFILE, not another initialization request.
	remote.Write(BuildZhexHeader(FrameZRINIT, 0x23000000))
	header(FrameZFILE)
	if _, _, err := r.readSubpacket(); err != nil {
		t.Fatal(err)
	}
	remote.Write(BuildZhexHeader(FrameZRPOS, 0))
	for attempt := 0; attempt < 2; attempt++ {
		header(FrameZDATA)
		if data, _, err := r.readSubpacket(); err != nil || string(data) != "abc" {
			t.Fatalf("retry data %q: %v", data, err)
		}
		if attempt == 0 {
			remote.Write(BuildZhexHeader(FrameZACK, 0))
			select {
			case p := <-progress:
				t.Fatalf("stale ACK advanced progress: %+v", p)
			case <-time.After(20 * time.Millisecond):
			}
		}
		remote.Write(BuildZhexHeader(FrameZACK, 3))
		header(FrameZEOF)
		if attempt == 0 {
			remote.Write(BuildZhexHeader(FrameZRPOS, 0))
		}
	}
	remote.Write(BuildZhexHeader(FrameZRINIT, 0x23000000))
	header(FrameZFIN)
	remote.Write(BuildZhexHeader(FrameZFIN, 0))
	r.readOOBestEffort()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestSenderRejectsOutOfRangeResume(t *testing.T) {
	s := &sender{size: 3}
	if err := s.streamData(4); err == nil {
		t.Fatal("out-of-range resume accepted")
	}
}

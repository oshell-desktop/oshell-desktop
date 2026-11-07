package terminal

import (
	"bytes"
	"context"
	"io"
	"net"
	"oshell-desktop/internal/zmodem"
	"strings"
	"testing"
	"time"
)

type transferBuffer struct{ bytes.Buffer }

func (*transferBuffer) Close() error { return nil }

func TestTransferMultiplexerDownloadThenShellThenUpload(t *testing.T) {
	loadLib(t)
	local, remote := net.Pipe()
	defer remote.Close()
	done := make(chan error, 2)
	received := &transferBuffer{}
	payload := bytes.Repeat([]byte{0, 0x18, 0x11, 0xff, 'x'}, 20000)
	term, e := New(Options{Conn: local, NoBlink: true, AutomaticTransfers: true,
		TransferHandler: func(ctx context.Context, upload bool, conn net.Conn) error {
			if upload {
				return zmodem.SendFiles(conn, []zmodem.SendFile{{Name: "back.bin", Size: int64(len(payload)), Reader: bytes.NewReader(payload)}}, nil)
			}
			return zmodem.ReceiveFiles(conn, func(zmodem.ReceiveFile) (io.WriteCloser, error) { return received, nil }, nil)
		}, TransferFinished: func(e error) { done <- e }})
	if e != nil {
		t.Fatal(e)
	}
	defer term.Close()
	remoteDone := make(chan error, 1)
	go func() {
		// Act as sz: request a receiver before entering the file sender.
		if _, err := remote.Write(zmodem.BuildZhexHeader(zmodem.FrameZRQINIT, 0)); err != nil {
			remoteDone <- err
			return
		}
		remoteDone <- zmodem.SendFiles(remote, []zmodem.SendFile{{Name: "in.bin", Size: int64(len(payload)), Reader: bytes.NewReader(payload)}}, nil)
	}()
	wait := func(ch <-chan error) {
		t.Helper()
		select {
		case e := <-ch:
			if e != nil {
				t.Fatal(e)
			}
		case <-time.After(15 * time.Second):
			t.Fatal("transfer timeout")
		}
	}
	wait(remoteDone)
	wait(done)
	if !bytes.Equal(received.Bytes(), payload) {
		t.Fatal("download corrupt")
	}
	if !term.Available() {
		t.Fatal("input not restored")
	}
	if _, e = remote.Write([]byte("\r\nshell-ready$ ")); e != nil {
		t.Fatal(e)
	}
	deadline := time.Now().Add(time.Second)
	for !strings.Contains(term.Text(), "shell-ready$") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !strings.Contains(term.Text(), "shell-ready$") {
		t.Fatalf("lost prompt: %q", term.Text())
	}
	back := &transferBuffer{}
	go func() {
		remoteDone <- zmodem.ReceiveFiles(remote, func(zmodem.ReceiveFile) (io.WriteCloser, error) { return back, nil }, nil)
	}()
	wait(remoteDone)
	wait(done)
	if !bytes.Equal(back.Bytes(), payload) {
		t.Fatal("upload corrupt")
	}
}

func TestTransferMultiplexerCancellationRestoresInput(t *testing.T) {
	loadLib(t)
	source := newPipe()
	started := make(chan struct{})
	finished := make(chan error, 1)
	term, e := New(Options{Conn: source, NoBlink: true, AutomaticTransfers: true, TransferHandler: func(ctx context.Context, _ bool, _ net.Conn) error { close(started); <-ctx.Done(); return ctx.Err() }, TransferFinished: func(e error) { finished <- e }})
	if e != nil {
		t.Fatal(e)
	}
	defer term.Close()
	term.transfers.feed(zmodem.BuildZhexHeader(zmodem.FrameZRINIT, 0))
	<-started
	if term.Available() {
		t.Fatal("input not paused")
	}
	term.ConfirmedPaste("must-not-be-sent")
	term.CancelTransfer()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("cancel blocked")
	}
	if !term.Available() {
		t.Fatal("input still paused")
	}
	if got := source.take(1); strings.Contains(got, "must-not-be-sent") || !strings.Contains(got, string(bytes.Repeat([]byte{0x18}, 8))) {
		t.Fatalf("cancel bytes %q", got)
	}
	term.ConfirmedPaste("next command")
	if got := source.take(12); got != "next command" {
		t.Fatalf("input after cancel: %q", got)
	}
}

// The reader may finish an upload while the transport already holds the next
// download header. Force that handoff before delivering the download payload.
func TestTransferMultiplexerCompletedSessionHandsOffBufferedHeader(t *testing.T) {
	loadLib(t)
	header := zmodem.BuildZhexHeader(zmodem.FrameZRQINIT, 0)
	payload := []byte("next-transfer-payload")
	want := append(append([]byte(nil), header...), payload...)
	received := make(chan []byte, 1)
	finishedBeforeNext := make(chan bool, 2)
	var term *Terminal
	var err error
	term, err = New(Options{Conn: newPipe(), NoBlink: true, AutomaticTransfers: true,
		TransferHandler: func(ctx context.Context, upload bool, conn net.Conn) error {
			conn.SetReadDeadline(time.Now().Add(time.Second))
			buf := make([]byte, len(want))
			n, err := io.ReadFull(conn, buf)
			received <- buf[:n]
			return err
		},
		TransferFinished: func(error) { finishedBeforeNext <- term.transfers.active == nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	c := term.transfers
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	previous := &protocolSession{ctx: ctx, cancel: cancel, done: make(chan struct{})}
	previous.pipe = newProtocolPipe(ctx, previous.done, nil)
	previous.pipe.ReturnBuffered(header)
	previous.pipe.finish()
	close(previous.done)
	c.mu.Lock()
	c.active = previous
	term.in.pause(true)
	c.mu.Unlock()
	c.feed(payload)
	if !<-finishedBeforeNext {
		t.Error("previous completion callback ran after next transfer started")
	}
	select {
	case got := <-received:
		if !bytes.Equal(got, want) {
			t.Fatalf("next transfer lost payload: got %q, want %q", got, want)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("next transfer stalled")
	}
	<-finishedBeforeNext
	if !term.Available() {
		t.Fatal("input not restored after consecutive transfers")
	}
}

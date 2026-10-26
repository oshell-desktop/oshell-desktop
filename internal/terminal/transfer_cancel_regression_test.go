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

func TestTransferSkippedBatchRestoresShellWithoutAbort(t *testing.T) {
	loadLib(t)
	source := newPipe()
	header := zmodem.BuildZhexHeader(zmodem.FrameZRINIT, 0)
	finished := make(chan error, 1)
	term, err := New(Options{Conn: source, NoBlink: true, AutomaticTransfers: true,
		TransferHandler: func(_ context.Context, _ bool, conn net.Conn) error {
			if _, err := io.ReadFull(conn, make([]byte, len(header))); err != nil {
				return err
			}
			return &zmodem.BatchResultError{Skipped: []string{"kept.bin"}}
		}, TransferFinished: func(err error) { finished <- err },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	term.transfers.feed(append(header, []byte("\r\nshell-ready$ ")...))
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("batch did not finish")
	}
	deadline := time.Now().Add(time.Second)
	for !strings.Contains(term.Text(), "shell-ready$") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !term.Available() || !strings.Contains(term.Text(), "shell-ready$") {
		t.Fatal("shell was not restored")
	}
	source.mu.Lock()
	sent := len(source.sent)
	source.mu.Unlock()
	if sent != 0 {
		t.Fatal("abort bytes were sent after completed batch")
	}
}

func TestTransferPeerCancelWhilePickerOpen(t *testing.T) {
	loadLib(t)
	choosing := make(chan struct{})
	stopped := make(chan struct{})
	term, err := New(Options{Conn: newPipe(), NoBlink: true, AutomaticTransfers: true,
		TransferHandler: func(ctx context.Context, _ bool, _ net.Conn) error {
			close(choosing)
			<-ctx.Done()
			close(stopped)
			return ctx.Err()
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	term.transfers.feed(zmodem.BuildZhexHeader(zmodem.FrameZRINIT, 0))
	<-choosing
	term.transfers.feed(bytes.Repeat([]byte{0x18}, 3))
	select {
	case <-stopped:
		t.Fatal("partial CAN sequence cancelled the picker")
	default:
	}
	term.transfers.feed(append(bytes.Repeat([]byte{0x18}, 5), []byte("\r\nshell-ready$ ")...))
	select {
	case <-stopped:
	case <-time.After(300 * time.Millisecond):
		term.CancelTransfer()
		<-stopped
		t.Fatal("peer cancellation was buffered without cancelling the open picker; terminal input remained paused")
	}
}

func TestTransferCancelWhilePickerBackpressures(t *testing.T) {
	loadLib(t)
	choosing := make(chan struct{})
	term, err := New(Options{Conn: newPipe(), NoBlink: true, AutomaticTransfers: true,
		TransferHandler: func(ctx context.Context, _ bool, _ net.Conn) error { close(choosing); <-ctx.Done(); return ctx.Err() },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	term.transfers.feed(zmodem.BuildZhexHeader(zmodem.FrameZRINIT, 0))
	<-choosing
	session := term.transfers.active
	defer session.cancel()
	fed := make(chan struct{})
	go func() { term.transfers.feed(make([]byte, 256<<10)); close(fed) }()
	deadline := time.Now().Add(2 * time.Second)
	for {
		session.pipe.mu.Lock()
		full := len(session.pipe.buffer) == 256<<10
		session.pipe.mu.Unlock()
		if full {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("fixture did not fill buffer")
		}
		time.Sleep(time.Millisecond)
	}
	cancelled := make(chan struct{})
	go func() { term.CancelTransfer(); close(cancelled) }()
	blocked := false
	select {
	case <-cancelled:
	case <-time.After(300 * time.Millisecond):
		blocked = true
	}
	// Rescue only this synthetic transfer, so a failing probe cannot hang tests.
	session.cancel()
	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("cancel rescue failed")
	}
	select {
	case <-fed:
	case <-time.After(2 * time.Second):
		t.Fatal("feed rescue failed")
	}
	if blocked {
		t.Fatal("CancelTransfer blocked behind full protocol buffer while picker waited for cancellation")
	}
}

package terminal

import (
	"context"
	"net"
	"oshell-desktop/internal/zmodem"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestExplicitUploadHandshakeCancelAndTimeout(t *testing.T) {
	loadLib(t)
	for _, mode := range []string{"cancel", "timeout", "handshake"} {
		t.Run(mode, func(t *testing.T) {
			source := newPipe()
			finished := make(chan error, 2)
			selected := make(chan []string, 2)
			var busy atomic.Bool
			term, err := New(Options{Conn: source, NoBlink: true, AutomaticTransfers: false,
				TransferStarted: func() { busy.Store(true) },
				TransferHandler: func(ctx context.Context, _ bool, _ net.Conn) error {
					selected <- UploadPaths(ctx)
					return nil
				}, TransferFinished: func(err error) { busy.Store(false); finished <- err }})
			if err != nil {
				t.Fatal(err)
			}
			defer term.Close()
			paths := []string{"original.bin"}
			if err = term.startUpload(paths, 200*time.Millisecond); err != nil {
				t.Fatal(err)
			}
			paths[0] = "must-not-replace.bin"
			if !busy.Load() {
				t.Fatal("waiting for rz did not suppress managed idle input")
			}
			if err = term.StartUpload(paths); err == nil {
				t.Fatal("accepted concurrent upload")
			}
			if got := source.take(4); got != "\x15rz\r" {
				t.Fatalf("command %q", got)
			}
			term.ConfirmedPaste("must-not-mix")
			switch mode {
			case "cancel":
				term.CancelTransfer()
			case "timeout":
				term.transfers.feed([]byte("rz: command not found\r\n"))
			case "handshake":
				header := zmodem.BuildZhexHeader(zmodem.FrameZRINIT, 0)
				for _, b := range header {
					term.transfers.feed([]byte{b})
				}
			}
			select {
			case err = <-finished:
			case <-time.After(3 * time.Second):
				t.Fatal("completion blocked")
			}
			if !term.Available() || busy.Load() {
				t.Fatal("input not restored")
			}
			if mode == "handshake" {
				if err != nil || !slices.Equal(<-selected, []string{"original.bin"}) {
					t.Fatal("drop not bound to its session", err)
				}
			} else {
				if err == nil {
					t.Fatal("false success")
				}
				if got := source.take(1); got != "\x03" {
					t.Fatalf("pending cancellation %q", got)
				}
				if len(selected) != 0 {
					t.Fatal("sent file before handshake")
				}
				if mode == "timeout" && !strings.Contains(term.Text(), "command not found") {
					t.Fatal("shell error hidden")
				}
			}
			term.ConfirmedPaste("ok")
			if got := source.take(2); got != "ok" {
				t.Fatalf("input after transfer %q", got)
			}
			term.SetAutomaticTransfers(true)
			term.transfers.feed(zmodem.BuildZhexHeader(zmodem.FrameZRINIT, 0))
			select {
			case paths := <-selected:
				if len(paths) != 0 {
					t.Fatal("stale dropped files reused")
				}
			case <-time.After(time.Second):
				t.Fatal("automatic transfer lost")
			}
			<-finished
		})
	}
}

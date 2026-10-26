package terminal

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/zmodem"
)

func TestFocusReportsFollowVisibleHost(t *testing.T) {
	loadLib(t)
	conn := newPipe()
	term, err := New(Options{Conn: conn, NoBlink: true})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	// Commands arriving before the view appears must see the unfocused state.
	term.Feed([]byte("\x1b[?1004h"))
	if got := conn.take(3); got != "\x1b[O" {
		t.Fatalf("before view: %q", got)
	}
	visible := true
	tt := ui.NewTester(func(c *ui.Context) {
		if visible {
			View(c, term).Fill().AutoFocus()
		} else {
			ui.Text(c, "other tab")
		}
	}, 400, 200)
	tt.Frame()
	if got := conn.take(3); got != "\x1b[I" {
		t.Fatalf("focus view: %q", got)
	}
	check := func(want string) {
		t.Helper()
		// A queue barrier makes absence and duplicate-report checks deterministic.
		term.Send([]byte("|"))
		if got := conn.take(len(want) + 1); got != want+"|" {
			t.Fatalf("got %q want %q", got, want+"|")
		}
	}
	tt.Frame()
	check("")
	tt.SetFocused(false)
	check("\x1b[O")
	tt.SetFocused(false)
	check("")
	term.Feed([]byte("\x1b[?1004l"))
	tt.SetFocused(true)
	check("")
	term.Feed([]byte("\x1b[?1004h"))
	check("\x1b[I")
	visible = false
	term.SetVisible(false)
	tt.Frame()
	check("\x1b[O")
	term.Feed([]byte("\x1b[?1004h"))
	check("\x1b[O")
	visible = true
	term.SetVisible(true)
	tt.Frame()
	check("\x1b[I")
	term.Feed([]byte("\x1b G"))
	tt.SetFocused(false)
	check("\x9bO")
	tt.SetFocused(true)
	check("\x9bI")
}

func TestFocusReportsDoNotEnterFileTransfer(t *testing.T) {
	loadLib(t)
	conn := newPipe()
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	header := zmodem.BuildZhexHeader(zmodem.FrameZRINIT, 0)
	term, err := New(Options{Conn: conn, NoBlink: true, AutomaticTransfers: true,
		TransferHandler: func(ctx context.Context, _ bool, stream net.Conn) error {
			if _, err := io.ReadFull(stream, make([]byte, len(header))); err != nil {
				return err
			}
			close(started)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}, TransferFinished: func(error) { close(finished) },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill().AutoFocus() }, 400, 200)
	tt.Frame()
	term.Feed([]byte("\x1b[?1004h"))
	if got := conn.take(3); got != "\x1b[I" {
		t.Fatal("setup", got)
	}
	term.transfers.feed(header)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("transfer did not start")
	}
	tt.SetFocused(false)
	tt.SetFocused(true)
	// Only the transfer's own raw writer may send bytes during the transfer.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := term.in.raw(ctx, []byte("binary")); err != nil {
		t.Fatal(err)
	}
	if got := conn.take(6); got != "binary" {
		t.Fatalf("focus contaminated transfer: %q", got)
	}
	close(release)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("transfer did not finish")
	}
	term.Feed([]byte("\x1b[?1004h"))
	if got := conn.take(3); got != "\x1b[I" {
		t.Fatalf("focus after transfer: %q", got)
	}
}

func TestFocusReportsFollowEditorAndTerminal(t *testing.T) {
	loadLib(t)
	conn := newPipe()
	term, err := New(Options{Conn: conn, NoBlink: true})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	var terminalFocus, editorFocus ui.Handle
	var draft string
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() {
			View(c, term).Bind(&terminalFocus).Grow(1).FillWidth().AutoFocus()
			ui.TextInput(c, &draft).Bind(&editorFocus).Height(28).FillWidth()
		})
	}, 400, 240)
	tt.Frame()
	term.Feed([]byte("\x1b[?1004h"))
	if got := conn.take(3); got != "\x1b[I" {
		t.Fatalf("initial: %q", got)
	}
	editorFocus.Focus()
	tt.Frame()
	if got := conn.take(3); got != "\x1b[O" {
		t.Fatalf("editor: %q", got)
	}
	tt.Type("draft")
	if draft != "draft" {
		t.Fatal("editor did not receive input", draft)
	}
	term.Feed([]byte("\x1b[?1004h"))
	if got := conn.take(3); got != "\x1b[O" {
		t.Fatalf("enable while editing: %q", got)
	}
	terminalFocus.Focus()
	tt.Frame()
	if got := conn.take(3); got != "\x1b[I" {
		t.Fatalf("return to terminal: %q", got)
	}
}

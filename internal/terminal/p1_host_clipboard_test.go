package terminal

import (
	"github.com/egoist/mygo/ui"
	"testing"
)

func TestP1OSC52DeniedByProductionTerminal(t *testing.T) {
	loadLib(t)
	conn := newPipe()
	term, err := New(Options{Conn: conn, NoBlink: true})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill() }, 640, 300)
	tt.SetClipboard("synthetic-original")
	for _, part := range []string{"\x1b]52;c;ZXZpbA==\a", "\x1b]52;c;?\a", "\x1b]52;", "c;ZXZpbA==", "\x1b\\", "\x1b]52;p;ZXZpbA==\a"} {
		term.Feed([]byte(part))
		tt.Frame()
	}
	if tt.Clipboard() != "synthetic-original" || len(term.clipOut) != 0 {
		t.Fatal("remote OSC52 wrote host clipboard")
	}
	// A user paste is a write barrier: any earlier query reply would precede it.
	term.ConfirmedPaste("manual")
	if sent := conn.take(6); sent != "manual" {
		t.Fatalf("OSC52 query replied or manual paste changed: %q", sent)
	}
}

func TestMiddleClickPasteRespectsMouseTracking(t *testing.T) {
	term, tt := wordClickFixture(t, "", 500)
	tt.SetClipboard("sample")
	var received []string
	term.opts.BeforePaste = func(s string) bool {
		received = append(received, s)
		return false
	}
	ev := ui.InputEvent{Kind: ui.InputPointerDown, Button: 2, X: 20, Y: 20}
	term.v.handleInput(ev)
	term.SetMiddleClickPaste(true)
	term.v.handleInput(ev)
	term.Feed([]byte("\x1b[?1000h"))
	term.v.handleInput(ev)
	if len(received) != 1 || received[0] != "sample" {
		t.Fatalf("clipboard deliveries: %q", received)
	}
}

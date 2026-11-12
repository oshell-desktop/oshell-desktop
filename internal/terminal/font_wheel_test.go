package terminal

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestFontWheelAccumulation(t *testing.T) {
	now := time.Now()
	var s fontWheelState
	for i := 0; i < 3; i++ {
		if s.step(-99, now) != 0 {
			t.Fatal("font zoomed before threshold")
		}
	}
	if s.step(-99, now) != 1 {
		t.Fatal("accumulated wheel did not enlarge font")
	}
	if s.step(250, now) != 0 || s.step(50, now) != -1 {
		t.Fatal("direction reversal retained old deltas")
	}
	s.step(-250, now)
	if s.step(-100, now.Add(2*time.Second)) != 0 {
		t.Fatal("old partial gesture survived pause")
	}
	if s.step(-10000, now.Add(2*time.Second)) != 1 {
		t.Fatal("coalesced event must only change one size")
	}
	s = fontWheelState{}
	for i := 0; i < 99; i++ {
		if s.step(-3, now) != 0 {
			t.Fatal("precise scroll too sensitive")
		}
	}
	if s.step(-3, now) != 1 {
		t.Fatal("precise deltas not accumulated")
	}
}

func TestCtrlWheelFontZoomStaysLocal(t *testing.T) {
	loadLib(t)
	conn := newPipe()
	term, err := New(Options{Conn: conn, Font: Font{Family: "monospace", Size: 13}, NoBlink: true})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill().AutoFocus() }, 600, 300)
	cols, _ := term.Size()
	term.Feed([]byte("\x1b[?1049h\x1b[?1000h\x1b[?1006h"))
	tt.Frame()
	for i := 0; i < 4; i++ {
		term.v.handleInput(ui.InputEvent{Kind: ui.InputScroll, Mods: ui.Ctrl, DY: -99, X: 30, Y: 30})
	}
	tt.Frame()
	if term.opts.Font.Size != 14 || term.opts.Font.Family != "monospace" {
		t.Fatal("zoom changed wrong font fields", term.opts.Font)
	}
	term.Send([]byte("barrier"))
	if got := conn.take(7); got != "barrier" {
		t.Fatalf("Ctrl wheel leaked remote mouse bytes: %q", got)
	}
	term.v.handleInput(ui.InputEvent{Kind: ui.InputCancel})
	tt.HoldModifiers(ui.Ctrl)
	term.v.handleInput(ui.InputEvent{Kind: ui.InputScroll, Mods: ui.Ctrl, DY: -250})
	tt.HoldModifiers(0)
	term.v.handleInput(ui.InputEvent{Kind: ui.InputScroll, Mods: ui.Ctrl, DY: -100})
	if term.opts.Font.Size != 14 {
		t.Fatal("modifier release retained partial zoom")
	}
	// Adjacent point sizes can round to the same physical cell width.
	for i := 0; i < 4; i++ {
		term.v.handleInput(ui.InputEvent{Kind: ui.InputScroll, Mods: ui.Ctrl, DY: -300})
	}
	tt.Frame()
	if after, _ := term.Size(); after >= cols {
		t.Fatal("zoom did not resize terminal grid", cols, after)
	}
	for _, bound := range []struct{ size, delta float32 }{{26, -300}, {10, 300}} {
		term.SetFont(Font{Size: bound.size})
		term.v.handleInput(ui.InputEvent{Kind: ui.InputCancel})
		term.v.handleInput(ui.InputEvent{Kind: ui.InputScroll, Mods: ui.Ctrl, DY: bound.delta})
		if term.opts.Font.Size != bound.size {
			t.Fatal("font zoom exceeded existing size bounds")
		}
	}
	term.v.handleInput(ui.InputEvent{Kind: ui.InputScroll, DY: 40, X: 30, Y: 30})
	if got := conn.take(1); got == "" {
		t.Fatal("ordinary remote mouse wheel was swallowed")
	}
}

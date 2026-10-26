package vt

import (
	"bytes"
	"fmt"
	"testing"
)

// SwiftTerm keeps options.cursorStyle (the view's appearance) separate from
// cursorBlink (DEC mode 12). DECSTR preserves both; RIS only clears mode 12.
func TestOShellCursorStyleAndBlinkReport(t *testing.T) {
	for _, chunk := range []int{1, 7, 4096} {
		var reply bytes.Buffer
		term, rs := resetTerminal(t, Effects{WritePTY: func(b []byte) { reply.Write(b) }})
		term.SetDefaultCursor(CursorBar, false)
		check := func(style CursorStyle, blink, mode bool) {
			t.Helper()
			if err := rs.Update(term); err != nil {
				t.Fatal(err)
			}
			c := rs.Cursor()
			if c.Style != style || c.Blinking != blink {
				t.Fatalf("chunk=%d cursor=%+v want style=%v blink=%v", chunk, c, style, blink)
			}
			reply.Reset()
			writeControlChunks(term, []byte("\x1b[?12$p"), chunk)
			state := 2
			if mode {
				state = 1
			}
			if got, want := reply.String(), fmt.Sprintf("\x1b[?12;%d$y", state); got != want {
				t.Errorf("mode report=%q want=%q", got, want)
			}
			if term.Mode(12) != mode {
				t.Error("public mode query disagrees with protocol")
			}
		}
		write := func(s string) { writeControlChunks(term, []byte(s), chunk) }
		check(CursorBar, false, false)
		for _, tc := range []struct {
			command string
			style   CursorStyle
			blink   bool
		}{
			{"\x1b[ q", CursorBlock, true},
			{"\x1b[0 q", CursorBlock, true},
			{"\x1b[1;99 q", CursorBlock, true},
			{"\x1b[2 q", CursorBlock, false},
			{"\x1b[3 q", CursorUnderline, true},
			{"\x1b[4; q", CursorUnderline, false},
			{"\x1b[5 q", CursorBar, true},
			{"\x1b[6 q", CursorBar, false},
		} {
			write(tc.command)
			check(tc.style, tc.blink, false)
			write("\x1b[?12h")
			check(tc.style, tc.blink, true)
			write("\x1b[!p")
			check(tc.style, tc.blink, true)
			write("\x1bc")
			check(tc.style, tc.blink, false)
		}
		write("\x1b[5 q\x1b[?12h\x1b[99 q\x1b[0q")
		check(CursorBar, true, true) // invalid values/intermediates do nothing
		write("\x1b[?1049h\x1b[4 q\x1b[?1049l\x1b7\x1b[5 q\x1b8")
		check(CursorBar, true, true) // appearance is not a saved-cursor property
		write("\x1b[?12l")
		check(CursorBar, true, false)
		write("\x1b G\x1b[?12$p")
		if !bytes.HasSuffix(reply.Bytes(), []byte("\x9b?12;2$y")) {
			t.Errorf("8-bit report=%q", reply.String())
		}
	}
}

func TestOShellCursorStyleReplay(t *testing.T) {
	for _, mode := range []bool{false, true} {
		for style := 1; style <= 6; style++ {
			term, rs := resetTerminal(t, Effects{})
			term.Write(fmt.Appendf(nil, "\x1b[%d q", style))
			if mode {
				term.Write([]byte("\x1b[?12h"))
			}
			if err := rs.Update(term); err != nil {
				t.Fatal(err)
			}
			want := rs.Cursor()
			replayed, rendered := resetTerminal(t, Effects{})
			replayed.SetDefaultCursor(CursorUnderline, true)
			replayed.Write([]byte("\x1b[?12h"))
			replayed.Write(term.VT())
			if err := rendered.Update(replayed); err != nil {
				t.Fatal(err)
			}
			got := rendered.Cursor()
			if got.Style != want.Style || got.Blinking != want.Blinking || replayed.Mode(12) != mode {
				t.Errorf("style=%d mode=%v replay=%+v want=%+v protocol=%v", style, mode, got, want, replayed.Mode(12))
			}
		}
	}
}

func TestOShellUnsupportedPrivateModeSaveRestore(t *testing.T) {
	var reply bytes.Buffer
	term, rs := resetTerminal(t, Effects{WritePTY: func(b []byte) { reply.Write(b) }})
	term.Write([]byte("\x1b[?7;12;1004;1049s\x1b[?7l\x1b[?12;1004h\x1b[5 q\x1b[?7;12;1004;1049r"))
	if term.Mode(7) || !term.Mode(12) || !term.Mode(1004) || term.Mode(1049) {
		t.Fatal("unsupported private restore changed terminal modes")
	}
	if reply.String() != "\x1b[I" {
		t.Errorf("private restore produced an extra focus report: %q", reply.String())
	}
	if err := rs.Update(term); err != nil {
		t.Fatal(err)
	}
	if c := rs.Cursor(); c.Style != CursorBar || !c.Blinking {
		t.Errorf("private restore changed appearance: %+v", c)
	}
}

package vt

import (
	"bytes"
	"testing"
)

func TestOShellFocusReports(t *testing.T) {
	for _, chunk := range []int{1, 7, 4096} {
		var reply bytes.Buffer
		term, _ := resetTerminal(t, Effects{WritePTY: func(p []byte) { reply.Write(p) }})
		check := func(want string) {
			t.Helper()
			if reply.String() != want {
				t.Errorf("chunk=%d got=%q want=%q", chunk, reply.String(), want)
			}
			reply.Reset()
		}
		write := func(s string) { writeControlChunks(term, []byte(s), chunk) }
		write("\x1b[?1004h")
		check("\x1b[I") // Swift's raw terminal starts focused.
		write("\x1b[?1004h\x1b[6n\x1b[?1004l\x1b[?1004h")
		check("\x1b[I\x1b[1;1R\x1b[I") // each enable replies at its position in the input
		term.SetFocus(false)
		check("\x1b[O")
		term.SetFocus(false)
		check("")
		write("\x1b[?1004l")
		term.SetFocus(true)
		check("")
		write("\x1b G\x1b[?1004h")
		check("\x9bI")
		term.SetFocus(false)
		check("\x9bO")
		write("\x1b[!p\x1b[?1004h")
		check("\x9bO") // DECSTR preserves both host focus and reporting.
		write("\x1bc\x1b[?1004h")
		check("\x1b[O") // RIS resets response encoding, not host focus.
		term.SetFocus(true)
		check("\x1b[I")
	}
}

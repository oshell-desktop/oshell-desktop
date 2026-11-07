package terminal

import (
	"strings"
	"testing"
)

// SF-I04: Windows CRLF is one line break, including in large clipboard text.
func TestPasteNormalizesLineEndings(t *testing.T) {
	loadLib(t)
	for _, bracketed := range []bool{false, true} {
		mode := "plain"
		if bracketed {
			mode = "bracketed"
		}
		for _, tc := range []struct{ name, input, normalized string }{
			{"lf", "first\nsecond\n", "first\nsecond\n"},
			{"crlf", "first\r\nsecond\r\n", "first\nsecond\n"},
			{"cr", "first\rsecond\r", "first\nsecond\n"},
			{"mixed-and-blank", "  中文\r\n\r\n\tsecond\rthird\nlast  ", "  中文\n\n\tsecond\nthird\nlast  "},
			{"large-crlf", strings.Repeat("中文 clipboard line\r\n", 5000), strings.Repeat("中文 clipboard line\n", 5000)},
		} {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				conn := newPipe()
				term, err := New(Options{Conn: conn, NoBlink: true})
				if err != nil {
					t.Fatal(err)
				}
				defer term.Close()
				want := strings.ReplaceAll(tc.normalized, "\n", "\r")
				if bracketed {
					term.Feed([]byte("\x1b[?2004h"))
					want = "\x1b[200~" + tc.normalized + "\x1b[201~"
				}
				term.ConfirmedPaste(tc.input)
				term.Send([]byte("<barrier>"))
				want += "<barrier>"
				got := conn.take(len(want))
				if got != want {
					t.Fatalf("paste bytes differ: got %d bytes (%d CR, %d LF), want %d bytes (%d CR, %d LF)", len(got), strings.Count(got, "\r"), strings.Count(got, "\n"), len(want), strings.Count(want, "\r"), strings.Count(want, "\n"))
				}
			})
		}
	}
}

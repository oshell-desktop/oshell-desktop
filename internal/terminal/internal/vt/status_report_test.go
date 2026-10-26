package vt

import (
	"bytes"
	"testing"
)

// Expectations follow SwiftTerm Terminal.cmdDeviceStatus and
// cmdSendDeviceAttributes with OShell's default xterm-256color options.
func TestOShellStatusReports(t *testing.T) {
	for _, chunk := range []int{1, 2, 4096} {
		var reply bytes.Buffer
		term, _ := resetTerminal(t, Effects{WritePTY: func(b []byte) { reply.Write(b) }})
		for _, tc := range []struct{ request, want string }{
			{"\x1b[c", "\x1b[?65;1;2;6;21;22;17;28c"},
			{"\x1b[>c", "\x1b[>65;20;1c"},
			{"\x1b[0;99c", "\x1b[?65;1;2;6;21;22;17;28c"},
			{"\x1b[1c\x1b[>1c\x1b[=c", ""},
			{"\x1b[5;99n", "\x1b[0n"},
			{"\x1b[3;4H\x1b[?6n", "\x1b[?3;4;1R"},
			{"\x1b[?15n\x1b[?25n\x1b[?26n", "\x1b[?10n\x1b[?21n\x1b[?27;1;0;0n"},
			{"\x1b[?53n\x1b[?55n\x1b[?56n", "\x1b[?53n\x1b[?57;1n"},
			{"\x1b[?62n\x1b[?63n\x1b[?63;42;99n", "\x1b[0*{\x1bP0!~0000\x1b\\\x1bP42!~0000\x1b\\"},
			{"\x1b[?75n\x1b[?85n\x1b[?999n", "\x1b[?70n\x1b[?83n"},
			{"\x1b G\x1b[c\x1b[>c\x1b[?63;7n", "\x9b?65;1;2;6;21;22;17;28c\x9b>65;20;1c\x907!~0000\x9c"},
		} {
			reply.Reset()
			writeControlChunks(term, []byte(tc.request), chunk)
			if got := reply.String(); got != tc.want {
				t.Errorf("chunk=%d %q => %q want=%q", chunk, tc.request, got, tc.want)
			}
		}
	}
}

func TestOShellStatusCursorBoundaries(t *testing.T) {
	for _, tc := range []struct{ setup, want string }{
		{"abcdefghijkl", "\x1b[1;13R\x1b[?1;13;1R"},
		{"\x1b[3;5r\x1b[?6h\x1b[1d", "\x1b[1;1R\x1b[?-1;1;1R"},
		{"\x1b[?69h\x1b[3;9s\x1b[2;4r\x1b[?6h\x1b[2;2H", "\x1b[2;2R\x1b[?2;2;1R"},
		// Normal CPR subtracts saved left margin even when margin mode is off;
		// private CPR only subtracts it when both margin and origin modes apply.
		{"\x1b[?69h\x1b[3;9s\x1b[?69l\x1b[2;4r\x1b[?6h\x1b[2;6H", "\x1b[2;4R\x1b[?2;6;1R"},
	} {
		var reply bytes.Buffer
		term, _ := resetTerminal(t, Effects{WritePTY: func(b []byte) { reply.Write(b) }})
		term.Write([]byte(tc.setup))
		before := term.Text()
		x, y := term.Cursor()
		writeControlChunks(term, []byte("\x1b[6n\x1b[?6n"), 1)
		if reply.String() != tc.want {
			t.Errorf("%q => %q want=%q", tc.setup, reply.String(), tc.want)
		}
		if a, b := term.Cursor(); a != x || b != y || term.Text() != before {
			t.Fatal("query changed terminal content or cursor")
		}
		if tc.setup == "abcdefghijkl" {
			term.Write([]byte("X"))
			if a, b := term.Cursor(); a != 1 || b != 1 {
				t.Fatal("query consumed pending wrap", a, b)
			}
		}
	}
}

func TestOShellStatusStringMarginsAndStyle(t *testing.T) {
	var reply bytes.Buffer
	term, _ := resetTerminal(t, Effects{WritePTY: func(b []byte) { reply.Write(b) }})
	term.Write([]byte("\x1b[?69h\x1b[3;9s\x1b[?69l\x1b[2;4r\x1b[6 q\x1bP$qs\x1b\\\x1bP$qr\x1b\\\x1bP$q q\x1b\\"))
	want := "\x1bP1$r3;9s\x1b\\\x1bP1$r2;4r\x1b\\\x1bP1$r2 q\x1b\\"
	if reply.String() != want {
		t.Fatalf("status strings=%q want=%q", reply.String(), want)
	}
}

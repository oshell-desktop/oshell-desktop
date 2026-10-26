package vt

import (
	"bytes"
	"strings"
	"testing"
)

// OShell's MacTerminalView.isProcessTrusted returns true, so the production
// query sums Unicode scalars rather than returning the untrusted zero response.
func TestOShellRectangleChecksum(t *testing.T) {
	for _, chunk := range []int{1, 2, 4096} {
		var reply bytes.Buffer
		term, _ := resetTerminal(t, Effects{WritePTY: func(b []byte) { reply.Write(b) }})
		term.Write([]byte("ABC\x1b[2;1H界e\u0301🙂\x1b[5;12HZ"))
		for _, tc := range []struct{ query, want string }{
			{"\x1b[7;1;1;1;1;3*y", "\x1bP7!~00c6\x1b\\"},
			{"\x1b[y\x1b[7;1*y", "\x1bP0!~0000\x1b\\\x1bP7!~0000\x1b\\"},
			{"\x1b[8;0;2;1;2;5*y", "\x1bP8!~26f34\x1b\\"},
			{"\x1b[9;99;2;2;2;2*y", "\x1bP9!~0020\x1b\\"},
			{"\x1b[10;1;1;1;0;0*y", "\x1bP10!~0000\x1b\\"},
			{"\x1b[11;1;5;12;65535;65535*y", "\x1bP11!~005a\x1b\\"},
			{"\x1b[12;1;65535;1;65535;12*y", "\x1bP12!~0000\x1b\\"},
			{"\x1b[13;1;1;1;1;3y", "\x1bP13!~00c6\x1b\\"},
			{"\x1b[?14;1;1;1;1;3$y", "\x1bP14!~00c6\x1b\\"},
			{"\x1b[15:1;1:1;1:3*y", "\x1bP15!~00c6\x1b\\"},
			{"\x1b[99999999999999999999;1;1;1;1;3*y", "\x1bP65535!~00c6\x1b\\"},
			{"\x1b[16;1;1;1;1;*y", "\x1bP16!~0000\x1b\\"},
			{"\x1b[17;1;1;1;1;3;99*y", "\x1bP17!~00c6\x1b\\"},
			{"\x1b G\x1b[18;1;1;1;1;3*y\x1b F", "\x9018!~00c6\x9c"},
		} {
			reply.Reset()
			before := term.Text()
			x, y := term.Cursor()
			writeControlChunks(term, []byte(tc.query), chunk)
			if got := reply.String(); got != tc.want {
				t.Errorf("chunk %d %q: %q want %q", chunk, tc.query, got, tc.want)
			}
			if a, b := term.Cursor(); a != x || b != y || term.Text() != before {
				t.Fatal("query changed content/cursor")
			}
		}
		// A missing final rectangle parameter defaults to the page; an explicit
		// trailing zero does not. This page contains 53 spaces and the scalars above.
		reply.Reset()
		writeControlChunks(term, []byte("\x1b[19;1;*y"), chunk)
		if got, want := reply.String(), "\x1bP19!~276b4\x1b\\"; got != want {
			t.Errorf("default rectangle: %q want %q", got, want)
		}
		term.Write([]byte("X"))
		if x, y := term.Cursor(); x != 1 || y != 4 {
			t.Fatal("query consumed pending wrap", x, y)
		}
	}
}

func TestOShellRectangleChecksumEmptyAndOverflow(t *testing.T) {
	var reply bytes.Buffer
	term, _ := resetTerminal(t, Effects{WritePTY: func(b []byte) { reply.Write(b) }})
	term.Write([]byte("\x1b[0;0;0*y"))
	if reply.String() != "\x1bP0!~0780\x1b\\" {
		t.Fatal("empty cells", reply.String())
	}
	reply.Reset()
	term.Write([]byte("\x1b[" + strings.Repeat("1;", 24) + "1*y"))
	if reply.Len() != 0 {
		t.Fatal("over-budget parameter list was dispatched")
	}
}

func TestOShellRectangleChecksumScreenAndOrigin(t *testing.T) {
	var reply bytes.Buffer
	term, _ := resetTerminal(t, Effects{WritePTY: func(b []byte) { reply.Write(b) }})
	check := func(query, want string) {
		t.Helper()
		reply.Reset()
		before, bar := term.Text(), term.Scrollbar()
		term.Write([]byte(query))
		if reply.String() != "\x1bP1!~"+want+"\x1b\\" {
			t.Fatalf("%q: %q want %s", query, reply.String(), want)
		}
		if term.Text() != before || term.Scrollbar() != bar {
			t.Fatal("query changed screen or viewport")
		}
	}
	term.Write([]byte(strings.Repeat("history\r\n", 12) + "\x1b[2J\x1b[2;3HABC\x1b[5;12HZ"))
	term.ScrollToTop()
	check("\x1b[1;1;2;3;2;5*y", "00c6")
	term.ScrollToBottom()
	term.Write([]byte("\x1b[?69h\x1b[3;9s\x1b[2;4r\x1b[?6h"))
	check("\x1b[1;1;1;1;1;3*y", "00c6")
	// SwiftTerm applies saved left margins even after disabling margin mode.
	term.Write([]byte("\x1b[?69l"))
	check("\x1b[1;1;1;1;1;3*y", "00c6")
	// Validate before clamping to the page, not to the scrolling region.
	check("\x1b[1;1;5;12;5;12*y", "005a")
	check("\x1b[1;1;6;12;5;12*y", "0000")
	term.Write([]byte("\x1b[?6l\x1b[?47h\x1b[HDEF"))
	check("\x1b[1;99;1;1;1;3*y", "00cf")
	term.Write([]byte("\x1b[?2026h\x1b[HGHI"))
	check("\x1b[1;1;1;1;1;3*y", "00d8")
	term.Write([]byte("\x1b[?2026l\x1b[?47l"))
	check("\x1b[1;1;2;3;2;5*y", "00c6")
}

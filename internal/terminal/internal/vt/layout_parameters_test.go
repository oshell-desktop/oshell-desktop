package vt

import (
	"fmt"
	"strings"
	"testing"
)

func TestOShellHorizontalMarginParameterPresence(t *testing.T) {
	for _, tc := range []struct {
		parameters  string
		left, right int
	}{
		{"", 0, 11}, {"3", 2, 11}, {"3;0", 0, 0}, {"3;", 0, 0},
		{"0;0", 0, 0}, {"4;4", 3, 3}, {"9;4", 3, 3},
		{"999;999", 11, 11}, {"3;9;7", 2, 8},
	} {
		t.Run(tc.parameters, func(t *testing.T) {
			term, _ := resetTerminal(t, Effects{})
			term.Write([]byte("\x1b[?69h\x1b[3;6H\x1b[" + tc.parameters + "s"))
			layoutCursor(t, term, "", 5, 2)
			layoutCursor(t, term, "\x1b[?6h\x1b[H", tc.left, 0)
			layoutCursor(t, term, "\x1b[99;99H\t", tc.right, 4)
		})
	}
}

func TestOShellParameterizedCursorSaveWithoutMarginMode(t *testing.T) {
	for _, parameters := range []string{"3", "3;0", "3;4;5"} {
		term, _ := resetTerminal(t, Effects{})
		layoutCursor(t, term, "\x1b[3;6H\x1b["+parameters+"s\x1b[H\x1b8", 5, 2)
	}
}

func TestOShellInvalidScrollRegionStillHomesCursor(t *testing.T) {
	for _, origin := range []bool{false, true} {
		t.Run(fmt.Sprint(origin), func(t *testing.T) {
			term, _ := resetTerminal(t, Effects{})
			term.Write([]byte("\x1b[2;4r"))
			wantY := 0
			if origin {
				term.Write([]byte("\x1b[?6h"))
				wantY = 1
			}
			layoutCursor(t, term, "\x1b[2;6H\x1b[4;2r", 0, wantY)
			layoutCursor(t, term, "\x1b[2;6H\x1b[99;99r", 0, wantY)
			term.Write([]byte("\x1b[2;5;9r\x1b[?6h"))
			layoutCursor(t, term, "\x1b[99;99H", 11, 4)
		})
	}
}

func TestOShellTabParameterAndMarginEdges(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	term.Write([]byte("\x1b[3g\x1b[1;4H\x1bH\x1b[1;9H\x1bH"))
	layoutCursor(t, term, "\x1b[H\x1b[0I", 3, 0)
	layoutCursor(t, term, "\x1b[1;9H\x1b[0Z", 3, 0)
	layoutCursor(t, term, "\x1b[H\x1b[1;9I", 3, 0)
	term.Write([]byte("\x1b[?69h\x1b[5;10s\x1b[?6h"))
	layoutCursor(t, term, "\x1b[1;5H\x1b[Z", 3, 0)
	layoutCursor(t, term, "\x1b[?6l\x1b[1;12H\t", 9, 0)
}

func TestOShellFragmentedSingleColumnMarginWrap(t *testing.T) {
	for _, fragmented := range []bool{false, true} {
		t.Run(fmt.Sprint(fragmented), func(t *testing.T) {
			term, _ := resetTerminal(t, Effects{})
			// Swift's DECOM changes the coordinate origin without moving the
			// cursor. Position explicitly before exercising single-column wrap.
			data := []byte("\x1b[?69h\x1b[4;4s\x1b[?6h\x1b[HABC")
			if fragmented {
				for _, b := range data {
					term.Write([]byte{b})
				}
			} else {
				term.Write(data)
			}
			if text := term.FormattedText(false); text != "   A\n   B\n   C" {
				t.Fatalf("single-column wrap: %q", text)
			}
			layoutCursor(t, term, "\x1b[?6l\x1b[?69l\x1b[H\t", 8, 0)
		})
	}
}

func TestOShellTabClearsPendingWrapAtLastColumn(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	term.Write([]byte("\x1b[3g\x1b[1;9H\x1bH\x1b[1;12H\x1bH\x1b[Habcdefghijkl"))
	layoutCursor(t, term, "\x1b[Z", 11, 0)
	layoutCursor(t, term, "\x1b[Z", 8, 0)
	term.Write([]byte("\x1b[Habcdefghijkl\tZ"))
	if text := strings.TrimSpace(term.Text()); text != "abcdefghijkZ" {
		t.Fatalf("tab did not clear pending wrap: %q", text)
	}
}

func TestOShellAbsoluteCursorCanPassRightMargin(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	term.Write([]byte("\x1b[?69h\x1b[3;9s\x1b[2;4r\x1b[?6h"))
	layoutCursor(t, term, "\x1b[99;99H", 11, 3)
	layoutCursor(t, term, "\t", 8, 3)
	layoutCursor(t, term, "\x1b[1;99f", 11, 1)
}

func TestOShellTrailingMarginParameterAcrossReads(t *testing.T) {
	for _, chunk := range []int{1, 2, 7} {
		term, _ := resetTerminal(t, Effects{})
		data := []byte("\x1b[?69h\x1b[3;s\x1b[?6h\x1b[H")
		for len(data) > 0 {
			n := min(chunk, len(data))
			term.Write(data[:n])
			data = data[n:]
		}
		layoutCursor(t, term, "", 0, 0)
	}
}

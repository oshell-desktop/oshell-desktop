package vt

import (
	"fmt"
	"strings"
	"testing"
)

func TestOShellCursorCommandCoordinateRules(t *testing.T) {
	for _, tc := range []struct {
		command string
		x, y    int
	}{
		{"2G", 3, 2}, {"2`", 1, 2}, {"2d", 4, 1},
		{"2a", 6, 2}, {"2e", 4, 4}, {"99a", 11, 2}, {"99e", 4, 4},
		{"99;99f", 11, 4}, {"99;99H", 11, 3},
		{"2;2;99H", 3, 2}, {"2;2;99f", 3, 2},
		{"2;99A", 4, 1}, {"2;99B", 4, 3}, {"2;99C", 6, 2}, {"99;99D", 2, 2},
		{"0a", 5, 2}, {"0e", 4, 3}, {"0G", 2, 2}, {"0`", 0, 2}, {"0d", 4, 0},
		{"G", 2, 2}, {"`", 0, 2}, {"d", 4, 0},
	} {
		for _, chunk := range []int{1, 2, 4096} {
			t.Run(fmt.Sprintf("%s/%d", tc.command, chunk), func(t *testing.T) {
				term, _ := resetTerminal(t, Effects{})
				term.Write([]byte("\x1b[?69h\x1b[3;9s\x1b[2;4r\x1b[?6h\x1b[2;3H"))
				writeControlChunks(term, []byte("\x1b["+tc.command), chunk)
				layoutCursor(t, term, "", tc.x, tc.y)
			})
		}
	}
}

func TestOShellCursorBackwardRespectsMarginWithoutReverseWrapping(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		term, _ := resetTerminal(t, Effects{})
		term.Write([]byte("\x1b[?69h\x1b[3;9s"))
		if reverse {
			term.Write([]byte("\x1b[?45h"))
		}
		layoutCursor(t, term, "\x1b[3;6H\x1b[99D", 2, 2)
		layoutCursor(t, term, "\x1b[1D", 2, 2)
		layoutCursor(t, term, "\x1b[3;2H\x1b[99D", 0, 2)
	}
}

func TestOShellCursorLineCommandsRetainLogicalLeftMargin(t *testing.T) {
	for _, mode := range []string{"", "\x1b[?69l"} {
		term, _ := resetTerminal(t, Effects{})
		term.Write([]byte("\x1b[?69h\x1b[3;9s" + mode))
		layoutCursor(t, term, "\x1b[3;1H\x1b[0E", 2, 3)
		layoutCursor(t, term, "\x1b[3;1H\x1b[0F", 2, 1)
	}
}

func TestOShellCursorUpAndVPARetainPendingWrap(t *testing.T) {
	for _, command := range []string{"A", "1d"} {
		term, _ := resetTerminal(t, Effects{})
		term.Write([]byte("\x1b[2;1Habcdefghijkl\x1b[" + command))
		layoutCursor(t, term, "Z", 1, 1)
	}
	// CUB consumes SwiftTerm's logical column after the last printed cell.
	term, _ := resetTerminal(t, Effects{})
	term.Write([]byte("abcdefghijkl"))
	layoutCursor(t, term, "\x1b[D", 11, 0)
	layoutCursor(t, term, "Z", 11, 0)
}

func TestOShellUnsupportedCursorAliasesDoNotMove(t *testing.T) {
	for _, command := range []string{"2j", "2k"} {
		term, _ := resetTerminal(t, Effects{})
		layoutCursor(t, term, "\x1b[3;6H\x1b["+command, 5, 2)
	}
}

func TestOShellCursorLimitsInsideAndOutsideMargins(t *testing.T) {
	for _, tc := range []struct {
		start, move string
		x, y        int
	}{
		{"1;2", "99A", 1, 0}, {"3;2", "99A", 1, 1},
		{"3;2", "99B", 1, 3}, {"5;2", "99B", 1, 4},
		{"3;6", "99C", 8, 2}, {"3;10", "99C", 11, 2},
		{"3;6", "99D", 2, 2}, {"3;2", "99D", 0, 2},
	} {
		term, _ := resetTerminal(t, Effects{})
		term.Write([]byte("\x1b[?69h\x1b[3;9s\x1b[2;4r"))
		layoutCursor(t, term, "\x1b["+tc.start+"H\x1b["+tc.move, tc.x, tc.y)
	}
}

func TestOShellCursorParameterBoundsAndRecovery(t *testing.T) {
	for _, chunk := range []int{1, 3, 4096} {
		term, _ := resetTerminal(t, Effects{})
		term.Write([]byte("\x1b[3;6H"))
		writeControlChunks(term, []byte("\x1b[2"+strings.Repeat(";0", 23)+"C"), chunk)
		layoutCursor(t, term, "", 7, 2)
		writeControlChunks(term, []byte("\x1b[2"+strings.Repeat(";0", 24)+"C"), chunk)
		layoutCursor(t, term, "", 7, 2)
		writeControlChunks(term, []byte("\x1b[999999999999999999999D"), chunk)
		layoutCursor(t, term, "", 0, 2)
		// Swift ignores collectors for these cursor handlers, including ?.
		layoutCursor(t, term, "\x1b[?2C", 2, 2)
	}
}

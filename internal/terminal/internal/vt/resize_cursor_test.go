package vt

import (
	"strings"
	"testing"
)

func TestOShellResizeLeavesCursorParagraph(t *testing.T) {
	for _, tc := range []struct {
		cols int
		want []string
		x, y int
	}{
		{6, []string{"ABCD", "EFG"}, 3, 1},
		{2, []string{"AB", "EF"}, 1, 1},
	} {
		term, _ := resetTerminal(t, Effects{})
		term.Resize(4, 5, 8, 16)
		term.Write([]byte("ABCDEFG"))
		if err := term.Resize(tc.cols, 5, 8, 16); err != nil {
			t.Fatal(err)
		}
		for y, want := range tc.want {
			text, _ := term.RowText(y)
			if strings.TrimRight(string(text), " ") != want {
				t.Fatalf("cols %d row %d: %q want %q", tc.cols, y, string(text), want)
			}
		}
		if x, y := term.Cursor(); x != tc.x || y != tc.y {
			t.Fatal("resize moved cursor within frozen paragraph", tc.cols, x, y)
		}
	}
}

func TestOShellResizeClampsPendingWrap(t *testing.T) {
	for _, tc := range []struct {
		cols, rows int
		want       string
	}{
		{16, 5, "abcdefghijklX"},
		{6, 5, "abcdeX"},
		{12, 6, "abcdefghijkX"},
	} {
		term, _ := resetTerminal(t, Effects{})
		term.Write([]byte("abcdefghijkl"))
		term.Resize(tc.cols, tc.rows, 8, 16)
		term.Write([]byte("X"))
		if got := term.FormattedText(false); got != tc.want {
			t.Fatalf("size %dx%d: %q want %q", tc.cols, tc.rows, got, tc.want)
		}
	}
}

func TestOShellResizeStillReflowsOtherParagraphs(t *testing.T) {
	for _, wrap := range []string{"", "\x1b[?7l"} {
		term, _ := resetTerminal(t, Effects{})
		term.Resize(4, 5, 8, 16)
		term.Write([]byte("ABCDEFG\x1b[5;1H" + wrap))
		term.Resize(6, 5, 8, 16)
		for y, want := range []string{"ABCDEF", "G"} {
			text, _ := term.RowText(y)
			if strings.TrimRight(string(text), " ") != want {
				t.Fatal("non-cursor paragraph did not reflow", wrap, y, string(text))
			}
		}
	}
}

func TestOShellResizeSameGridKeepsHoldAndPendingWrap(t *testing.T) {
	var holds []bool
	term, _ := resetTerminal(t, Effects{RenderHold: func(v bool) { holds = append(holds, v) }})
	term.Write([]byte("abcdefghijkl\x1b[?2026h"))
	term.Resize(12, 5, 9, 18)
	if !term.Mode(ModeSyncOutput) || len(holds) != 1 || !holds[0] {
		t.Fatal("same-grid pixel update ended synchronized output")
	}
	term.Write([]byte("\x1b[?2026lX"))
	if got := term.FormattedText(false); got != "abcdefghijkl\nX" {
		t.Fatal("same-grid update consumed pending wrap", got)
	}
}

func TestOShellResizeRetainsWideWrapSpacerStyle(t *testing.T) {
	term, rs := resetTerminal(t, Effects{})
	term.Resize(4, 5, 8, 16)
	term.Write([]byte("\x1b[1;31;44mABC界"))
	term.Resize(6, 5, 8, 16)
	rs.Update(term)
	rs.Rows()
	rs.NextRow()
	rs.RowCells()
	s := rs.CellStyle(3)
	if !s.Bold || s.FG.Index != 1 || s.BG.Index != 4 {
		t.Fatal("wide wrap spacer lost original style", s)
	}
	term.Write([]byte("\x1b[0m\x1b[2J"))
	term.Resize(8, 5, 8, 16)
}

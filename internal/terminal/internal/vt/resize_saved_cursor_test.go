package vt

import (
	"fmt"
	"strings"
	"testing"
)

func TestOShellRestoreSavedPendingWrap(t *testing.T) {
	for _, save := range []string{"\x1b7", "\x1b[s"} {
		term, _ := resetTerminal(t, Effects{})
		term.Write([]byte("abcdefghijkl" + save + "\x1b[2;1H\x1b8X"))
		if got := term.FormattedText(false); got != "abcdefghijkX" {
			t.Fatal("restored pending wrap instead of clamping logical column", save, got)
		}
	}
}

func TestOShellResizeSavedCursorKeepsColumnAndRelativeRow(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	term.Resize(4, 5, 8, 16)
	term.Write([]byte("ABCDEFG\x1b7\x1b[5;1H"))
	term.Resize(8, 5, 8, 16)
	term.Write([]byte("\x1b8"))
	if x, y := term.Cursor(); x != 3 || y != 0 {
		t.Fatal("saved cursor followed text reflow", x, y)
	}
	term.Write([]byte("X"))
	if got := term.FormattedText(false); got != "ABCXEFG" {
		t.Fatal(got)
	}
}

func TestOShellResizeKeepsOutOfViewSavedRow(t *testing.T) {
	for _, screen := range []string{"", "\x1b[?47h"} {
		term, _ := resetTerminal(t, Effects{})
		term.Write([]byte(screen + "\x1b[5;4H\x1b7\x1b[H"))
		term.Resize(12, 2, 8, 16)
		term.Resize(12, 5, 8, 16)
		term.Write([]byte("\x1b8"))
		if x, y := term.Cursor(); x != 3 || y != 4 {
			t.Fatal("resize discarded saved row", screen, x, y)
		}
	}
}

func TestOShellResizeSavedNegativeRowSurvivesRoundTrip(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	term.Resize(4, 5, 8, 16)
	term.Write([]byte("\x1b7ABCDEFG\x1b[5;1H"))
	term.Resize(8, 5, 8, 16)
	term.Resize(4, 5, 8, 16)
	term.Write([]byte("\x1b8"))
	if x, y := term.Cursor(); x != 0 || y != 0 {
		t.Fatal("saved negative row was clamped before restore", x, y)
	}
}

func TestOShellResizeAlternateSavedRowTracksTrim(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	term.Write([]byte("\x1b[?47h\x1b[5;4H\x1b7"))
	term.Resize(12, 2, 8, 16)
	term.Resize(12, 5, 8, 16)
	term.Write([]byte("\x1b8"))
	if x, y := term.Cursor(); x != 3 || y != 1 {
		t.Fatal("alternate saved row missed top trimming", x, y)
	}
}

func TestOShellSavedCursorHistoryCapacityTrim(t *testing.T) {
	for _, alternate := range []bool{false, true} {
		term, _ := resetTerminal(t, Effects{})
		term.Write([]byte(strings.Repeat("h\r\n", 7) + "\x1b7"))
		history := int(term.Scrollbar().Total - term.Scrollbar().Len)
		if history != 3 {
			t.Fatal("unexpected history fixture", history)
		}
		if alternate {
			term.Write([]byte("\x1b[?47h"))
		}
		if err := term.SetHistoryLines(1); err != nil {
			t.Fatal(err)
		}
		if alternate {
			term.Write([]byte("\x1b[?47l"))
		}
		if got := int(term.Scrollbar().Total - term.Scrollbar().Len); got != 1 {
			t.Fatal("capacity change did not trim partial history page", got)
		}
		term.Write([]byte("\x1b8"))
		if x, y := term.Cursor(); x != 0 || y != 2 {
			t.Fatal("history capacity trim lost saved coordinate adjustment", alternate, x, y)
		}
	}
}

func TestOShellSavedCursorED3DoesNotAdjustRow(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	term.Write([]byte(strings.Repeat("h\r\n", 7) + "\x1b7\x1b[3J\x1b[H\x1b8"))
	if x, y := term.Cursor(); x != 0 || y != 4 {
		t.Fatal("ED3 incorrectly used history-capacity semantics", x, y)
	}
}

func TestOShellHistoryCapacityKeepsExactTailAndPayload(t *testing.T) {
	for _, limit := range []int{0, 1, 500} {
		term, _ := resetTerminal(t, Effects{})
		term.Write([]byte("\x1b]8;;https://history.test/\x1b\\\x1b[31m"))
		for i := 0; i < 612; i++ {
			term.Write([]byte(fmt.Sprintf("%03d\r\n", i)))
		}
		term.Write([]byte("\x1b]8;;\x1b\\"))
		if err := term.SetHistoryLines(limit); err != nil {
			t.Fatal(err)
		}
		if got := int(term.Scrollbar().Total - term.Scrollbar().Len); got != limit {
			t.Fatal("wrong retained history", limit, got)
		}
		term.ScrollToTop()
		text, _ := term.RowText(0)
		if want := fmt.Sprintf("%03d", 608-limit); strings.TrimSpace(string(text)) != want {
			t.Fatal("wrong retained first row", limit, string(text), want)
		}
		if a, b, ok := term.HyperlinkRange(0, 0); !ok || a != 0 || b != 3 {
			t.Fatal("partial page trim lost payload", a, b, ok)
		}
	}
}

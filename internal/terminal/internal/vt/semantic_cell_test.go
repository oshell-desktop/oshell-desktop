package vt

import (
	"strings"
	"testing"
)

func requireSemanticCells(t *testing.T, term *Terminal, y int, want ...SemanticContent) {
	t.Helper()
	for x, role := range want {
		if got, valid := term.SemanticContentAt(x, y); !valid || got != role {
			t.Fatalf("cell (%d,%d): role=%v valid=%v, want %v", x, y, got, valid, role)
		}
	}
}

// The pinned Swift insertion funnel stamps new cells; combining updates the
// existing CharData and preserves its role even across an OSC state change.
func TestOShellSemanticCellClassification(t *testing.T) {
	for _, fragmented := range []bool{false, true} {
		term, _ := resetTerminal(t, Effects{})
		data := "a\x1b]133;P\ab\x1b]133;P;k=r\ac\x1b]133;P;k=c\ad\x1b]133;P;k=s\ae\x1b]133;I\af\x1b]133;C\ag"
		if fragmented {
			for i := range len(data) {
				term.Write([]byte(data[i : i+1]))
			}
		} else {
			term.Write([]byte(data))
		}
		requireSemanticCells(t, term, 0, SemanticNone, SemanticPromptInitial, SemanticPromptRight, SemanticPromptContinuation, SemanticPromptSecondary, SemanticInput, SemanticOutput, SemanticNone)
		for _, xy := range [][2]int{{-1, 0}, {12, 0}, {0, -1}, {0, 5}, {0, 1 << 32}} {
			if _, valid := term.SemanticContentAt(xy[0], xy[1]); valid {
				t.Fatalf("accepted %v", xy)
			}
		}
	}
}

func TestOShellSemanticCellCombiningAndWide(t *testing.T) {
	for _, tc := range []struct {
		name, data string
		want       []SemanticContent
	}{
		{"combining", "\x1b]133;B\ae\x1b]133;C\a\u0301x", []SemanticContent{SemanticInput, SemanticOutput}},
		{"wide", "\x1b]133;P;k=s\a界\x1b]133;B\ax", []SemanticContent{SemanticPromptSecondary, SemanticPromptSecondary, SemanticInput}},
		{"expand", "\x1b]133;B\a♥\x1b]133;C\a\ufe0fx", []SemanticContent{SemanticInput, SemanticInput, SemanticOutput}},
		{"shrink", "\x1b]133;B\a⌚\x1b]133;C\a\ufe0ex", []SemanticContent{SemanticInput, SemanticOutput}},
		{"selector-after-selector", "\x1b]133;B\a♥\ufe0f\x1b]133;C\a\ufe0ex", []SemanticContent{SemanticInput, SemanticInput, SemanticOutput}},
		{"right-mark-only", "\x1b]133;B\ax\x1b]133;A;k=r\ay", []SemanticContent{SemanticInput, SemanticInput}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			term, _ := resetTerminal(t, Effects{})
			term.SetModeDefault(ModeGraphemes, true)
			term.Write([]byte(tc.data))
			requireSemanticCells(t, term, 0, tc.want...)
		})
	}
}

func TestOShellSemanticCellEraseResetAndSwitch(t *testing.T) {
	for _, tc := range []struct {
		name, data string
		want       []SemanticContent
	}{
		{"erase", "\x1b]133;B\aabc\x1b[2G\x1b[X", []SemanticContent{SemanticInput, SemanticNone, SemanticInput}},
		{"insert", "\x1b]133;B\aabc\x1b[2G\x1b[@", []SemanticContent{SemanticInput, SemanticNone, SemanticInput, SemanticInput}},
		{"soft-reset", "\x1b]133;B\aab\x1b[!px", []SemanticContent{SemanticInput, SemanticInput, SemanticNone}},
		{"unclassified-switch", "a\x1b[?47h\x1b[?47lx", []SemanticContent{SemanticNone, SemanticNone}},
		{"classified-switch", "\x1b]133;B\aa\x1b[?47h\x1b[?47lx", []SemanticContent{SemanticInput, SemanticOutput}},
		{"saved-cursor", "\x1b]133;B\aa\x1b7\x1b]133;C\ab\x1b8x", []SemanticContent{SemanticInput, SemanticOutput}},
		{"full-reset", "\x1b]133;B\aa\x1bcx", []SemanticContent{SemanticNone, SemanticNone}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			term, _ := resetTerminal(t, Effects{})
			term.Write([]byte(tc.data))
			requireSemanticCells(t, term, 0, tc.want...)
		})
	}
}

func TestOShellSemanticCellStaleEpochAndReflow(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	term.Write([]byte("\x1b]133;A\a\x1b]133;B\ax\r\nyz\x1b[H\x1b]133;N\a\x1b]133;B\an\r\n"))
	requireSemanticCells(t, term, 1, SemanticNone, SemanticNone)
	term.Write([]byte("\x1bc\x1b]133;B\aabcdefghijklmnop\x1b]133;C\a!\r\nz"))
	if err := term.Resize(24, 5, 8, 16); err != nil {
		t.Fatal(err)
	}
	for x := range 16 {
		if got, valid := term.SemanticContentAt(x, 0); !valid || got != SemanticInput {
			t.Fatalf("reflow cell %d = %v, %v", x, got, valid)
		}
	}
	if got, _ := term.SemanticContentAt(16, 0); got != SemanticOutput {
		t.Fatalf("output after reflow = %v", got)
	}
}

func TestOShellSemanticCellAlternateOwnsClassification(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	term.Write([]byte("\x1b]133;B\aa\x1b[?47hx"))
	requireSemanticCells(t, term, 0, SemanticNone, SemanticNone)
	term.Write([]byte("\x1b[?47ly"))
	requireSemanticCells(t, term, 0, SemanticInput, SemanticNone, SemanticOutput)
}

func TestOShellSemanticCellHistoryAndMovedRows(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	term.Write([]byte("\x1b]133;P;k=s\a界\x1b]133;B\ax\x1b[H\x1b[L"))
	requireSemanticCells(t, term, 0, SemanticNone, SemanticNone, SemanticNone)
	requireSemanticCells(t, term, 1, SemanticPromptSecondary, SemanticPromptSecondary, SemanticInput)
	term.Write([]byte("\x1b[3;1H\x1b]133;C\a" + strings.Repeat("z\r\n", 15)))
	if term.Scrollbar().Total <= 5 {
		t.Fatal("fixture did not create history")
	}
	for _, row := range []int{0, int(term.Scrollbar().Total) - 5} {
		term.ScrollToRow(row)
		requireSemanticCells(t, term, 1, SemanticPromptSecondary, SemanticPromptSecondary, SemanticInput)
	}
}

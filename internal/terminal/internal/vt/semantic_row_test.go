package vt

import (
	"strings"
	"testing"
)

// Expectations follow Buffer.semanticRowKind at the pinned Swift source.
// These exercise the production parser, page storage and C query together.
func TestOShellSemanticDerivedRows(t *testing.T) {
	for _, tc := range []struct {
		name, data string
		want       []SemanticRowKind
	}{
		{"unallocated-right", "\x1b]133;A;k=r\a", []SemanticRowKind{SemanticRowNone}},
		{"soft-wrap", "\x1b]133;A\a\x1b]133;B\a1234567890123", []SemanticRowKind{SemanticRowInitial, SemanticRowContinuation}},
		{"hard-input", "\x1b]133;A\a\x1b]133;B\ax\r\ny", []SemanticRowKind{SemanticRowInitial, SemanticRowContinuation}},
		{"stranded-epoch", "\x1b]133;A\a\x1b]133;B\ax\r\ny\x1b[H\x1b]133;N\a", []SemanticRowKind{SemanticRowInitial, SemanticRowNone}},
		{"completed-secondary", "\x1b]133;A\a\x1b]133;B\ax\r\n\x1b]133;P;k=s\ay\x1b]133;C\a\x1b[3;1H\x1b]133;N\a", []SemanticRowKind{SemanticRowInitial, SemanticRowContinuation, SemanticRowInitial}},
		{"completed-right", "\x1b]133;A\a\x1b[2;1H\x1b]133;P;k=r\a\x1b]133;D\a\x1b[3;1H\x1b]133;N\a", []SemanticRowKind{SemanticRowInitial, SemanticRowContinuation, SemanticRowInitial}},
		{"secondary-origin", "\x1b]133;N;k=s\a", []SemanticRowKind{SemanticRowInitial}},
		{"old-secondary-origin", "\x1b]133;N;k=s\a\x1b[2;1H\x1b]133;N\a", []SemanticRowKind{SemanticRowContinuation, SemanticRowInitial}},
		{"erased-opening", "\x1b]133;A\aprompt\x1b[2K\x1b]133;C\a", []SemanticRowKind{SemanticRowInitial}},
		{"alternate", "\x1b]133;A\a\x1b[?47h\x1b]133;A\a", []SemanticRowKind{SemanticRowNone}},
		{"reset", "\x1b]133;A\a\x1b[!p", []SemanticRowKind{SemanticRowNone}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			term, _ := resetTerminal(t, Effects{})
			for i := range len(tc.data) {
				term.Write([]byte(tc.data[i : i+1]))
			}
			for y, want := range tc.want {
				if got, valid := term.SemanticRow(y); !valid || got != want {
					t.Fatalf("row %d: got %v (valid %v), want %v", y, got, valid, want)
				}
			}
			for _, invalid := range []int{-1, int(term.Scrollbar().Total), 1 << 32} {
				if _, valid := term.SemanticRow(invalid); valid {
					t.Fatalf("accepted invalid row %d", invalid)
				}
			}
		})
	}
}

func TestOShellSemanticRowsUseAbsoluteHistory(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	term.Write([]byte("\x1b]133;A\a\x1b]133;B\a" + strings.Repeat("a\r\n", 30) + "\x1b]133;C\a"))
	if term.Scrollbar().Total <= 5 {
		t.Fatal("fixture did not create history")
	}
	check := func() {
		t.Helper()
		for y, want := range map[int]SemanticRowKind{0: SemanticRowInitial, 1: SemanticRowContinuation, 29: SemanticRowContinuation, 30: SemanticRowNone} {
			if got, valid := term.SemanticRow(y); !valid || got != want {
				t.Fatalf("history row %d = %v (valid %v), want %v", y, got, valid, want)
			}
		}
	}
	check()
	term.ScrollToTop()
	check()
	term.ScrollToBottom()
	if err := term.Resize(6, 5, 8, 16); err != nil {
		t.Fatal(err)
	}
	check()
	term.Write([]byte("\x1b[?47h"))
	if kind, valid := term.SemanticRow(0); !valid || kind != SemanticRowNone {
		t.Fatal("alternate query leaked primary history", kind, valid)
	}
	term.Write([]byte("\x1b[?47l"))
	check()
}

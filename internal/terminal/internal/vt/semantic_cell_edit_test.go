package vt

import (
	"fmt"
	"testing"
)

// Swift creates new CharData for DECFRA/DECERA/DECALN, but updates existing
// CharData for DECSERA. That distinction affects semantic click offsets even
// when the visible result of both erase commands is the same blank space.
func TestOShellSemanticCellRectangleEdits(t *testing.T) {
	for _, tc := range []struct {
		name, command string
		want          SemanticContent
	}{
		{"fill", "\x1b[90;1;2;1;2$x", SemanticNone},
		{"erase", "\x1b[1;2;1;2$z", SemanticNone},
		{"selective-erase", "\x1b[1;2;1;2${", SemanticInput},
	} {
		for _, chunk := range []int{1, 4096} {
			t.Run(fmt.Sprintf("%s/%d", tc.name, chunk), func(t *testing.T) {
				term, _ := resetTerminal(t, Effects{})
				term.Write([]byte("\x1b]133;A;cl=m\a\x1b]133;B\aabc"))
				writeControlChunks(term, []byte(tc.command), chunk)
				requireSemanticCells(t, term, 0, SemanticInput, tc.want, SemanticInput)
				wantClick := "\x1b[D\x1b[D"
				if tc.want == SemanticInput {
					wantClick += "\x1b[D"
				}
				if got := string(term.SemanticClickData(0, 0)); got != wantClick {
					t.Fatalf("click after rectangle edit = %q, want %q", got, wantClick)
				}
				term.Write([]byte("d"))
				if got, _ := term.SemanticContentAt(3, 0); got != SemanticInput {
					t.Fatal("rectangle edit changed subsequent classification", got)
				}
			})
		}
	}
}

func TestOShellSemanticCellAlignmentKeepsMarks(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	term.Write([]byte("\x1b]133;A;cl=m\a\x1b]133;B\aabc\x1b#8"))
	for y := range 5 {
		for x := range 12 {
			if got, valid := term.SemanticContentAt(x, y); !valid || got != SemanticNone {
				t.Fatalf("alignment retained role at %d,%d: %v/%v", x, y, got, valid)
			}
		}
	}
	if kind, valid := term.SemanticRow(0); !valid || kind != SemanticRowInitial {
		t.Fatal("alignment lost row mark", kind, valid)
	}
	if got := term.SemanticClickData(1, 0); len(got) != 0 {
		t.Fatalf("alignment retained input offsets: %q", got)
	}
	term.Write([]byte("z"))
	requireSemanticCells(t, term, 0, SemanticInput, SemanticNone)
}

func TestOShellSemanticCellRectangleCopyKeepsDestinationIdentity(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	if err := term.Resize(64, 80, 8, 16); err != nil {
		t.Fatal(err)
	}
	term.SetModeDefault(ModeGraphemes, true)
	term.Write([]byte("\x1b]133;A;cl=m\a\x1b]133;P;k=s\a界\x1b]133;B\ae\u0301\x1b]133;C\az\x1b[60;1H\x1b]133;N;cl=m\a\x1b]133;B\aq"))
	target, valid := term.CaptureSemanticClickTarget(59)
	if !valid {
		t.Fatal("missing destination identity")
	}
	// The owned snapshot must retain roles across pages, without replacing the
	// destination line's authored mark or identity with the source line's.
	term.Write([]byte("\x1b[1;1;1;4;1;60;2;1;0$v\x1b[1;1;1;4$z"))
	requireSemanticCells(t, term, 0, SemanticNone, SemanticNone, SemanticNone, SemanticNone)
	requireSemanticCells(t, term, 59, SemanticInput, SemanticPromptSecondary, SemanticPromptSecondary, SemanticInput, SemanticOutput)
	if y, ok := term.ResolveSemanticClickTarget(target); !ok || y != 59 {
		t.Fatal("copy replaced destination identity", y, ok)
	}
	if kind, ok := term.SemanticRow(59); !ok || kind != SemanticRowInitial {
		t.Fatal("copy replaced destination mark", kind, ok)
	}
}

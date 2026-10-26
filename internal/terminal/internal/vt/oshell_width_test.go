package vt

import (
	"os"
	"testing"
)

func TestOShellAmbiguousWidth(t *testing.T) {
	loadLib(t)
	if oshellABI < 1 || oshellABI > 62 {
		if os.Getenv("OSHELL_REQUIRE_WIDTH_EXTENSION") == "1" {
			t.Fatal("required OShell width extension is absent")
		}
		t.Skip("requires OShell native extension")
	}
	for _, tc := range []struct {
		name, text   string
		narrow, wide int
	}{
		{"latin ambiguous", "·α─", 3, 6},
		{"ASCII and CJK", "A中", 3, 3},
		{"combining", "a\u0301", 1, 1},
		{"emoji", "😀", 2, 2},
		{"ambiguous text selector", "®\ufe0e", 1, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, wide := range []bool{false, true} {
				term, e := NewTerminal(40, 4, Effects{})
				if e != nil {
					t.Fatal(e)
				}
				term.SetModeDefault(ModeGraphemes, true)
				if e = term.SetAmbiguousWidth(wide); e != nil {
					t.Fatal(e)
				}
				term.Write([]byte(tc.text))
				x, y := term.Cursor()
				want := tc.narrow
				if wide {
					want = tc.wide
				}
				if x != want || y != 0 {
					t.Errorf("wide=%v cursor=(%d,%d), want (%d,0)", wide, x, y, want)
				}
				term.Reset()
				term.Write([]byte(tc.text))
				x, y = term.Cursor()
				if x != want || y != 0 {
					t.Errorf("policy lost after reset: wide=%v cursor=(%d,%d)", wide, x, y)
				}
				term.Free()
			}
		})
	}
	term, e := NewTerminal(5, 4, Effects{})
	if e != nil {
		t.Fatal(e)
	}
	defer term.Free()
	if e = term.SetAmbiguousWidth(true); e != nil {
		t.Fatal(e)
	}
	term.Write([]byte("abc·Z"))
	x, y := term.Cursor()
	if x != 1 || y != 1 {
		t.Fatalf("wrap cursor %d,%d", x, y)
	}
	if got := term.Text(); got != "abc·Z" {
		t.Fatalf("synthetic padding leaked to copied text: %q", got)
	}
}

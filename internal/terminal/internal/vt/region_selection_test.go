package vt

import (
	"fmt"
	"strings"
	"testing"
)

func selectionRegionFixture(t *testing.T, modes string) *Terminal {
	term, _ := resetTerminal(t, Effects{})
	lines := make([]string, 5)
	for i := range lines {
		lines[i] = strings.Repeat(fmt.Sprint(i), 12)
	}
	term.Write([]byte(strings.Join(lines, "\r\n") + "\x1b[2;4r" + modes))
	return term
}

func TestOShellRegionSelectionScroll(t *testing.T) {
	for _, op := range []struct {
		name, seq  string
		kept, lost int
	}{
		{"SU", "\x1b[S", 2, 1}, {"SD", "\x1b[T", 2, 3},
		{"IND", "\x1b[4;1H\x1bD", 2, 1}, {"RI", "\x1b[2;1H\x1bM", 2, 3},
		{"IL", "\x1b[3;1H\x1b[L", 2, 3}, {"DL", "\x1b[3;1H\x1b[M", 3, 2},
	} {
		for _, kind := range []string{"kept", "lost", "crossing", "outside"} {
			t.Run(op.name+"/"+kind, func(t *testing.T) {
				term := selectionRegionFixture(t, "")
				a, b := op.kept, op.kept
				want := strings.Repeat(fmt.Sprint(a), 3)
				keep := true
				switch kind {
				case "lost":
					a, b = op.lost, op.lost
					want = ""
					keep = false
				case "crossing":
					a, b = 0, op.kept
					want = ""
					keep = false
				case "outside":
					a, b = 4, 4
					want = "444"
				}
				term.SelectCells(0, a, 2, b)
				term.Write([]byte(op.seq))
				got, ok := term.SelectionText()
				if got != want || ok != keep {
					t.Fatalf("selection %q/%v want %q/%v", got, ok, want, keep)
				}
			})
		}
	}
}

func TestOShellMarginScrollSelection(t *testing.T) {
	for _, command := range []string{"\x1b[S", "\x1b[T", "\x1b[2;4H\x1b[L", "\x1b[2;4H\x1b[M", "\x1b[4;4H\x1bD", "\x1b[2;4H\x1bM"} {
		for _, kind := range []string{"inside", "outside", "spanning"} {
			t.Run(fmt.Sprintf("%q/%s", command, kind), func(t *testing.T) {
				term := selectionRegionFixture(t, "\x1b[?69h\x1b[3;7s")
				x1, y1, x2, y2 := 3, 2, 5, 2
				want := ""
				keep := false
				if kind == "outside" {
					x1, x2 = 0, 0
					want = "2"
					keep = true
				}
				if kind == "spanning" {
					x1, x2, y1, y2 = 0, 1, 0, 2
				}
				term.SelectCells(x1, y1, x2, y2)
				term.Write([]byte(command))
				got, ok := term.SelectionText()
				if got != want || ok != keep {
					t.Fatalf("selection %q/%v want %q/%v", got, ok, want, keep)
				}
			})
		}
	}
}

func TestOShellRegionSelectionDragFollowsText(t *testing.T) {
	for _, op := range []struct {
		seq      string
		from, to int
	}{
		{"\x1b[S", 2, 1}, {"\x1b[T", 2, 3}, {"\x1b[4;1H\x1bD", 2, 1}, {"\x1b[2;1H\x1bM", 2, 3},
		{"\x1b[3;1H\x1b[L", 2, 3}, {"\x1b[3;1H\x1b[M", 3, 2},
	} {
		t.Run(fmt.Sprintf("%q", op.seq), func(t *testing.T) {
			term := selectionRegionFixture(t, "")
			g, err := NewGesture(500e6, 5)
			if err != nil {
				t.Fatal(err)
			}
			defer g.Free(term)
			geo := Geometry{Columns: 12, CellWidth: 8, Height: 80}
			a, _ := term.CellAt(0, op.from)
			b, _ := term.CellAt(3, op.from)
			g.Press(term, a, 2, float64(op.from*16+5), 1)
			sel, ok := g.Drag(term, b, 26, float64(op.from*16+5), geo, false)
			if !ok {
				t.Fatal("drag fixture")
			}
			term.SetSelection(&sel)
			term.Write([]byte(op.seq))
			next, _ := term.CellAt(5, op.to)
			sel, ok = g.Drag(term, next, 42, float64(op.to*16+5), geo, false)
			if !ok {
				t.Fatal("retained anchor lost")
			}
			term.SetSelection(&sel)
			if got, _ := term.SelectionText(); got != strings.Repeat(fmt.Sprint(op.from), 5) {
				t.Fatal("drag did not follow source text", got)
			}
		})
	}
}

func TestOShellFullMarginSelectionPolicy(t *testing.T) {
	for _, tc := range []struct {
		seq  string
		keep bool
	}{
		{"\x1b[S", false}, {"\x1b[T", false}, {"\x1b[4;1H\x1bD", true}, {"\x1b[2;1H\x1bM", true},
	} {
		term := selectionRegionFixture(t, "\x1b[?69h\x1b[1;12s")
		term.SelectCells(0, 2, 2, 2)
		term.Write([]byte(tc.seq))
		got, ok := term.SelectionText()
		if ok != tc.keep || tc.keep && got != "222" {
			t.Fatal("full-margin explicit/index policy", tc.seq, got, ok)
		}
	}
	term := selectionRegionFixture(t, "\x1b[?69h\x1b[3;7s")
	term.SelectCells(0, 2, 1, 2)
	term.Write([]byte("\x1b[S"))
	if _, ok := term.SelectionText(); ok {
		t.Fatal("exclusive selection endpoint touching margin was kept")
	}
}

package vt

import (
	"fmt"
	"testing"
)

func TestOShellRecycledHistorySelection(t *testing.T) {
	for _, tc := range []struct {
		name           string
		x1, y1, x2, y2 int
		want           string
		keep           bool
	}{
		{"removed", 0, 0, 2, 0, "", false},
		{"partial", 0, 0, 2, 1, "", false},
		{"reverse partial", 2, 1, 0, 0, "", false},
		{"retained", 0, 1, 2, 2, "001\n002", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			term, _ := resetTerminal(t, Effects{})
			term.SetHistoryLines(2)
			for i := 0; i < 6; i++ {
				term.Write([]byte(fmt.Sprintf("%03d\r\n", i)))
			}
			term.ScrollToTop()
			if !term.SelectCells(tc.x1, tc.y1, tc.x2, tc.y2) {
				t.Fatal("selection fixture failed")
			}
			term.Write([]byte("006\r\n"))
			got, ok := term.SelectionText()
			if ok != tc.keep || got != tc.want {
				t.Fatalf("selection after recycling = %q/%v, want %q/%v", got, ok, tc.want, tc.keep)
			}
		})
	}
}

func TestOShellRecycledHistoryCancelsDragAnchor(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		term, _ := resetTerminal(t, Effects{})
		term.SetHistoryLines(2)
		for i := 0; i < 6; i++ {
			term.Write([]byte(fmt.Sprintf("%03d\r\n", i)))
		}
		term.ScrollToTop()
		g, err := NewGesture(500e6, 5)
		if err != nil {
			t.Fatal(err)
		}
		defer g.Free(term)
		geo := Geometry{Columns: 12, CellWidth: 8, Height: 80}
		a, _ := term.CellAt(0, 0)
		b, _ := term.CellAt(3, 1)
		if reverse {
			a, b = b, a
		}
		g.Press(term, a, 2, 5, 1)
		selection, ok := g.Drag(term, b, 26, 21, geo, false)
		if !ok {
			t.Fatal("drag fixture failed")
		}
		term.SetSelection(&selection)
		term.Write([]byte("006\r\n"))
		if g.Anchored(term) {
			t.Fatal("discarded selection kept drag anchor", reverse)
		}
		next, _ := term.CellAt(3, 2)
		if _, ok := g.Drag(term, next, 26, 37, geo, false); ok {
			t.Fatal("drag resurrected invalid selection", reverse)
		}
		g.Press(term, next, 26, 37, 900e6)
		if !g.Anchored(term) {
			t.Fatal("new press did not establish an anchor")
		}
	}
}

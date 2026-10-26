package vt

import (
	"fmt"
	"strings"
	"testing"
)

func endpointGesture(t *testing.T, term *Terminal) *Gesture {
	t.Helper()
	g, err := NewGesture(500e6, 5)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { g.Free(term) })
	return g
}

func TestOShellRowSelectionUsesPhysicalRowAndExclusiveEnd(t *testing.T) {
	for _, row := range []int{0, 1} {
		term, _ := resetTerminal(t, Effects{})
		term.Write([]byte("ABCDEFGHIJKLMNOPQRSTUVWX"))
		g := endpointGesture(t, term)
		ref, _ := term.CellAt(2, row)
		sel, ok := g.PressCount(term, ref, 18, float64(row*16+5), 1, 3)
		if !ok {
			t.Fatal("row selection fixture")
		}
		term.SetSelection(&sel)
		want := "ABCDEFGHIJK"
		if row == 1 {
			want = "MNOPQRSTUVW"
		}
		if got, _ := term.SelectionText(); got != want {
			t.Fatal("row selection copied a wrapped paragraph or final cell", row, got, want)
		}
	}
}

func TestOShellSelectionDirectionSurvivesExplicitTrim(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprint(reverse), func(t *testing.T) {
			term, r := resetTerminal(t, Effects{})
			var lines []string
			for i := 0; i < 7; i++ {
				lines = append(lines, strings.Repeat(fmt.Sprint(i), 12))
			}
			term.Write([]byte(strings.Join(lines, "\r\n")))
			term.ScrollToTop()
			g := endpointGesture(t, term)
			a, b := 0, 11
			if reverse {
				a, b = b, a
			}
			first, _ := term.screenCell(a, 2)
			last, _ := term.screenCell(b, 2)
			g.Press(term, first, float64(a*8+2), 37, 1)
			sel, ok := g.Drag(term, last, float64(b*8+2), 37, Geometry{Columns: 12, CellWidth: 8, Height: 80}, false)
			if !ok {
				t.Fatal("drag fixture")
			}
			term.SetSelection(&sel)
			for _, trim := range []bool{false, true} {
				if trim {
					term.Write([]byte("\x1b[3J"))
				}
				r.Update(term)
				r.Rows()
				for row := 0; r.NextRow(); row++ {
					if row != 2 {
						continue
					}
					start, end, selected := r.RowSelection()
					wantEnd := 11
					if reverse {
						wantEnd = 10
					}
					if !selected || start != 0 || end != wantEnd {
						t.Fatalf("reverse=%v trim=%v highlight %d..%d/%v want 0..%d", reverse, trim, start, end, selected, wantEnd)
					}
				}
			}
		})
	}
}

func TestOShellSelectionBoundaryRowIsNotNormalizedAway(t *testing.T) {
	for _, soft := range []bool{false, true} {
		term, _ := resetTerminal(t, Effects{})
		text := "ABCDEFGHIJKL\r\nMNOPQRSTUVWX"
		if soft {
			text = "ABCDEFGHIJKLMNOPQRSTUVWX"
		}
		term.Write([]byte(text))
		g := endpointGesture(t, term)
		a, _ := term.CellAt(0, 0)
		b, _ := term.CellAt(0, 1)
		g.Press(term, a, 2, 5, 1)
		sel, ok := g.Drag(term, b, 2, 21, Geometry{Columns: 12, CellWidth: 8, Height: 80}, false)
		if !ok {
			t.Fatal("boundary fixture")
		}
		term.SetSelection(&sel)
		want := "ABCDEFGHIJKL\n"
		if soft {
			want = "ABCDEFGHIJKL"
		}
		if got, _ := term.SelectionText(); got != want {
			t.Errorf("soft=%v copy=%q want %q", soft, got, want)
		}
		term.Write([]byte("\x1b[2;4r\x1b[S"))
		if _, ok := term.SelectionText(); ok {
			t.Error("exclusive endpoint crossing scroll region was normalized to prior row", soft)
		}
	}
}

func TestOShellSelectionBoundaryRowContent(t *testing.T) {
	for _, tc := range []struct {
		name, suffix, want string
		endRow             int
	}{
		{"empty", "\r\n", "ABCDEFGHIJKL", 1},
		{"space", "\r\n ", "ABCDEFGHIJKL\n", 1},
		{"styled-empty", "\r\n\x1b[41m\x1b[K", "ABCDEFGHIJKL\n", 1},
		{"soft", "M", "ABCDEFGHIJKL", 1},
		{"interior-empty", "\r\n\r\nM", "ABCDEFGHIJKL\n\n", 2},
		{"interior-styled-empty", "\r\n\x1b[41m\x1b[K\x1b[0m\r\nM", "ABCDEFGHIJKL\n\n", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			term, _ := resetTerminal(t, Effects{})
			term.Write([]byte("ABCDEFGHIJKL" + tc.suffix))
			g := endpointGesture(t, term)
			a, _ := term.CellAt(0, 0)
			b, _ := term.CellAt(0, tc.endRow)
			g.Press(term, a, 2, 5, 1)
			sel, ok := g.Drag(term, b, 2, 21, Geometry{Columns: 12, CellWidth: 8, Height: 80}, false)
			if !ok {
				t.Fatal("drag fixture")
			}
			term.SetSelection(&sel)
			if got, _ := term.SelectionText(); got != tc.want {
				t.Fatalf("copy=%q want %q", got, tc.want)
			}
		})
	}
}

func TestOShellSelectionBoundaryAcrossHistoryPages(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	if err := term.SetHistoryLines(4096); err != nil {
		t.Fatal(err)
	}
	const endRow = 3000
	term.Write([]byte("A" + strings.Repeat("\r\n", endRow) + "M"))
	g := endpointGesture(t, term)
	a, ok := term.screenCell(0, 0)
	if !ok {
		t.Fatal("start row missing")
	}
	b, ok := term.screenCell(0, endRow)
	if !ok {
		t.Fatal("end row missing")
	}
	g.Press(term, a, 2, 5, 1)
	sel, ok := g.Drag(term, b, 2, 69, Geometry{Columns: 12, CellWidth: 8, Height: 80}, false)
	if !ok {
		t.Fatal("drag fixture")
	}
	term.SetSelection(&sel)
	if got, _ := term.SelectionText(); got != "A"+strings.Repeat("\n", endRow) {
		t.Fatalf("copy length=%d, want %d including all pending newlines", len(got), endRow+1)
	}
}

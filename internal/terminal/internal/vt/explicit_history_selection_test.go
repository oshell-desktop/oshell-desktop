package vt

import (
	"fmt"
	"strings"
	"testing"
)

func explicitHistorySelectionFixture(t *testing.T) *Terminal {
	term, _ := resetTerminal(t, Effects{})
	var lines []string
	for i := 0; i < 7; i++ {
		lines = append(lines, strings.Repeat(fmt.Sprint(i), 6))
	}
	term.Write([]byte(strings.Join(lines, "\r\n")))
	return term
}

func TestOShellExplicitHistorySelectionCoordinates(t *testing.T) {
	for _, op := range []string{"capacity", "ED3"} {
		for _, row := range []int{0, 2, 6} {
			t.Run(fmt.Sprintf("%s/row%d", op, row), func(t *testing.T) {
				term := explicitHistorySelectionFixture(t)
				if !term.SelectSearchRange(row, 0, row, 3) {
					t.Fatal("fixture")
				}
				if op == "capacity" {
					if err := term.SetHistoryLines(0); err != nil {
						t.Fatal(err)
					}
				} else {
					term.Write([]byte("\x1b[3J"))
				}
				want := ""
				if row < 5 {
					want = strings.Repeat(fmt.Sprint(row+2), 3)
				}
				if got, active := term.SelectionText(); got != want || !active {
					t.Fatalf("copy=%q/%v want %q/true", got, active, want)
				}
				if row == 6 {
					if a, _, b, _, found := term.SearchSelectionRange(); !found || a != 6 || b != 6 {
						t.Fatal("search lost logical range outside storage", a, b, found)
					}
					// Growing storage makes the original logical range readable.
					term.SetHistoryLines(10)
					term.Write([]byte("\r\nnewA\r\nnewB"))
					if got, _ := term.SelectionText(); got != "new" {
						t.Fatal("out-of-storage coordinates did not reappear", got)
					}
				}
			})
		}
	}
}

func TestOShellExplicitHistoryWordLineAndReset(t *testing.T) {
	for _, count := range []int{2, 3} {
		for _, row := range []int{2, 6} {
			t.Run(fmt.Sprintf("count%d/row%d", count, row), func(t *testing.T) {
				term := explicitHistorySelectionFixture(t)
				g, err := NewGesture(500e6, 5)
				if err != nil {
					t.Fatal(err)
				}
				defer g.Free(term)
				a, _ := term.screenCell(1, row)
				sel, ok := g.PressCount(term, a, 10, 25, 1, count)
				if !ok {
					t.Fatal("seed fixture")
				}
				term.SetSelection(&sel)
				term.Write([]byte("\x1b[3J"))
				b, _ := term.screenCell(2, min(row, 4))
				sel, ok = g.Drag(term, b, 18, 30, Geometry{Columns: 12, CellWidth: 8, Height: 80}, false)
				if !ok {
					t.Fatal("logical seed lost")
				}
				term.SetSelection(&sel)
				want := strings.Repeat(fmt.Sprint(min(row+2, 6)), 6)
				if got, _ := term.SelectionText(); got != want {
					t.Fatal("word/row drag", got, want)
				}
				term.Write([]byte("\x1bc"))
				if g.Anchored(term) {
					t.Fatal("reset retained logical anchor")
				}
				if _, ok := term.SelectionText(); ok {
					t.Fatal("reset retained logical selection")
				}
			})
		}
	}
}

func TestOShellExplicitHistoryRetainsOriginalWordSeed(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	term.Write([]byte("aaa zzz\r\n111\r\nlongword\r\n333\r\n444\r\n555\r\n666"))
	g, err := NewGesture(500e6, 5)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Free(term)
	a, _ := term.screenCell(1, 0)
	sel, ok := g.PressCount(term, a, 10, 25, 1, 2)
	if !ok {
		t.Fatal("word fixture")
	}
	term.SetSelection(&sel)
	term.Write([]byte("\x1b[3J"))
	for _, tc := range []struct {
		col  int
		want string
	}{{2, "lon"}, {5, "longword"}, {1, "lon"}} {
		b, _ := term.screenCell(tc.col, 0)
		sel, ok = g.Drag(term, b, float64(tc.col*8+2), 30, Geometry{Columns: 12, CellWidth: 8, Height: 80}, false)
		if !ok {
			t.Fatal("logical word anchor lost")
		}
		term.SetSelection(&sel)
		if got, _ := term.SelectionText(); got != tc.want {
			t.Fatal("word seed was recomputed from replacement text", got, tc.want)
		}
	}
}

func TestOShellExplicitHistoryCopyAndHighlight(t *testing.T) {
	term := explicitHistorySelectionFixture(t)
	term.SelectSearchRange(2, 1, 6, 3)
	term.Write([]byte("\x1b[3J"))
	if got, _ := term.SelectionText(); got != "44444\n555555\n666" {
		t.Fatal("copy did not clip only the last row", got)
	}
	r, err := NewRenderState()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Free()
	r.Update(term)
	r.Rows()
	for row := 0; r.NextRow(); row++ {
		a, b, ok := r.RowSelection()
		if row < 2 && ok || row == 2 && (!ok || a != 1 || b != 11) || row > 2 && (!ok || a != 0 || b != 11) {
			t.Fatal("highlight was clipped to the copy endpoint", row, a, b, ok)
		}
	}
}

func TestOShellExplicitHistorySelectionDrag(t *testing.T) {
	for _, row := range []int{0, 2, 6} {
		t.Run(fmt.Sprint(row), func(t *testing.T) {
			term := explicitHistorySelectionFixture(t)
			g, err := NewGesture(500e6, 5)
			if err != nil {
				t.Fatal(err)
			}
			defer g.Free(term)
			a, _ := term.screenCell(0, row)
			b, _ := term.screenCell(3, row)
			geo := Geometry{Columns: 12, CellWidth: 8, Height: 80}
			g.Press(term, a, 2, 25, 1)
			sel, ok := g.Drag(term, b, 26, 25, geo, false)
			if !ok {
				t.Fatal("drag fixture")
			}
			term.SetSelection(&sel)
			term.Write([]byte("\x1b[3J"))
			targetRow := min(row, 4)
			target, _ := term.screenCell(5, targetRow)
			sel, ok = g.Drag(term, target, 42, 30, geo, false)
			if !ok {
				t.Fatal("logical drag anchor lost")
			}
			term.SetSelection(&sel)
			want := strings.Repeat(fmt.Sprint(row+2), 5)
			if row == 6 {
				// The reversed logical endpoint clips to row 4, column 0;
				// it does not normalize into the former physical row.
				want = ""
			}
			if got, _ := term.SelectionText(); got != want {
				t.Fatal("drag used recycled storage coordinates", got, want)
			}
		})
	}
}

func TestOShellExplicitSelectionThenRegionAndRecycle(t *testing.T) {
	term := explicitHistorySelectionFixture(t)
	term.SelectSearchRange(2, 0, 2, 3)
	term.Write([]byte("\x1b[3J\x1b[2;4r\x1b[S"))
	if got, _ := term.SelectionText(); got != "444" {
		t.Fatal("region edit did not move logical selection", got)
	}
	term.Write([]byte("\x1b[S"))
	if _, ok := term.SelectionText(); ok {
		t.Fatal("lost logical selection remained active")
	}
	term = explicitHistorySelectionFixture(t)
	term.SelectSearchRange(6, 0, 6, 3)
	term.SetHistoryLines(0)
	term.Write([]byte("\r\nnext\r\nnext"))
	if got, ok := term.SelectionText(); got != "" || !ok {
		t.Fatal("out-of-storage selection changed during recycling", got, ok)
	}
}

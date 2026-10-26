package vt

import (
	"fmt"
	"testing"
)

func rowImageMarginFixture(t *testing.T, col int) *Terminal {
	t.Helper()
	term, _ := inlineTerminal(t)
	term.Write([]byte(fmt.Sprintf("\x1b[1;%dH", col+1)))
	term.Write(append(append([]byte("\x1b]1337;File="), inlinePayload("", inlinePNG(t, 2, 100))...), '\a'))
	if len(term.Graphics(10, 20)) != 5 {
		t.Fatal("expected five independently attached rows")
	}
	return term
}

func checkRowImageMapping(t *testing.T, term *Terminal, col, cellHeight int, want map[int]int) {
	t.Helper()
	g := term.Graphics(10, cellHeight)
	if len(g) != len(want) {
		t.Fatalf("stripe count %d, want %d: %+v", len(g), len(want), g)
	}
	seen := make(map[int]bool)
	for _, p := range g {
		source := p.Source.Min.Y / 20
		row, ok := want[source]
		if !ok || seen[source] || p.Y != row*cellHeight || p.X != col*10 || p.Height != 20 || p.Source.Dy() != 20 {
			t.Fatalf("source stripe %d has wrong row/geometry: %+v; want %v", source, p, want)
		}
		seen[source] = true
	}
}

func TestRowImageFullWidthLineEdits(t *testing.T) {
	for _, cellHeight := range []int{10, 20, 40} {
		for _, tc := range []struct {
			command string
			want    map[int]int
		}{
			{"\x1b[L", map[int]int{0: 0, 1: 1, 2: 3, 3: 4}},
			{"\x1b[M", map[int]int{0: 0, 1: 1, 3: 2, 4: 3}},
			{"\x1b[99L", map[int]int{0: 0, 1: 1}},
			{"\x1b[99M", map[int]int{0: 0, 1: 1}},
		} {
			t.Run(fmt.Sprintf("%d/%q", cellHeight, tc.command), func(t *testing.T) {
				term := rowImageMarginFixture(t, 5)
				if err := term.Resize(20, 10, 10, cellHeight); err != nil {
					t.Fatal(err)
				}
				term.Write([]byte("\x1b[2;5r\x1b[3;6H" + tc.command))
				checkRowImageMapping(t, term, 5, cellHeight, tc.want)
			})
		}
	}
}

func TestRowImageMarginModeExplicitEditsPreserveImages(t *testing.T) {
	for _, horizontal := range []string{"\x1b[5;9s", "\x1b[1;20s"} {
		for _, command := range []string{"\x1b[S", "\x1b[T", "\x1b[L", "\x1b[M"} {
			t.Run(fmt.Sprintf("%q/%q", horizontal, command), func(t *testing.T) {
				term := rowImageMarginFixture(t, 5)
				term.Write([]byte("\x1b[?69h" + horizontal + "\x1b[2;4r\x1b[2;6H" + command))
				checkRowImageMapping(t, term, 5, 20, map[int]int{0: 0, 1: 1, 2: 2, 3: 3, 4: 4})
			})
		}
	}
}

func TestRowImageNarrowIndexClearsWholeAffectedLines(t *testing.T) {
	for _, col := range []int{0, 5, 12} {
		for _, command := range []string{"\x1b[4;6H\n", "\x1b[2;6H\x1bM", "\x1b[4;9HXY"} {
			t.Run(fmt.Sprintf("%d/%q", col, command), func(t *testing.T) {
				term := rowImageMarginFixture(t, col)
				term.Write([]byte("\x1b[?69h\x1b[5;9s\x1b[2;4r" + command))
				checkRowImageMapping(t, term, col, 20, map[int]int{0: 0, 4: 4})
			})
		}
	}
}

func TestRowImageLineEditsOutsideRowsDoNothing(t *testing.T) {
	for _, row := range []int{1, 6} {
		for _, command := range []string{"\x1b[L", "\x1b[M"} {
			t.Run(fmt.Sprintf("%d/%q", row, command), func(t *testing.T) {
				term := rowImageMarginFixture(t, 5)
				term.Write([]byte(fmt.Sprintf("\x1b[2;5r\x1b[%d;6H%s", row, command)))
				checkRowImageMapping(t, term, 5, 20, map[int]int{0: 0, 1: 1, 2: 2, 3: 3, 4: 4})
			})
		}
	}
}

func TestRowImageNarrowClearPreservesHistoryAndOtherScreen(t *testing.T) {
	t.Run("history", func(t *testing.T) {
		term := rowImageMarginFixture(t, 0)
		term.Write([]byte("\x1b[10;1H\n\n\n\n\n\x1b[1;1H"))
		term.Write(append(append([]byte("\x1b]1337;File="), inlinePayload("", inlinePNG(t, 2, 20))...), '\a'))
		term.Write([]byte("\x1b[?69h\x1b[5;9s\x1b[1;3r\x1b[3;6H\n"))
		if len(term.Graphics(10, 20)) != 0 {
			t.Fatal("active row was not cleared")
		}
		term.ScrollToTop()
		checkRowImageMapping(t, term, 0, 20, map[int]int{0: 0, 1: 1, 2: 2, 3: 3, 4: 4})
	})
	t.Run("alternate", func(t *testing.T) {
		term := rowImageMarginFixture(t, 5)
		term.Write([]byte("\x1b[?1049h\x1b[1;6H"))
		term.Write(append(append([]byte("\x1b]1337;File="), inlinePayload("", inlinePNG(t, 2, 100))...), '\a'))
		term.Write([]byte("\x1b[?69h\x1b[5;9s\x1b[2;4r\x1b[4;6H\n"))
		checkRowImageMapping(t, term, 5, 20, map[int]int{0: 0, 4: 4})
		term.Write([]byte("\x1b[?1049l"))
		checkRowImageMapping(t, term, 5, 20, map[int]int{0: 0, 1: 1, 2: 2, 3: 3, 4: 4})
	})
}

func TestRowImageNarrowInsertionCanClearItsOwnStripes(t *testing.T) {
	term, _ := inlineTerminal(t)
	term.Write([]byte("\x1b[?69h\x1b[5;9s\x1b[2;3r\x1b[3;6H"))
	term.Write(append(append([]byte("\x1b]1337;File="), inlinePayload("", inlinePNG(t, 2, 200))...), '\a'))
	if len(term.Graphics(10, 20)) != 0 {
		t.Fatal("each bottom-row index should clear the inserted stripe")
	}
	// Clearing the only attached stripe during insertion must preserve the
	// still-in-progress pixel source. A following ordinary insertion works.
	term.Write([]byte("\x1b[?69l\x1b[r\x1b[H"))
	term.Write(append(append([]byte("\x1b]1337;File="), inlinePayload("", inlinePNG(t, 2, 20))...), '\a'))
	checkRowImageMapping(t, term, 0, 20, map[int]int{0: 0})
}

func TestRowImageNarrowIndexOutsideColumnsDoesNotClear(t *testing.T) {
	for _, command := range []string{"\x1b[4;1H\n", "\x1b[2;20H\x1bM"} {
		t.Run(fmt.Sprintf("%q", command), func(t *testing.T) {
			term := rowImageMarginFixture(t, 5)
			term.Write([]byte("\x1b[?69h\x1b[5;9s\x1b[2;4r" + command))
			checkRowImageMapping(t, term, 5, 20, map[int]int{0: 0, 1: 1, 2: 2, 3: 3, 4: 4})
		})
	}
}

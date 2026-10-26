package terminal

import (
	"fmt"
	"testing"
)

func TestHistoryLimitPreservesScrolledSelection(t *testing.T) {
	term, tt, _ := bidiViewFixture(t)
	tt.Frame()
	_, rows := term.Size()
	for i := 0; i < rows+4; i++ {
		term.Feed([]byte(fmt.Sprintf("%03d\r\n", i)))
	}
	term.mu.Lock()
	term.term.ScrollToTop()
	term.mu.Unlock()
	tt.Frame()
	x0, y0 := cellCenter(term, 0, 0)
	x3, y3 := cellCenter(term, 3, 0)
	beginCharacterDrag(tt, x0, y0)
	tt.Move(x3, y3)
	tt.Release(x3, y3)
	for _, limit := range []int{1000, 500} {
		if err := term.SetHistoryLimit(limit); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if got := term.SearchSelection(); got != "000" {
			t.Fatal("retained history selection lost", limit, got)
		}
		term.mu.Lock()
		offset := term.term.Scrollbar().Offset
		term.mu.Unlock()
		if offset != 0 {
			t.Fatal("history capacity change jumped viewport", offset)
		}
	}
}

func TestHistoryLimitPruningPrimaryPreservesAlternateSelection(t *testing.T) {
	term, tt, _ := bidiViewFixture(t)
	tt.Frame()
	if err := term.SetHistoryLimit(1000); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 650; i++ {
		term.Feed([]byte("history\r\n"))
	}
	term.Feed([]byte("\x1b[?47h\x1b[Halternate"))
	tt.Frame()
	x0, y0 := cellCenter(term, 0, 0)
	x3, y3 := cellCenter(term, 3, 0)
	beginCharacterDrag(tt, x0, y0)
	tt.Move(x3, y3)
	tt.Release(x3, y3)
	if got := term.SearchSelection(); got != "alt" {
		t.Fatal("alternate fixture", got)
	}
	if err := term.SetHistoryLimit(500); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := term.SearchSelection(); got != "alt" {
		t.Fatal("primary history pruning cleared alternate selection", got)
	}
	term.Feed([]byte("\x1b[?47l"))
	term.mu.Lock()
	bar := term.term.Scrollbar()
	term.mu.Unlock()
	if bar.Total-bar.Len != 500 {
		t.Fatal("primary history was not pruned", bar)
	}
}

func TestHistoryLimitWithoutPruningPreservesSelectionAndDrag(t *testing.T) {
	for _, tc := range []struct {
		name     string
		from, to int
	}{
		{"increase", 500, 1000},
		{"decrease", 1000, 500},
		{"same", 500, 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			term, tt, _ := bidiViewFixture(t)
			tt.Frame()
			if err := term.SetHistoryLimit(tc.from); err != nil {
				t.Fatal(err)
			}
			term.Feed([]byte("abcdef"))
			tt.Frame()
			x0, y0 := cellCenter(term, 0, 0)
			x3, y3 := cellCenter(term, 3, 0)
			beginCharacterDrag(tt, x0, y0)
			tt.Move(x3, y3)
			if got := term.SearchSelection(); got != "abc" {
				t.Fatal("selection fixture", got)
			}
			if err := term.SetHistoryLimit(tc.to); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			if got := term.SearchSelection(); got != "abc" {
				t.Fatal("capacity-only change cleared selection", got)
			}
			term.v.copy()
			if got := tt.Clipboard(); got != "abc" {
				t.Fatal("capacity-only change lost copy", got)
			}
			x5, y5 := cellCenter(term, 5, 0)
			tt.Move(x5, y5)
			if got := term.SearchSelection(); got != "abcde" {
				t.Fatal("capacity-only change lost drag anchor", got)
			}
			tt.Release(x5, y5)
		})
	}
}

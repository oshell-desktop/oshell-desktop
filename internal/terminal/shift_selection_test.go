package terminal

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestShiftCharacterSelectionAndDrag(t *testing.T) {
	for _, tc := range []struct {
		name       string
		end        int
		want, drag string
	}{{"forward", 8, "cdefgh", "cdef"}, {"reverse", 0, "abcde", "f"}} {
		t.Run(tc.name, func(t *testing.T) {
			term, tt := wordClickFixture(t, "abcdefghij", 320)
			x, y := cellCenter(term, 2, 0)
			beginCharacterDrag(tt, x, y)
			x, y = cellCenter(term, 5, 0)
			tt.Move(x, y)
			tt.Release(x, y)

			x, y = cellCenter(term, tc.end, 0)
			term.v.pointerEvent(ui.InputEvent{Kind: ui.InputPointerDown, X: x, Y: y, Mods: ui.Shift})
			if got := term.SearchSelection(); got != tc.want {
				t.Fatalf("shift=%q want %q", got, tc.want)
			}

			x, y = cellCenter(term, 6, 0)
			term.v.pointerEvent(ui.InputEvent{Kind: ui.InputPointerMove, X: x, Y: y})
			term.v.pointerEvent(ui.InputEvent{Kind: ui.InputPointerUp, X: x, Y: y})
			if got := term.SearchSelection(); got != tc.drag {
				t.Fatalf("continued drag=%q want %q", got, tc.drag)
			}
		})
	}
}
func TestShiftWordSelectionKeepsOriginalSeedForDrag(t *testing.T) {
	term, tt := wordClickFixture(t, "aaa bbb ccc ddd", 320)
	clickWord(term, tt, 5, 0)
	clickWord(term, tt, 5, 0)

	x, y := cellCenter(term, 9, 0)
	tt.ClickAtWith(ui.Shift, x, y)
	if got := term.SearchSelection(); got != "bbb ccc" {
		t.Fatal("forward word shift", got)
	}
	x, y = cellCenter(term, 1, 0)
	term.v.pointerEvent(ui.InputEvent{Kind: ui.InputPointerDown, X: x, Y: y, Mods: ui.Shift})
	if got := term.SearchSelection(); got != "aaa bbb ccc" {
		t.Fatal("reverse word shift", got)
	}

	x, y = cellCenter(term, 13, 0)
	term.v.pointerEvent(ui.InputEvent{Kind: ui.InputPointerMove, X: x, Y: y})
	term.v.pointerEvent(ui.InputEvent{Kind: ui.InputPointerUp, X: x, Y: y})
	if got := term.SearchSelection(); got != "bbb ccc ddd" {
		t.Fatal("drag lost original seed", got)
	}
}

func TestShiftSingleClickKeepsWordDelimiterPolicy(t *testing.T) {
	term, tt := wordClickFixture(t, "aaa foo.bar baz.qux", 320)
	clickWord(term, tt, 5, 0)
	clickWord(term, tt, 5, 0)
	if got := term.SearchSelection(); got != "foo" {
		t.Fatal("word fixture", got)
	}

	shiftClick(term, tt, 14, 0)
	if got := term.SearchSelection(); got != "foo.bar baz" {
		t.Fatal("Shift single click changed word delimiters", got)
	}
	// Shift double click establishes its own whitespace-only word policy.
	shiftClick(term, tt, 14, 0)
	if got := term.SearchSelection(); got != "baz.qux" {
		t.Fatal("Shift double click failed to apply its word policy", got)
	}
}
func TestShiftRowSelectionKeepsOriginalRowAnchor(t *testing.T) {
	term, tt := wordClickFixture(t, "row0\r\nrow1\r\nrow2\r\nrow3", 320)
	for range 3 {
		clickWord(term, tt, 1, 1)
	}

	x, y := cellCenter(term, 1, 3)
	tt.ClickAtWith(ui.Shift, x, y)
	if got := term.SearchSelection(); got != "row1\nrow2\nrow3" {
		t.Fatal("forward row shift", got)
	}
	x, y = cellCenter(term, 1, 0)
	term.v.pointerEvent(ui.InputEvent{Kind: ui.InputPointerDown, X: x, Y: y, Mods: ui.Shift})
	if got := term.SearchSelection(); got != "row0\nrow1" {
		t.Fatal("reverse row shift", got)
	}

	x, y = cellCenter(term, 1, 2)
	term.v.pointerEvent(ui.InputEvent{Kind: ui.InputPointerMove, X: x, Y: y})
	term.v.pointerEvent(ui.InputEvent{Kind: ui.InputPointerUp, X: x, Y: y})
	if got := term.SearchSelection(); got != "row1\nrow2" {
		t.Fatal("drag lost row anchor", got)
	}
}

func TestShiftSelectionFromSearchAndEmptyRange(t *testing.T) {
	for _, empty := range []bool{false, true} {
		term, tt := wordClickFixture(t, "abcdefghij", 320)
		if empty {
			x, y := cellCenter(term, 2, 0)
			beginCharacterDrag(tt, x, y)
			tt.Release(x, y)
		} else {
			term.mu.Lock()
			term.term.SelectSearchRange(0, 2, 0, 5)
			term.mu.Unlock()
		}

		x, y := cellCenter(term, 8, 0)
		tt.ClickAtWith(ui.Shift, x, y)
		if got := tt.Clipboard(); got != "cdefgh" {
			t.Fatalf("empty=%v copied %q", empty, got)
		}
	}
}

func TestShiftMultiClickStillSelectsWordAndRow(t *testing.T) {
	term, tt := wordClickFixture(t, "aaa bbb ccc", 320)
	clickWord(term, tt, 1, 0)
	clickWord(term, tt, 1, 0)

	shiftClick(term, tt, 9, 0) // one click extends the old word selection
	shiftClick(term, tt, 9, 0) // two clicks replace it with the target word
	if got := term.SearchSelection(); got != "ccc" {
		t.Fatal("Shift double click extended instead of replacing the selection", got)
	}
	shiftClick(term, tt, 9, 0)
	if got := term.SearchSelection(); got != "aaa bbb ccc" {
		t.Fatal("Shift triple click did not select the physical row", got)
	}
}

func TestShiftDragTracksSelectionMovedByRegionScroll(t *testing.T) {
	term, tt := wordClickFixture(t, "000\r\n111\r\nabcdefghij\r\n333\r\n444", 320)
	term.mu.Lock()
	term.term.SelectSearchRange(2, 2, 2, 5)
	term.mu.Unlock()
	x, y := cellCenter(term, 8, 2)
	term.v.pointerEvent(ui.InputEvent{Kind: ui.InputPointerDown, X: x, Y: y, Mods: ui.Shift})
	term.Feed([]byte("\x1b[2;4r\x1b[S"))
	tt.Frame()
	x, y = cellCenter(term, 6, 1)
	term.v.pointerEvent(ui.InputEvent{Kind: ui.InputPointerMove, X: x, Y: y})
	term.v.pointerEvent(ui.InputEvent{Kind: ui.InputPointerUp, X: x, Y: y})
	if got := term.SearchSelection(); got != "cdef" {
		t.Fatal("Shift drag did not follow the moved fixed endpoint", got)
	}
}

// Tester.Press does not carry HoldModifiers into a pointer event. Use the
// explicit modified-click API, or deliver a modified press to the input handler
// when the test must inspect state before releasing and continuing the drag.
func shiftClick(term *Terminal, tt *ui.Tester, col, row int) {
	x, y := cellCenter(term, col, row)
	tt.ClickAtWith(ui.Shift, x, y)
}

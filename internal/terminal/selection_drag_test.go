package terminal

import (
	"fmt"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestBidiDragLogicalBoundaries(t *testing.T) {
	for _, tc := range []struct {
		text, want string
		end        int
	}{
		{"ab אב cd", "ab", 3},
		{"אבג abc", "אבג", 3},
		{"\x1b[8l\x1b[2 kABCDEFG", "ABC", 3},
		{"א\u05b0ב 界 😀 xyz", "א\u05b0ב", 3},
		{"א\u05b0ב 界 😀 xyz", "א\u05b0ב 界 😀", 9},
	} {
		for _, backwards := range []bool{false, true} {
			for _, fraction := range []float32{0.1, 0.9} {
				t.Run(fmt.Sprintf("%q/end=%d/reverse=%v/fraction=%.1f", tc.text, tc.end, backwards, fraction), func(t *testing.T) {
					term, tt, _ := bidiViewFixture(t)
					term.Feed([]byte(tc.text))
					tt.Frame()
					v := term.v
					// Swift's character selection is a half-open logical range;
					// both directions and either side of a displayed cell agree.
					a, b := 0, tc.end
					if backwards {
						a, b = b, a
					}
					point := func(logical int) (float32, float32) {
						col := v.visualColumn(0, logical)
						x, y := cellCenter(term, col, 0)
						return x + (fraction-0.5)*float32(v.cellW)/v.scale, y
					}
					x0, y0 := point(a)
					x1, y1 := point(b)
					beginCharacterDrag(tt, x0, y0)
					tt.Move(x1, y1)
					if got := term.SearchSelection(); got != tc.want {
						t.Fatalf("logical drag = %q, want %q", got, tc.want)
					}
					// Moving back to the anchor must clear the old selection.
					tt.Move(x0, y0)
					if got := term.SearchSelection(); got != "" {
						t.Fatalf("returned to anchor retained %q", got)
					}
					tt.Release(x0, y0)
				})
			}
		}
	}
}

func TestBidiWordDragBothDirections(t *testing.T) {
	term, tt, _ := bidiViewFixture(t)
	term.Feed([]byte("abc אב xyz"))
	tt.Frame()
	point := func(logical int) (float32, float32) {
		return cellCenter(term, term.v.visualColumn(0, logical), 0)
	}
	x, y := point(4)
	tt.ClickAt(x, y)
	tt.Press(x, y)
	for _, tc := range []struct {
		col  int
		want string
	}{{0, "abc אב"}, {7, "אב xyz"}, {5, "אב"}} {
		x, y = point(tc.col)
		tt.Move(x, y)
		if got := term.SearchSelection(); got != tc.want {
			t.Fatalf("word drag to logical %d = %q, want %q", tc.col, got, tc.want)
		}
	}
	tt.Release(x, y)
}

func TestBidiDragAcrossSoftWrap(t *testing.T) {
	term, tt, _ := bidiViewFixture(t)
	prefix := "א" + strings.Repeat(".", term.v.cols-1)
	term.Feed([]byte(prefix + "אב xyz"))
	tt.Frame()
	x0, y0 := cellCenter(term, term.v.visualColumn(0, 0), 0)
	x1, y1 := cellCenter(term, term.v.visualColumn(1, 2), 1)
	beginCharacterDrag(tt, x0, y0)
	tt.Move(x1, y1)
	tt.Release(x1, y1)
	if got := term.SearchSelection(); got != prefix+"אב" {
		t.Fatalf("soft wrap logical copy = %q, want %q", got, prefix+"אב")
	}
}

func TestSelectionAutoscrollOneRow(t *testing.T) {
	for _, bidi := range []bool{false, true} {
		t.Run(fmt.Sprint(bidi), func(t *testing.T) {
			term, tt, _ := bidiViewFixture(t)
			term.mu.Lock()
			term.term.SetBidiEnabled(bidi)
			term.mu.Unlock()
			term.Feed([]byte(strings.Repeat("abc אב\r\n", 60)))
			tt.Frame()
			term.mu.Lock()
			term.term.ScrollToRow(10)
			term.mu.Unlock()
			v := term.v
			x, y := cellCenter(term, 2, 2)
			v.pointerEvent(ui.InputEvent{Kind: ui.InputPointerDown, X: x, Y: y})
			bottom := float32(v.oy+v.rows*v.cellH+8) / v.scale
			v.pointerEvent(ui.InputEvent{Kind: ui.InputPointerMove, X: x, Y: bottom})
			term.mu.Lock()
			before := term.term.Scrollbar().Offset
			v.autoscroll()
			after := term.term.Scrollbar().Offset
			term.mu.Unlock()
			if after != before+1 {
				t.Fatalf("one timer tick scrolled %d rows, want 1", after-before)
			}
			v.pointerEvent(ui.InputEvent{Kind: ui.InputPointerUp, X: x, Y: bottom})
		})
	}
}

func TestSelectionDragReturnClearsAndInvalidAnchorPreserves(t *testing.T) {
	term, tt, _ := bidiViewFixture(t)
	term.Feed([]byte("ordinary selection"))
	tt.Frame()
	x0, y0 := cellCenter(term, 0, 0)
	x1, y1 := cellCenter(term, 5, 0)
	beginCharacterDrag(tt, x0, y0)
	tt.Move(x1, y1)
	if got := term.SearchSelection(); got != "ordin" {
		t.Fatal("initial drag", got)
	}
	tt.Move(x0, y0)
	if got := term.SearchSelection(); got != "" {
		t.Fatal("empty drag retained old text", got)
	}
	term.Feed([]byte("\x1b[?47h\x1b[Halt"))
	term.mu.Lock()
	term.term.SelectSearchRange(0, 0, 0, 3)
	term.mu.Unlock()
	if got := term.SearchSelection(); got != "alt" {
		t.Fatal("alternate selection fixture", got)
	}
	// The old screen's anchor is invalid; a stale drag must not clear a new
	// selection made by another action in the replacement screen.
	tt.Move(x1, y1)
	if got := term.SearchSelection(); got != "alt" {
		t.Fatal("invalid gesture erased a new screen selection", got)
	}
	tt.Release(x1, y1)
}

func TestSelectionAutoscrollRetainsRectangle(t *testing.T) {
	term, tt, _ := bidiViewFixture(t)
	term.Feed([]byte(strings.Repeat("0123456789\r\n", 60)))
	tt.Frame()
	term.mu.Lock()
	term.term.ScrollToRow(10)
	term.mu.Unlock()
	v := term.v
	x0, y := cellCenter(term, 2, 2)
	x1, _ := cellCenter(term, 4, 2)
	bottom := float32(v.oy+v.rows*v.cellH+8) / v.scale
	v.pointerEvent(ui.InputEvent{Kind: ui.InputPointerDown, X: x0, Y: y, Mods: ui.Alt})
	v.pointerEvent(ui.InputEvent{Kind: ui.InputPointerMove, X: x1, Y: bottom, Mods: ui.Alt})
	term.mu.Lock()
	v.autoscroll()
	text, _ := term.term.SelectionText()
	term.mu.Unlock()
	for _, line := range strings.Split(text, "\n") {
		if line != "23" {
			t.Fatalf("autoscroll expanded rectangle to full rows: %q", text)
		}
	}
	v.pointerEvent(ui.InputEvent{Kind: ui.InputPointerUp, X: x1, Y: bottom, Mods: ui.Alt})
}

func TestBidiAutoscrollMapsNewViewport(t *testing.T) {
	term, tt, _ := bidiViewFixture(t)
	for row := 0; row < 60; row++ {
		dir := 1 + row%2
		term.Feed([]byte(fmt.Sprintf("\x1b[8l\x1b[%d kabcdefghijklmno\r\n", dir)))
	}
	tt.Frame()
	term.mu.Lock()
	term.term.ScrollToRow(10)
	term.mu.Unlock()
	v := term.v
	x, y := cellCenter(term, 3, 2)
	bottom := float32(v.oy+v.rows*v.cellH+8) / v.scale
	v.pointerEvent(ui.InputEvent{Kind: ui.InputPointerDown, X: x, Y: y})
	v.pointerEvent(ui.InputEvent{Kind: ui.InputPointerMove, X: x, Y: bottom})
	for i := 0; i < 2; i++ {
		term.mu.Lock()
		v.autoscroll()
		// The captured pointer is one row beyond the visible grid. Swift maps
		// it into the buffer, rather than clamping it to the last visible row.
		end := int(term.term.Scrollbar().Offset) + v.rows
		text, _ := term.term.SelectionText()
		term.mu.Unlock()
		want := "abc"
		if end%2 == 1 {
			want = "abcdefghijklmno"
		}
		lines := strings.Split(text, "\n")
		if got := lines[len(lines)-1]; got != want {
			t.Fatalf("new viewport end row %d = %q, want %q", end, got, want)
		}
	}
	v.pointerEvent(ui.InputEvent{Kind: ui.InputPointerUp, X: x, Y: bottom})
}

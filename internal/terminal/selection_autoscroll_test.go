package terminal

import (
	"github.com/egoist/mygo/ui"
	"strings"
	"testing"
	"time"
)

func TestSelectionAutoscrollDoesNotRunOnEveryPaint(t *testing.T) {
	term, tt, _ := bidiViewFixture(t)
	term.Feed([]byte(strings.Repeat("abcdefghij\r\n", 120)))
	tt.Frame()
	term.mu.Lock()
	term.term.ScrollToRow(20)
	term.mu.Unlock()
	tt.Frame()
	v := term.v
	x, y := cellCenter(term, 2, 2)
	v.pointerEvent(ui.InputEvent{Kind: ui.InputPointerDown, X: x, Y: y})
	y = float32(v.oy+(v.rows+2)*v.cellH) / v.scale
	v.pointerEvent(ui.InputEvent{Kind: ui.InputPointerMove, X: x, Y: y})
	term.mu.Lock()
	v.selectionScrollNext = time.Now().Add(time.Hour)
	before := term.term.Scrollbar().Offset
	term.mu.Unlock()
	for range 10 {
		tt.Frame()
	}
	term.mu.Lock()
	after := term.term.Scrollbar().Offset
	term.mu.Unlock()
	if after != before {
		t.Fatalf("paint scrolled from %d to %d without a timer firing", before, after)
	}
	v.pointerEvent(ui.InputEvent{Kind: ui.InputPointerUp, X: x, Y: y})
}

func TestSelectionAutoscrollDeadlinesAndCoalescing(t *testing.T) {
	term, tt, _ := bidiViewFixture(t)
	term.Feed([]byte(strings.Repeat("abcdefghij\r\n", 160)))
	tt.Frame()
	v := term.v
	term.mu.Lock()
	defer term.mu.Unlock()
	term.term.ScrollToRow(20)
	v.selecting = true
	base := time.Now().Add(time.Hour)
	v.setSelectionAutoscroll(1, base)
	for _, tc := range []struct{ ms, offset int }{{49, 20}, {50, 21}, {50, 21}, {99, 21}, {100, 22}, {410, 23}, {449, 23}, {450, 24}} {
		v.autoscrollAt(base.Add(time.Duration(tc.ms) * time.Millisecond))
		if got := term.term.Scrollbar().Offset; int(got) != tc.offset {
			t.Fatalf("at %d ms offset=%d want %d", tc.ms, got, tc.offset)
		}
	}
	// Pointer movement changes speed, without restarting a running timer.
	due := v.selectionScrollNext
	v.setSelectionAutoscroll(3, base.Add(475*time.Millisecond))
	if v.selectionScrollNext != due {
		t.Fatal("moving pointer reset timer phase")
	}
	v.autoscrollAt(due)
	if got := term.term.Scrollbar().Offset; got != 27 {
		t.Fatal("new speed not applied", got)
	}
	v.setSelectionAutoscroll(0, base)
	v.autoscrollAt(base.Add(time.Hour))
	if got := term.term.Scrollbar().Offset; got != 27 {
		t.Fatal("stopped timer scrolled", got)
	}
}

func TestSelectionAutoscrollUsesBufferDistance(t *testing.T) {
	term, tt, _ := bidiViewFixture(t)
	term.Feed([]byte(strings.Repeat("abcdefghij\r\n", 200)))
	tt.Frame()
	term.mu.Lock()
	term.term.ScrollToRow(50)
	term.mu.Unlock()
	tt.Frame()
	v := term.v
	x, y := cellCenter(term, 2, 2)
	v.pointerEvent(ui.InputEvent{Kind: ui.InputPointerDown, X: x, Y: y})
	for _, tc := range []struct{ row, want int }{{0, -1}, {1, 0}, {v.rows - 1, 0}, {v.rows, 1}, {v.rows + 2, 3}, {v.rows + 6, 10}, {v.rows + 10, max(v.rows, 20)}, {-2, -3}, {-6, -10}, {-10, -max(v.rows, 20)}} {
		fractional := float32(tc.row) + 0.25
		if tc.row < 0 {
			fractional = float32(tc.row) - 0.25
		}
		y = (float32(v.oy) + fractional*float32(v.cellH)) / v.scale
		v.pointerEvent(ui.InputEvent{Kind: ui.InputPointerMove, X: x, Y: y})
		if v.selectionScrollDelta != tc.want {
			t.Fatalf("row=%d speed=%d want %d", tc.row, v.selectionScrollDelta, tc.want)
		}
	}
	// With no retained rows below the viewport, a below-bottom hit clamps to
	// the final buffer row and does not create a downward timer.
	term.mu.Lock()
	term.term.ScrollToBottom()
	term.mu.Unlock()
	y = float32(v.oy+(v.rows+12)*v.cellH) / v.scale
	v.pointerEvent(ui.InputEvent{Kind: ui.InputPointerMove, X: x, Y: y})
	if v.ticking {
		t.Fatal("downward timer started beyond the last buffer row")
	}
	v.pointerEvent(ui.InputEvent{Kind: ui.InputPointerUp, X: x, Y: y})
}

func TestSelectionDragCanEndOutsideViewport(t *testing.T) {
	term, tt, _ := bidiViewFixture(t)
	term.Feed([]byte(strings.Repeat("abcdefghij\r\n", 160)))
	tt.Frame()
	term.mu.Lock()
	term.term.ScrollToRow(20)
	term.mu.Unlock()
	tt.Frame()
	v := term.v
	x, y := cellCenter(term, 2, 2)
	beginCharacterDrag(tt, x, y)
	y = float32(v.oy+(v.rows+6)*v.cellH) / v.scale
	v.pointerEvent(ui.InputEvent{Kind: ui.InputPointerMove, X: x, Y: y})
	term.mu.Lock()
	_, _, end, _, ok := term.term.SearchSelectionRange()
	term.mu.Unlock()
	if !ok || end != 20+v.rows+6 {
		t.Fatalf("end row=%d/%v want %d", end, ok, 20+v.rows+6)
	}
	v.pointerEvent(ui.InputEvent{Kind: ui.InputPointerUp, X: x, Y: y})
}

func TestSelectionAutoscrollContinuesAfterCancellation(t *testing.T) {
	term, tt, _ := bidiViewFixture(t)
	term.Feed([]byte(strings.Repeat("abcdefghij\r\n", 160)))
	tt.Frame()
	term.mu.Lock()
	term.term.ScrollToRow(20)
	term.mu.Unlock()
	tt.Frame()
	v := term.v
	x, y := cellCenter(term, 2, 2)
	v.pointerEvent(ui.InputEvent{Kind: ui.InputPointerDown, X: x, Y: y})
	y = float32(v.oy+v.rows*v.cellH) / v.scale
	v.pointerEvent(ui.InputEvent{Kind: ui.InputPointerMove, X: x, Y: y})
	term.mu.Lock()
	term.term.SetSelection(nil)
	v.autoscrollAt(v.selectionScrollNext)
	offset, active := term.term.Scrollbar().Offset, term.term.SelectionActive()
	term.mu.Unlock()
	if offset != 21 || active || !v.ticking {
		t.Fatalf("cancelled selection: offset=%d active=%v timer=%v", offset, active, v.ticking)
	}
	v.pointerEvent(ui.InputEvent{Kind: ui.InputPointerUp, X: x, Y: y})
	if v.ticking {
		t.Fatal("release left the timer active")
	}
}

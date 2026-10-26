package terminal

import (
	"time"

	"oshell-desktop/internal/terminal/internal/vt"
)

const selectionScrollInterval = 50 * time.Millisecond

// selectionCellAt uses buffer coordinates, as MacTerminalView.calculateMouseHit
// does. A captured drag may point at history above or below the viewport.
// The caller holds t.mu; the returned reference must not survive buffer edits.
func (v *view) selectionCellAt(x, y float32) (vt.GridRef, int, bool) {
	t := v.t.term
	bar := t.Scrollbar()
	// Swift Int truncates toward zero, including just above the first row.
	row := int((y*v.scale - float32(v.oy)) / float32(v.cellH))
	absRow := max(0, min(row+int(bar.Offset), int(bar.Total)-1))
	row = absRow - int(bar.Offset)
	width := v.cellW * int(lineHorizontalScale(t.BufferLineModeAt(absRow)))
	col := max(0, min(int((x*v.scale-float32(v.ox))/float32(width)), v.cols-1))
	if row >= 0 && row < v.rows {
		col = v.logicalColumn(row, col)
	} else if maps := t.BidiRows(absRow, 1); len(maps) > 0 {
		if m := maps[0].Row.VisualToLogical; col < len(m) {
			col = m[col]
		}
	}
	ref, ok := t.BufferCellAt(col, absRow)
	return ref, row, ok
}

func selectionScrollVelocity(row, rows int) int {
	distance, direction := 0, 1
	switch {
	case row <= 0:
		distance, direction = -row, -1
	case row >= rows:
		distance = row - rows
	default:
		return 0
	}
	speed := 1
	switch {
	case distance > 9:
		speed = max(rows, 20)
	case distance > 5:
		speed = 10
	case distance > 1:
		speed = 3
	}
	return direction * speed
}

func (v *view) setSelectionAutoscroll(delta int, now time.Time) {
	v.selectionScrollDelta = delta
	if delta == 0 {
		v.ticking = false
		v.selectionScrollNext = time.Time{}
		return
	}
	if !v.ticking {
		v.selectionScrollNext = now.Add(selectionScrollInterval)
	}
	v.ticking = true
}

// Repaints may run more often than the timer. Fire once at a due deadline and
// coalesce missed intervals, retaining the timer phase instead of catching up
// with a burst of viewport changes. The caller holds t.mu.
func (v *view) autoscrollAt(now time.Time) {
	if !v.ticking || now.Before(v.selectionScrollNext) {
		return
	}
	missed := now.Sub(v.selectionScrollNext)/selectionScrollInterval + 1
	v.selectionScrollNext = v.selectionScrollNext.Add(missed * selectionScrollInterval)
	v.autoscroll()
}

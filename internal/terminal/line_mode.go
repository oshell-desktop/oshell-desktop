package terminal

import "oshell-desktop/internal/terminal/internal/vt"

func lineHorizontalScale(mode vt.LineMode) float32 {
	if mode == vt.LineSingle {
		return 1
	}
	return 2
}

func lineTransform(mode vt.LineMode, height float32) (sx, sy, offset float32) {
	sx, sy = lineHorizontalScale(mode), 1
	if mode == vt.LineDoubleTop || mode == vt.LineDoubleBottom {
		sy = 2
	}
	if mode == vt.LineDoubleBottom {
		offset = -height
	}
	return
}

// Input handlers hold t.mu and query the current viewport, including scrolling
// before the next render frame has refreshed the row cache.
func (v *view) inputCellWidth(row int) int {
	if v.t != nil && v.t.term != nil {
		return v.cellW * int(lineHorizontalScale(v.t.term.LineModeAt(row)))
	}
	return v.cellW
}

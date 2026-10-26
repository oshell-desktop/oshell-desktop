package terminal

import (
	"math"
	"slices"

	"oshell-desktop/internal/terminal/internal/vt"
)

// Both drawing and input use owned maps. Input may refresh before the next
// frame; the already painted row caches retain their previous maps until paint.
type bidiView struct {
	bidi                 []vt.BidiPresentation
	bidiFrame            []vt.BidiPresentation
	bidiRevision         uint64
	bidiScroll           vt.Scrollbar
	bidiCols             int
	bidiReady, bidiDirty bool
}

// refreshBidi requires t.mu. It never reads the asynchronous render snapshot.
func (v *view) refreshBidi() {
	t := v.t
	if t == nil {
		return
	}
	if t.term == nil || !t.term.BidiEnabled() {
		if len(v.bidi) != 0 {
			v.bidiDirty = true
		}
		v.bidi, v.bidiReady = nil, false
		return
	}
	bar := t.term.Scrollbar()
	cols, _ := t.term.Size()
	if v.bidiReady && v.bidiRevision == t.revision && v.bidiScroll == bar && v.bidiCols == cols {
		return
	}
	next := t.term.BidiRows(int(bar.Offset), int(bar.Len))
	if !slices.EqualFunc(v.bidi, next, func(a, b vt.BidiPresentation) bool {
		return a.RTL == b.RTL && slices.Equal(a.Row.Cells, b.Row.Cells) &&
			slices.Equal(a.Row.LogicalToVisual, b.Row.LogicalToVisual) && slices.Equal(a.Row.VisualToLogical, b.Row.VisualToLogical)
	}) {
		v.bidiDirty = true
	}
	v.bidi, v.bidiRevision, v.bidiScroll, v.bidiCols, v.bidiReady = next, t.revision, bar, cols, true
}

func (v *view) visualColumn(row, logical int) int {
	if row >= 0 && row < len(v.lines) {
		m := v.lines[row].bidi.LogicalToVisual
		if logical >= 0 && logical < len(m) {
			return m[logical]
		}
	}
	return logical
}

func (v *view) logicalColumn(row, visual int) int {
	v.refreshBidi()
	if row >= 0 && row < len(v.bidi) {
		m := v.bidi[row].Row.VisualToLogical
		if visual >= 0 && visual < len(m) {
			return m[visual]
		}
	}
	return visual
}

// Pixel mouse reports retain physical coordinates. Cell reports use the same
// logical column as selection and link hit testing, including wide glyphs.
func (v *view) mousePosition(x, y float32) (float32, float32) {
	px, py := x*v.scale, y*v.scale
	if !v.t.term.BidiEnabled() || v.t.term.Mode(1016) {
		return px, py
	}
	col, row := v.cellAt(x, y)
	w := float32(v.inputCellWidth(row))
	if row >= len(v.bidi) || len(v.bidi[row].Row.VisualToLogical) == 0 || px < float32(v.ox) || px >= float32(v.ox)+float32(v.cols)*w {
		return px, py
	}
	fraction := float32(math.Mod(float64(px-float32(v.ox)), float64(w)))
	return float32(v.ox) + float32(col)*w + max(fraction, 0), py
}

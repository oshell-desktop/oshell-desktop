package terminal

import (
	"slices"

	"github.com/egoist/mygo/ui"
)

type linkHoverState struct {
	visibilityEpoch uint64
	x, y            float32
	enabled, dirty  bool
	match           terminalLink
	preview         string
	previewSet      bool
	previewDirty    bool
}

func (v *view) requestLinkHover(x, y float32, enabled bool) {
	h := &v.linkHover
	if epoch := v.t.visibilityEpoch.Load(); epoch != h.visibilityEpoch {
		h.visibilityEpoch, h.dirty, h.preview = epoch, true, ""
		h.previewSet = false
		h.previewDirty = true
		v.linkPressed, v.linkDragged = false, false
	}
	// Swift mouseDragged updates selection; mouseMoved/flagsChanged update
	// the link. Release performs a fresh lookup at its final position.
	if v.linkPressed && enabled == h.enabled {
		return
	}
	if h.x != x || h.y != y || h.enabled != enabled {
		h.x, h.y, h.enabled, h.dirty = x, y, enabled, true
		h.previewDirty = true
	}
}

// mouseUp refreshes the underline even after a drag or output at a stationary
// pointer. Unlike mouseMoved/flagsChanged, it does not refresh the URL preview.
func (v *view) requestLinkRelease(x, y float32, modifier bool) {
	h := &v.linkHover
	h.x, h.y, h.enabled, h.dirty = x, y, h.enabled || modifier, true
}

// refreshLinkHover runs with the terminal lock, after layout and BiDi refresh.
// Output alone does not create a new hover event. Ranges use buffer rows so
// scrolling never leaves an underline pinned to an unrelated screen row.
func (v *view) refreshLinkHover(held bool) bool {
	h := &v.linkHover
	if !h.dirty || held && h.enabled {
		return false
	}
	old := h.match.ranges
	h.dirty = false
	if !h.enabled {
		h.match, h.preview = terminalLink{}, ""
		h.previewSet = false
	} else {
		col, row := v.cellAt(h.x, h.y)
		h.match = v.linkMatchAt(col, row)
		if h.previewDirty && h.match.explicit {
			h.preview = h.match.clickURL()
			h.previewSet = true
		}
		// The source retains the last explicit preview over implicit or plain
		// cells until the modifier is released or the pointer leaves the view.
	}
	h.previewDirty = false
	return !slices.Equal(old, h.match.ranges)
}

func (v *view) hoveredLinkColumns(y int) columnSpan {
	for _, r := range v.linkHover.match.ranges {
		if r.row == y+int(v.scrollbar.Offset) {
			return columnSpan{r.start, r.end}
		}
	}
	return columnSpan{}
}

func (v *view) drawLinkPreview(p *ui.Painter, r ui.Rect) {
	text := v.linkHover.preview
	if !v.linkHover.previewSet {
		return
	}
	font := ui.Font{Size: 12}
	glyphs := ui.Shape(text, font)
	width := float32(4)
	for _, g := range glyphs {
		width += g.Advance
	}
	m := font.Metrics()
	height := m.Ascent + m.Descent + 4
	y := r.Y + r.H - height
	p.Clip(r, 0, func() {
		p.Fill(ui.Rect{X: r.X, Y: y, W: width, H: height}, color(v.colors.Foreground), 0)
		p.Glyphs(glyphs, r.X+2, y+2+m.Ascent, color(v.colors.Background))
	})
}

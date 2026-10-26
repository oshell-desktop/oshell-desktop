package terminal

import (
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/terminal/internal/vt"
)

const kittyPlaceholderScalar = '\U0010eeee'

// A placeholder is decoded from raw cell colors, before palette lookup,
// inverse, selection or highlights. Coordinates follow visual cell order.
type kittyPlaceholder struct {
	col, imageRow, imageCol, msb int
	imageID, placementID         uint32
	fg, underline                vt.Color
}

func placeholderColorID(c vt.Color) uint32 {
	switch c.Kind {
	case 1:
		return uint32(c.Index)
	case 2:
		return uint32(c.RGB.R)<<16 | uint32(c.RGB.G)<<8 | uint32(c.RGB.B)
	default:
		return 0
	}
}

// SwiftTerm KittyPlaceholderDecoder, OShell v0.2.92. Only recognized marks
// count; omitted coordinates inherit from the adjacent cell of the same row
// when both original color attributes agree (not merely their RGB values).
func decodeKittyPlaceholder(chars []rune, style vt.Style, col int, previous *kittyPlaceholder) (kittyPlaceholder, bool) {
	if len(chars) == 0 || chars[0] != kittyPlaceholderScalar {
		return kittyPlaceholder{}, false
	}
	p := kittyPlaceholder{col: col, fg: style.FG, underline: style.UnderlineColor}
	var marks [3]int
	n := 0
	for _, ch := range chars[1:] {
		if index, ok := kittyPlaceholderDiacriticIndex[ch]; ok && n < len(marks) {
			marks[n] = index
			n++
		}
	}
	if n > 0 {
		p.imageRow = marks[0]
	}
	if n > 1 {
		p.imageCol = marks[1]
	}
	if n > 2 {
		p.msb = marks[2]
	}
	if previous != nil && previous.col == col-1 && previous.fg == p.fg && previous.underline == p.underline {
		switch n {
		case 0:
			p.imageRow, p.imageCol, p.msb = previous.imageRow, previous.imageCol+1, previous.msb
		case 1:
			if previous.imageRow == p.imageRow {
				p.imageCol, p.msb = previous.imageCol+1, previous.msb
			}
		case 2:
			if previous.imageRow == p.imageRow && previous.imageCol+1 == p.imageCol {
				p.msb = previous.msb
			}
		}
	}
	p.imageID = placeholderColorID(p.fg) | uint32(min(p.msb, 255))<<24
	p.placementID = placeholderColorID(p.underline)
	return p, true
}

// Placeholder images always paint above text and below ordinary over-text
// images. The virtual record's z and crop are intentionally ignored, matching
// both OShell's CoreGraphics and Metal renderers. The whole image is aspect
// fitted and centered, then clipped to each placeholder cell without rounding
// the source crop (which would introduce seams between adjacent cells).
func (v *view) drawKittyPlaceholders(p *ui.Painter, ox, oy float32) {
	byImage := make(map[uint32][]vt.Graphic)
	for _, g := range v.graphics {
		if g.Virtual && g.Columns > 0 && g.Rows > 0 {
			byImage[g.LogicalImageID()] = append(byImage[g.LogicalImageID()], g)
		}
	}
	if len(byImage) == 0 {
		return
	}
	cw, ch := float32(v.cellW)/v.scale, float32(v.cellH)/v.scale
	grid := ui.Rect{X: ox, Y: oy, W: float32(v.cols) * cw, H: float32(v.rows) * ch}
	p.Clip(grid, 0, func() {
		for row, line := range v.lines {
			sx, sy, offset := lineTransform(line.mode, ch)
			anchor := oy + float32(row)*ch
			for _, cell := range line.placeholders {
				for _, g := range byImage[cell.imageID] {
					if cell.placementID != 0 && g.PlacementID != cell.placementID || cell.imageRow >= g.Rows || cell.imageCol >= g.Columns {
						continue
					}
					bitmap := v.graphicBitmaps[graphicBitmapKey{g.Generation, g.Orientation, g.Source}]
					if bitmap == nil || g.Source.Empty() {
						break
					}
					dest, source, visible := kittyPlaceholderGeometry(g, cell, float64(v.cellW)/float64(v.scale), float64(v.cellH)/float64(v.scale), float64(v.scale))
					if !visible {
						break
					}
					dest.X, dest.Y = ox+dest.X*sx, anchor+dest.Y*sy+offset
					dest.W, dest.H = dest.W*sx, dest.H*sy
					clip := ui.Rect{X: ox + float32(cell.col)*cw*sx, Y: anchor + offset, W: cw * sx, H: ch * sy}
					if sy == 2 {
						clip.Y, clip.H = anchor, ch
					}
					p.Clip(clip, 0, func() { p.BitmapRegionSubpixel(bitmap, dest, source) })
					break
				}
			}
		}
	})
}

func (v *view) hasKittyPlaceholder(g vt.Graphic) bool {
	for _, line := range v.lines {
		for _, cell := range line.placeholders {
			if cell.imageID == g.LogicalImageID() && (cell.placementID == 0 || cell.placementID == g.PlacementID) && cell.imageRow < g.Rows && cell.imageCol < g.Columns {
				return true
			}
		}
	}
	return false
}

// OShell's Metal path intersects in CGFloat before converting vertices/UVs
// to floats. Large virtual grids must not round away a one-cell difference
// before computing letterboxing; only the visible cell reaches the painter.
func kittyPlaceholderGeometry(g vt.Graphic, cell kittyPlaceholder, cw, ch, scale float64) (ui.Rect, ui.Rect, bool) {
	w, h := float64(g.Columns)*cw, float64(g.Rows)*ch
	sw, sh := float64(g.Source.Dx()), float64(g.Source.Dy())
	if w <= 0 || h <= 0 || sw <= 0 || sh <= 0 || scale <= 0 {
		return ui.Rect{}, ui.Rect{}, false
	}
	fit := min(w/sw, h/sh)
	iw, ih := sw*fit, sh*fit
	// Positive Y offsets move up in OShell's bottom-up drawing space.
	x := float64(cell.col-cell.imageCol)*cw + float64(g.X)/scale + (w-iw)/2
	y := -float64(cell.imageRow)*ch - float64(g.Y)/scale + (h-ih)/2
	left, top := max(float64(cell.col)*cw, x), max(0, y)
	right, bottom := min(float64(cell.col+1)*cw, x+iw), min(ch, y+ih)
	if right <= left || bottom <= top {
		return ui.Rect{}, ui.Rect{}, false
	}
	dst := ui.Rect{X: float32(left), Y: float32(top), W: float32(right - left), H: float32(bottom - top)}
	src := ui.Rect{X: float32((left - x) / iw), Y: float32((top - y) / ih), W: float32((right - left) / iw), H: float32((bottom - top) / ih)}
	return dst, src, true
}

package terminal

import (
	"github.com/egoist/mygo/ui"
	"image"
)

type graphicBitmapKey struct {
	generation  uint64
	orientation uint8
	source      image.Rectangle
}

// This runs on the UI thread after releasing the terminal write lock. Bitmaps
// own their pixels and are shared by all placements of the same generation, orientation and crop.
func (v *view) prepareGraphics() {
	if len(v.graphics) == 0 {
		v.graphicBitmaps = nil
		return
	}
	next := make(map[graphicBitmapKey]*ui.Bitmap)
	for i := range v.graphics {
		g := &v.graphics[i]
		if g.Virtual && !v.hasKittyPlaceholder(*g) {
			continue
		}
		if g.ResolveImage() == nil {
			continue
		}
		if g.Clipped && g.RasterClip.Empty() {
			continue
		}
		key := graphicBitmapKey{g.Generation, g.Orientation, g.Source}
		bitmap := next[key]
		if bitmap == nil {
			bitmap = v.graphicBitmaps[key]
		}
		if bitmap == nil {
			// Crop before resampling: clipping an enlarged full-image texture
			// lets interpolation bleed pixels from outside the requested crop.
			bitmap = ui.NewBitmap(g.Image.SubImage(g.Source))
		}
		next[key] = bitmap
	}
	v.graphicBitmaps = next
}

func (v *view) drawGraphics(p *ui.Painter, ox, oy float32, layer int) {
	if len(v.graphics) == 0 {
		return
	}
	grid := ui.Rect{X: ox, Y: oy, W: float32(v.cols*v.cellW) / v.scale, H: float32(v.rows*v.cellH) / v.scale}
	p.Clip(grid, 0, func() {
		for _, g := range v.graphics {
			if g.Virtual {
				continue
			}
			want := 3
			if g.Z < -1073741824 {
				want = 1
			} else if g.Z < 0 {
				want = 2
			}
			if want != layer {
				continue
			}
			bitmap := v.graphicBitmaps[graphicBitmapKey{g.Generation, g.Orientation, g.Source}]
			if bitmap == nil || g.Source.Empty() {
				continue
			}
			dest := ui.Rect{X: ox + float32(g.X)/v.scale, Y: oy + float32(g.Y)/v.scale, W: float32(g.Width) / v.scale, H: float32(g.Height) / v.scale}
			if g.ExactWidth > 0 && g.ExactHeight > 0 {
				dest.W, dest.H = float32(g.ExactWidth)/v.scale, float32(g.ExactHeight)/v.scale
			}
			ch := float32(v.cellH) / v.scale
			sx, sy, offset := lineTransform(g.LineMode, ch)
			anchor := oy + float32(g.AnchorRow)*ch
			dest.X, dest.Y = ox+(dest.X-ox)*sx, anchor+(dest.Y-anchor)*sy+offset
			dest.W, dest.H = dest.W*sx, dest.H*sy
			draw := func() {
				if g.ExactWidth > 0 || g.ExactHeight > 0 {
					p.BitmapSubpixel(bitmap, dest, ui.FillBox)
				} else {
					p.Image(bitmap, dest, ui.FillBox)
				}
			}
			paint := func() {
				if sy == 2 {
					p.Clip(ui.Rect{X: grid.X, Y: anchor, W: grid.W, H: ch}, 0, draw)
				} else {
					draw()
				}
			}
			if g.Clipped {
				if g.RasterClip.Empty() {
					continue
				}
				clip := ui.Rect{
					X: ox + float32(g.RasterClip.Min.X)/v.scale*sx,
					Y: anchor + (oy+float32(g.RasterClip.Min.Y)/v.scale-anchor)*sy + offset,
					W: float32(g.RasterClip.Dx()) / v.scale * sx,
					H: float32(g.RasterClip.Dy()) / v.scale * sy,
				}
				p.Clip(clip, 0, paint)
			} else {
				paint()
			}
		}
	})
}

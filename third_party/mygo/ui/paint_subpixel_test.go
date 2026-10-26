package ui

import (
	"image"
	"image/color"
	"testing"
)

func TestBitmapSubpixelRetainsFractionalCoverage(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	source.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	bitmap := NewBitmap(source)
	tt := NewTester(func(c *Context) {
		Box(c).Fill().Draw(func(p *Painter, r Rect) {
			p.Fill(r, RGB(0, 0, 255), 0)
			p.BitmapSubpixel(bitmap, Rect{X: 2, Y: 2, W: 8.25, H: 8.25}, FillBox)
			p.Image(bitmap, Rect{X: 22, Y: 2, W: 8.25, H: 8.25}, FillBox)
			p.BitmapSubpixel(nil, r, FillBox)
		})
	}, 40, 20)
	tt.Frame()
	for _, pos := range []image.Point{{10, 5}, {5, 10}} {
		got := tt.Image().RGBAAt(pos.X, pos.Y)
		if got.R < 62 || got.R > 66 || got.B < 189 || got.B > 193 || got.G != 0 || got.A != 255 {
			t.Fatalf("quarter-covered pixel %v: %v", pos, got)
		}
	}
	if got := tt.Image().RGBAAt(30, 5); got != (color.RGBA{0, 0, 255, 255}) {
		t.Fatal("ordinary Image no longer snaps to pixels", got)
	}
	tt.SetScale(2)
	for _, pos := range []image.Point{{20, 10}, {10, 20}} {
		got := tt.Image().RGBAAt(pos.X, pos.Y)
		if got.R < 126 || got.R > 130 || got.B < 125 || got.B > 129 || got.G != 0 || got.A != 255 {
			t.Fatalf("half-covered device pixel %v at scale 2: %v", pos, got)
		}
	}
}

func TestBitmapRegionSubpixelSharesSamplingAcrossCells(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 8, 6))
	for y := 0; y < 6; y++ {
		for x := 0; x < 8; x++ {
			source.SetNRGBA(x, y, color.NRGBA{uint8(x * 31), uint8(y * 43), uint8((x + y) * 17), 255})
		}
	}
	bitmap := NewBitmap(source)
	tt := NewTester(func(c *Context) {
		Box(c).Fill().Draw(func(p *Painter, r Rect) {
			p.Fill(r, RGB(0, 0, 255), 0)
			// The left image is a single fractional crop. On the right, two
			// adjacent cells sample exactly the same transform from one texture.
			p.BitmapRegionSubpixel(bitmap, Rect{2, 2, 24, 16}, Rect{.125, .125, .75, .75})
			p.BitmapRegionSubpixel(bitmap, Rect{32, 2, 12, 16}, Rect{.125, .125, .375, .75})
			p.BitmapRegionSubpixel(bitmap, Rect{44, 2, 12, 16}, Rect{.5, .125, .375, .75})
			p.BitmapRegionSubpixel(nil, r, Rect{0, 0, 1, 1})
			p.BitmapRegionSubpixel(bitmap, r, Rect{})
		})
	}, 60, 22)
	for _, scale := range []float32{1, 2} {
		tt.SetScale(scale)
		tt.Frame()
		im := tt.Image()
		for y := 2 * int(scale); y < 18*int(scale); y++ {
			for x := 2 * int(scale); x < 26*int(scale); x++ {
				a, b := im.RGBAAt(x, y), im.RGBAAt(x+30*int(scale), y)
				if a != b {
					t.Fatalf("scale %g sample %d,%d: full=%v split=%v", scale, x, y, a, b)
				}
			}
		}
	}
}

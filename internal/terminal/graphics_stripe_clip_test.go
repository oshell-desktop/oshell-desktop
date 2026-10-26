package terminal

import (
	"fmt"
	"image"
	imagecolor "image/color"
	"testing"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/terminal/internal/vt"
)

func stripeClipFixture() vt.Graphic {
	im := image.NewNRGBA(image.Rect(0, 0, 20, 40))
	for y := 0; y < 40; y++ {
		c := imagecolor.NRGBA{R: 255, A: 255}
		if y >= 20 {
			c = imagecolor.NRGBA{G: 255, A: 255}
		}
		for x := 0; x < 20; x++ {
			im.SetNRGBA(x, y, c)
		}
	}
	return vt.Graphic{Image: im, Generation: 1, Orientation: 1, Width: 20, Height: 40, Source: im.Rect, Clipped: true}
}

func renderStripeClips(v *view, layer int) *ui.Tester {
	v.prepareGraphics()
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Fill().Draw(func(p *ui.Painter, r ui.Rect) {
			p.Fill(r, ui.RGB(0, 0, 255), 0)
			v.drawGraphics(p, 3, 5, layer)
		})
	}, 100, 110)
	tt.Frame()
	return tt
}

func checkStripePixel(t *testing.T, tt *ui.Tester, x, y int, want imagecolor.RGBA) {
	t.Helper()
	if got := tt.Image().RGBAAt(x+3, y+5); got != want {
		t.Fatalf("grid pixel %d,%d: got %v, want %v", x, y, got, want)
	}
}

func TestGraphicStripeClipGapsOverlapAndScale(t *testing.T) {
	red, green, blue := imagecolor.RGBA{255, 0, 0, 255}, imagecolor.RGBA{0, 255, 0, 255}, imagecolor.RGBA{0, 0, 255, 255}
	for _, scale := range []float32{1, 2} {
		for _, secondRow := range []int{1, 6} {
			t.Run(fmt.Sprintf("scale=%g/row=%d", scale, secondRow), func(t *testing.T) {
				first := stripeClipFixture()
				second := first
				// Frozen 20-pixel stripes on a 10-pixel grid can overlap or
				// separate. They retain the same source sampling transform.
				first.RasterClip = image.Rect(0, 0, 20, 20)
				second.Y, second.AnchorRow = secondRow*10-20, secondRow
				second.RasterClip = image.Rect(0, secondRow*10, 20, secondRow*10+20)
				v := &view{cols: 4, rows: 10, cellW: 10, cellH: 10, scale: scale, graphics: []vt.Graphic{first, second}}
				tt := renderStripeClips(v, 3)
				if len(v.graphicBitmaps) != 1 {
					t.Fatal("stripes duplicated the shared source bitmap")
				}
				checkStripePixel(t, tt, int(5/scale), int(5/scale), red)
				checkStripePixel(t, tt, int(5/scale), int(float32(secondRow*10+5)/scale), green)
				if secondRow == 6 {
					checkStripePixel(t, tt, int(5/scale), int(35/scale), blue)
				}
				checkStripePixel(t, tt, int(5/scale), int(float32(secondRow*10+25)/scale), blue)
				checkStripePixel(t, tt, int(25/scale), int(5/scale), blue)
			})
		}
	}
}

func TestGraphicStripeClipLineModesAndLayer(t *testing.T) {
	red, blue := imagecolor.RGBA{255, 0, 0, 255}, imagecolor.RGBA{0, 0, 255, 255}
	for _, mode := range []vt.LineMode{vt.LineSingle, vt.LineDoubleWidth, vt.LineDoubleTop, vt.LineDoubleBottom} {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			g := stripeClipFixture()
			g.Y, g.AnchorRow, g.LineMode = 20, 1, mode
			g.RasterClip = image.Rect(5, 20, 15, 40)
			g.Z = -1
			v := &view{cols: 4, rows: 5, cellW: 10, cellH: 20, scale: 1, graphics: []vt.Graphic{g}}
			tt := renderStripeClips(v, 2)
			checkStripePixel(t, tt, 12, 25, red)
			checkStripePixel(t, tt, 2, 25, blue)
			checkStripePixel(t, tt, 12, 15, blue)
			checkStripePixel(t, tt, 12, 45, blue)
			tt = renderStripeClips(v, 3)
			checkStripePixel(t, tt, 12, 25, blue)
		})
	}
}

func TestGraphicStripeEmptyClipAndGridBoundary(t *testing.T) {
	blue, green := imagecolor.RGBA{0, 0, 255, 255}, imagecolor.RGBA{0, 255, 0, 255}
	g := stripeClipFixture()
	v := &view{cols: 1, rows: 1, cellW: 10, cellH: 20, scale: 1, graphics: []vt.Graphic{g}}
	tt := renderStripeClips(v, 3)
	checkStripePixel(t, tt, 5, 5, blue)
	if len(v.graphicBitmaps) != 0 {
		t.Fatal("empty stripe retained a bitmap")
	}
	g.Y = -20
	g.RasterClip = image.Rect(-10, -10, 30, 30)
	v.graphics = []vt.Graphic{g}
	tt = renderStripeClips(v, 3)
	checkStripePixel(t, tt, 5, 5, green)
	checkStripePixel(t, tt, 15, 5, blue)
	checkStripePixel(t, tt, 5, 25, blue)
}

package terminal

import (
	"fmt"
	imagecolor "image/color"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestTerminalKittyIndependentStripePixels(t *testing.T) {
	loadLib(t)
	term, err := New(Options{Conn: newPipe(), NoBlink: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { term.Close() })
	tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill() }, 400, 240)
	term.Feed([]byte("\x1b[?25l\x1b[48;2;0;0;255m\x1b[2J\x1b[H"))
	tt.Frame()
	w, h := term.v.cellW*2, term.v.cellH
	colors := []imagecolor.RGBA{{255, 0, 0, 255}, {0, 255, 0, 255}, {255, 255, 0, 255}}
	raw := make([]byte, w*h*3*3)
	for y := 0; y < h*3; y++ {
		c := colors[y/h]
		for x := 0; x < w; x++ {
			copy(raw[(y*w+x)*3:], []byte{c.R, c.G, c.B})
		}
	}
	feedGraphic(term, fmt.Sprintf("a=T,f=24,s=%d,v=%d,i=91,p=1,C=1", w, h*3), raw)
	term.Feed([]byte("\x1b[2;1H\x1b[L"))
	tt.Frame()
	blue := imagecolor.RGBA{0, 0, 255, 255}
	graphicColor(t, term, tt, 0, 0, colors[0])
	graphicColor(t, term, tt, 0, 1, blue)
	graphicColor(t, term, tt, 0, 2, colors[1])
	graphicColor(t, term, tt, 0, 3, colors[2])
	term.Feed([]byte("\x1b[3;1H\x1b[M"))
	tt.Frame()
	graphicColor(t, term, tt, 0, 0, colors[0])
	graphicColor(t, term, tt, 0, 2, colors[2])
	feedGraphic(term, "a=d,d=I,i=91", nil)
	tt.Frame()
	graphicColor(t, term, tt, 0, 0, blue)
	graphicColor(t, term, tt, 0, 2, blue)
	if len(term.v.graphics) != 0 || len(term.v.graphicBitmaps) != 0 {
		t.Fatal("deleted stripes retained frame or bitmap state")
	}
}

func TestTerminalKittyScaledStripesKeepSamplingAtSeams(t *testing.T) {
	loadLib(t)
	term, err := New(Options{Conn: newPipe(), NoBlink: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { term.Close() })
	tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill() }, 400, 240)
	term.Feed([]byte("\x1b[?25l"))
	tt.Frame()
	feedGraphic(term, "a=T,f=24,s=1,v=3,i=91,p=1,c=2,r=3,C=1", []byte{255, 0, 0, 0, 255, 0, 0, 0, 255})
	tt.Frame()
	cx, cy := cellCenter(term, 0, 0)
	w, h := int(float32(term.v.cellW)/term.v.scale), int(float32(term.v.cellH)/term.v.scale)
	x0, y0 := int(cx)-w/2, int(cy)-h/2
	before := make([]imagecolor.RGBA, 2*w*3*h)
	for y := 0; y < 3*h; y++ {
		for x := 0; x < 2*w; x++ {
			before[y*2*w+x] = tt.Image().RGBAAt(x0+x, y0+y)
		}
	}
	term.Feed([]byte("\x1b[2;1H\x1b[L"))
	tt.Frame()
	for sourceRow, targetRow := range []int{0, 2, 3} {
		for y := 0; y < h; y++ {
			for x := 0; x < 2*w; x++ {
				want := before[(sourceRow*h+y)*2*w+x]
				if got := tt.Image().RGBAAt(x0+x, y0+targetRow*h+y); got != want {
					t.Fatalf("stripe %d pixel %d,%d changed sampling after row split: %v != %v", sourceRow, x, y, got, want)
				}
			}
		}
	}
}

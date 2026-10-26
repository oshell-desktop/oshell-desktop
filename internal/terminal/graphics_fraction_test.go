package terminal

import (
	"fmt"
	imagecolor "image/color"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestTerminalKittyFractionalTailPixelsFollowTheirRow(t *testing.T) {
	loadLib(t)
	term, err := New(Options{Conn: newPipe(), NoBlink: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { term.Close() })
	tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill() }, 400, 240)
	term.Feed([]byte("\x1b[?25l\x1b[48;2;0;0;255m\x1b[2J\x1b[H"))
	tt.Frame()
	cw, ch := term.v.cellW, term.v.cellH
	sw, sh := 4*cw, 4*ch+1 // width=cw yields height=ch+0.25
	raw := make([]byte, sw*sh*3)
	for i := 0; i < len(raw); i += 3 {
		raw[i] = 255
	}
	feedGraphic(term, fmt.Sprintf("a=T,f=24,s=%d,v=%d,c=1,i=91,p=1,C=1", sw, sh), raw)
	tt.Frame()
	x, y := cellCenter(term, 0, 0)
	x0, y0 := int(x), int(y-float32(ch)/term.v.scale/2)
	checkTail := func(row int) {
		t.Helper()
		got := tt.Image().RGBAAt(x0, y0+row*ch)
		// A quarter of this pixel is red; the rest remains blue. An
		// integer-height transform either loses it or paints it fully.
		if got.R < 62 || got.R > 66 || got.G != 0 || got.B < 189 || got.B > 193 || got.A != 255 {
			t.Fatalf("fractional tail pixel in row %d: %v; cell=%dx%d scale=%g origin=%d,%d graphics=%+v nearby=%v/%v/%v", row, got, cw, ch, term.v.scale, x0, y0, term.v.graphics, tt.Image().RGBAAt(x0, y0+row*ch-1), tt.Image().RGBAAt(x0, y0+row*ch+1), tt.Image().RGBAAt(x0, y0+row*ch-2))
		}
		if got := tt.Image().RGBAAt(x0, y0+row*ch+1); got != (imagecolor.RGBA{0, 0, 255, 255}) {
			t.Fatal("transparent part of final stripe was stretched", got)
		}
	}
	checkTail(1)
	term.Feed([]byte("\x1b[2;1H\x1b[L"))
	tt.Frame()
	checkTail(2)
	if got := tt.Image().RGBAAt(x0, y0+ch); got != (imagecolor.RGBA{0, 0, 255, 255}) {
		t.Fatal("old stripe position retained pixels", got)
	}
	if len(term.v.graphicBitmaps) != 1 {
		t.Fatal("fractional stripes duplicated source bitmap")
	}
}

func TestTerminalKittyFractionalWidthClipsIntegerCanvas(t *testing.T) {
	loadLib(t)
	term, err := New(Options{Conn: newPipe(), NoBlink: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { term.Close() })
	tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill() }, 400, 240)
	term.Feed([]byte("\x1b[?25l\x1b[48;2;0;0;255m\x1b[2J\x1b[H"))
	tt.Frame()
	cw, ch := term.v.cellW, term.v.cellH
	sw, sh := 4*cw+3, 4*ch // height=ch yields width=cw+0.75
	raw := make([]byte, sw*sh*3)
	for i := 0; i < len(raw); i += 3 {
		raw[i] = 255
	}
	feedGraphic(term, fmt.Sprintf("a=T,f=24,s=%d,v=%d,r=1,i=91,p=1,C=1", sw, sh), raw)
	tt.Frame()
	x, y := cellCenter(term, 0, 0)
	x0 := int(x - float32(cw)/term.v.scale/2)
	if got := tt.Image().RGBAAt(x0+cw-1, int(y)); got != (imagecolor.RGBA{255, 0, 0, 255}) {
		t.Fatal("integer part of fractional canvas disappeared", got)
	}
	if got := tt.Image().RGBAAt(x0+cw, int(y)); got != (imagecolor.RGBA{0, 0, 255, 255}) {
		t.Fatal("fractional width escaped the integer bitmap canvas", got)
	}
}

package terminal

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	imagecolor "image/color"
	"image/png"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestTerminalRowImageTransparentTailRendering(t *testing.T) {
	loadLib(t)
	term, err := New(Options{Conn: newPipe(), NoBlink: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { term.Close() })
	view := ui.NewTester(func(c *ui.Context) { View(c, term).Fill() }, 400, 220)
	term.Feed([]byte("\x1b[?25l\x1b[48;2;0;0;255m\x1b[2J\x1b[H"))
	view.Frame()
	graphicColor(t, term, view, 0, 1, imagecolor.RGBA{0, 0, 255, 255})
	src := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			src.SetNRGBA(x, y, imagecolor.NRGBA{R: 255, A: 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, src); err != nil {
		t.Fatal(err)
	}
	term.Feed([]byte(fmt.Sprintf("\x1b]1337;File=inline=1;width=2;height=%dpx;preserveAspectRatio=0:%s\a", term.v.cellH+4, base64.StdEncoding.EncodeToString(b.Bytes()))))
	view.Frame()
	if len(term.v.graphics) != 2 || term.v.graphics[1].Height != term.v.cellH {
		t.Fatal("final stripe lacks its full row canvas")
	}
	graphicColor(t, term, view, 0, 0, imagecolor.RGBA{255, 0, 0, 255})
	graphicColor(t, term, view, 0, 1, imagecolor.RGBA{0, 0, 255, 255})
	x, y := cellCenter(term, 0, 1)
	y -= float32(term.v.cellH)/term.v.scale/2 - 1
	if got := view.Image().RGBAAt(int(x), int(y)); got != (imagecolor.RGBA{255, 0, 0, 255}) {
		t.Fatal("partial final row did not retain image pixels", got)
	}
}

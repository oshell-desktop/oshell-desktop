package terminal

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	imagecolor "image/color"
	"image/png"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestTerminalPNGOrientationBitmapIsolation(t *testing.T) {
	loadLib(t)
	term, err := New(Options{Conn: newPipe(), NoBlink: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { term.Close() })
	view := ui.NewTester(func(c *ui.Context) { View(c, term).Fill() }, 400, 220)
	im := image.NewNRGBA(image.Rect(0, 0, 40, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 40; x++ {
			c := imagecolor.NRGBA{R: 255, A: 255}
			if x >= 20 {
				c = imagecolor.NRGBA{B: 255, A: 255}
			}
			im.SetNRGBA(x, y, c)
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, im); err != nil {
		t.Fatal(err)
	}
	// EXIF orientation 2 (horizontal mirror), preserving dimensions and crop
	// bounds: generation+rectangle alone cannot identify the rendered bitmap.
	exif := []byte{'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x12, 1, 3, 0, 1, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0}
	chunk := make([]byte, 12+len(exif))
	binary.BigEndian.PutUint32(chunk, uint32(len(exif)))
	copy(chunk[4:], "eXIf")
	copy(chunk[8:], exif)
	binary.BigEndian.PutUint32(chunk[len(chunk)-4:], crc32.ChecksumIEEE(chunk[4:len(chunk)-4]))
	data := append([]byte(nil), encoded.Bytes()[:33]...)
	data = append(data, chunk...)
	data = append(data, encoded.Bytes()[33:]...)
	term.Feed([]byte("\x1b[?25l\x1b[1;1H"))
	feedGraphic(term, "a=T,f=100,i=7,p=1,C=1,c=4,r=2", data)
	term.Feed([]byte("\x1b[1;7H"))
	feedGraphic(term, "a=p,i=7,p=2,C=1,c=4,r=2,w=40,h=20", nil)
	view.Frame()
	graphicColor(t, term, view, 0, 0, imagecolor.RGBA{0, 0, 255, 255})
	graphicColor(t, term, view, 3, 0, imagecolor.RGBA{255, 0, 0, 255})
	graphicColor(t, term, view, 6, 0, imagecolor.RGBA{255, 0, 0, 255})
	graphicColor(t, term, view, 9, 0, imagecolor.RGBA{0, 0, 255, 255})
	if len(term.v.graphicBitmaps) != 2 {
		t.Fatal("raw and oriented texture cache collided")
	}
	feedGraphic(term, "a=d,d=i,i=7,p=1", nil)
	view.Frame()
	if len(term.v.graphicBitmaps) != 1 {
		t.Fatal("deleted orientation retained its bitmap")
	}
	graphicColor(t, term, view, 6, 0, imagecolor.RGBA{255, 0, 0, 255})
}

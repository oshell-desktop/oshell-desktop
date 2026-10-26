package terminal

import (
	"bytes"
	"encoding/base64"
	"image"
	imagecolor "image/color"
	"image/png"
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestTerminalInlineImageRendering(t *testing.T) {
	loadLib(t)
	term, err := New(Options{Conn: newPipe(), NoBlink: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { term.Close() })
	view := ui.NewTester(func(c *ui.Context) { View(c, term).Fill() }, 400, 220)
	im := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for y := range 2 {
		for x := range 2 {
			im.SetNRGBA(x, y, imagecolor.NRGBA{R: 255, A: 255})
		}
	}
	var encoded bytes.Buffer
	if err = png.Encode(&encoded, im); err != nil {
		t.Fatal(err)
	}
	term.Feed([]byte("\x1b[?25l\x1b]1337;File=inline=1;width=4;height=2;preserveAspectRatio=0:" + base64.StdEncoding.EncodeToString(encoded.Bytes()) + "\x07"))
	view.Frame()
	if len(term.v.graphics) == 0 && os.Getenv("OSHELL_REQUIRE_WIDTH_EXTENSION") != "1" {
		t.Skip("requires OShell inline-image extension")
	}
	graphicColor(t, term, view, 0, 0, imagecolor.RGBA{255, 0, 0, 255})
	graphicColor(t, term, view, 3, 1, imagecolor.RGBA{255, 0, 0, 255})
	term.Feed([]byte("\x1b[?2026h\x1b[2J"))
	view.Frame()
	graphicColor(t, term, view, 0, 0, imagecolor.RGBA{255, 0, 0, 255})
	term.Feed([]byte("\x1b[?2026l"))
	view.Frame()
	if len(term.v.graphics) != 0 || len(term.v.graphicBitmaps) != 0 {
		t.Fatal("inline image survived erase or retained bitmap cache")
	}
}

func TestTerminalInlineImageBeforeFirstView(t *testing.T) {
	loadLib(t)
	term, err := New(Options{Conn: newPipe(), NoBlink: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { term.Close() })
	var encoded bytes.Buffer
	if err = png.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	term.Feed([]byte("\x1b]1337;File=inline=1:" + base64.StdEncoding.EncodeToString(encoded.Bytes()) + "\x07"))
	term.mu.Lock()
	images := term.term.Graphics(8, 16)
	x, y := term.term.Cursor()
	term.mu.Unlock()
	if len(images) == 0 && os.Getenv("OSHELL_REQUIRE_WIDTH_EXTENSION") != "1" {
		t.Skip("requires OShell inline-image extension")
	}
	if len(images) != 1 || x != 0 || y != 1 {
		t.Fatal("image before first frame was discarded", len(images), x, y)
	}
	view := ui.NewTester(func(c *ui.Context) { View(c, term).Fill() }, 400, 220)
	view.Frame()
	if len(term.v.graphics) != 1 {
		t.Fatal("first frame lost the pending image")
	}
}

func TestTerminalSixelRendering(t *testing.T) {
	loadLib(t)
	term, err := New(Options{Conn: newPipe(), NoBlink: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { term.Close() })
	view := ui.NewTester(func(c *ui.Context) { View(c, term).Fill() }, 400, 220)
	term.Feed([]byte("\x1b[?25l\x1bPq#1;2;0;100;0" + strings.Repeat("!80~-", 8) + "\x1b\\"))
	view.Frame()
	if len(term.v.graphics) == 0 && os.Getenv("OSHELL_REQUIRE_WIDTH_EXTENSION") != "1" {
		t.Skip("requires OShell Sixel extension")
	}
	graphicColor(t, term, view, 0, 0, imagecolor.RGBA{0, 255, 0, 255})
	graphicColor(t, term, view, 2, 1, imagecolor.RGBA{0, 255, 0, 255})
	term.Feed([]byte("\x1b[2J"))
	view.Frame()
	if len(term.v.graphics) != 0 || len(term.v.graphicBitmaps) != 0 {
		t.Fatal("Sixel erase retained image or texture")
	}
}

func feedGraphic(term *Terminal, control string, pixels []byte) {
	term.Feed([]byte("\x1b_G" + control + ";" + base64.StdEncoding.EncodeToString(pixels) + "\x1b\\"))
}

func graphicColor(t *testing.T, term *Terminal, view *ui.Tester, col, row int, want imagecolor.RGBA) {
	t.Helper()
	x, y := cellCenter(term, col, row)
	got := view.Image().RGBAAt(int(x), int(y))
	if got != want {
		t.Fatalf("pixel (%d,%d) = %v, want %v", col, row, got, want)
	}
}

func TestTerminalGraphicsRenderAndSynchronizedUpdate(t *testing.T) {
	loadLib(t)
	term, err := New(Options{Conn: newPipe()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { term.Close() })
	view := ui.NewTester(func(c *ui.Context) { View(c, term).Fill().AutoFocus() }, 400, 220)
	term.Feed([]byte("\x1b[?25l"))
	feedGraphic(term, "a=T,f=24,s=2,v=1,i=11,c=4,r=2,C=1", []byte{255, 0, 0, 0, 255, 0})
	view.Frame()
	graphicColor(t, term, view, 0, 0, imagecolor.RGBA{255, 0, 0, 255})
	graphicColor(t, term, view, 3, 1, imagecolor.RGBA{0, 255, 0, 255})
	if len(term.v.graphicBitmaps) != 1 {
		t.Fatal("missing/shared bitmap cache")
	}
	save(t, view, "terminal-kitty-graphics")
	term.Feed([]byte("\x1b[?2026h"))
	feedGraphic(term, "a=t,f=24,s=2,v=1,i=11", []byte{0, 0, 255, 0, 0, 255})
	feedGraphic(term, "a=p,i=11,c=4,r=2,C=1", nil)
	view.Frame()
	graphicColor(t, term, view, 0, 0, imagecolor.RGBA{255, 0, 0, 255})
	term.Feed([]byte("\x1b[?2026l"))
	view.Frame()
	graphicColor(t, term, view, 0, 0, imagecolor.RGBA{0, 0, 255, 255})
	term.Feed([]byte("\x1b[H\x1b[?25h\x1b[2 q"))
	view.Frame()
	x, y := cellCenter(term, 0, 0)
	if view.Image().RGBAAt(int(x), int(y)) == (imagecolor.RGBA{0, 0, 255, 255}) {
		t.Fatal("image covered block cursor")
	}
	feedGraphic(term, "a=d,d=I,i=11", nil)
	view.Frame()
	if len(term.v.graphics) != 0 || len(term.v.graphicBitmaps) != 0 {
		t.Fatal("deleted image/texture retained")
	}
	feedGraphic(term, "a=T,f=24,s=1,v=1,i=12,c=1,r=1,C=1", []byte{255, 0, 0})
	view.Frame()
	term.Close()
	if len(term.v.graphics) != 0 || len(term.v.graphicBitmaps) != 0 {
		t.Fatal("close retained image/texture")
	}
}

func TestTerminalGraphicsCropAlphaAndZOrder(t *testing.T) {
	loadLib(t)
	term, err := New(Options{Conn: newPipe()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { term.Close() })
	view := ui.NewTester(func(c *ui.Context) { View(c, term).Fill() }, 400, 220)
	term.Feed([]byte("\x1b[?25l\x1b[48;2;255;0;0m\x1b[2J"))
	for _, tc := range []struct {
		z    string
		want imagecolor.RGBA
	}{
		{"-1073741825", imagecolor.RGBA{255, 0, 0, 255}},
		{"-1", imagecolor.RGBA{0, 255, 0, 255}},
		{"0", imagecolor.RGBA{0, 255, 0, 255}},
	} {
		feedGraphic(term, "a=d,d=A", nil)
		feedGraphic(term, "a=T,f=24,s=2,v=1,i=1,c=4,r=2,C=1,x=1,w=1,z="+tc.z, []byte{0, 0, 255, 0, 255, 0})
		view.Frame()
		graphicColor(t, term, view, 1, 0, tc.want)
	}
	feedGraphic(term, "a=d,d=A", nil)
	feedGraphic(term, "a=T,f=32,s=1,v=1,i=2,c=2,r=2,C=1", []byte{0, 0, 255, 128})
	view.Frame()
	x, y := cellCenter(term, 0, 0)
	got := view.Image().RGBAAt(int(x), int(y))
	if got.R < 125 || got.R > 129 || got.B < 126 || got.B > 130 || got.G != 0 || got.A != 255 {
		t.Fatal("non-premultiplied RGBA compositing", got)
	}
}

func TestTerminalGraphicsConcurrentCloseAndFirstView(t *testing.T) {
	loadLib(t)
	for i := 0; i < 25; i++ {
		term, err := New(Options{Conn: newPipe()})
		if err != nil {
			t.Fatal(err)
		}
		feedGraphic(term, "a=T,f=24,s=1,v=1,i=1,c=1,r=1,C=1", []byte{1, 2, 3})
		start, done := make(chan struct{}), make(chan struct{})
		go func() { <-start; term.Close(); close(done) }()
		first := true
		view := ui.NewTester(func(c *ui.Context) {
			if first {
				first = false
				close(start)
			}
			View(c, term).Fill()
		}, 160, 90)
		<-done
		view.Frame()
		if len(term.v.graphicBitmaps) != 0 || len(term.v.graphics) != 0 {
			t.Fatal("closed view retained graphics")
		}
	}
}

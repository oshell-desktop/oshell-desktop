package terminal

import (
	"image"
	imagecolor "image/color"
	"image/png"
	"os"
	"testing"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/terminal/internal/vt"
)

func TestTerminalDoubleLineRendering(t *testing.T) {
	loadLib(t)
	term, err := New(Options{Conn: newPipe(), NoBlink: true})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	view := ui.NewTester(func(c *ui.Context) { View(c, term).Fill() }, 640, 260)
	term.Feed([]byte("\x1b]11;#000000\a\x1b[?25l\x1b[38;2;255;0;0m\x1b[H█\x1b[2;1H\x1b#6█\x1b[3;1H\x1b#3▀▄\x1b[4;1H\x1b#4▀▄\x1b[5;1Hnormal"))
	view.Frame()
	red, black := imagecolor.RGBA{255, 0, 0, 255}, imagecolor.RGBA{0, 0, 0, 255}
	for _, c := range []struct {
		x, y  int
		color imagecolor.RGBA
	}{
		{0, 0, red}, {1, 0, black}, {0, 1, red}, {1, 1, red}, {2, 1, black},
		{0, 2, red}, {1, 2, red}, {2, 2, black}, {3, 2, black},
		{0, 3, black}, {1, 3, black}, {2, 3, red}, {3, 3, red},
	} {
		graphicColor(t, term, view, c.x, c.y, c.color)
	}
	if path := os.Getenv("OSHELL_LINE_MODE_PREVIEW"); path != "" {
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, view.Image())
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestTerminalDoubleLineGlyphScaleAndCursor(t *testing.T) {
	loadLib(t)
	term, err := New(Options{Conn: newPipe(), NoBlink: true})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	view := ui.NewTester(func(c *ui.Context) { View(c, term).Fill() }, 400, 200)
	term.Feed([]byte("\x1b]11;#000000\a\x1b[?25l\x1b[38;2;255;0;0m\x1b[HM\x1b[2;1H\x1b#6M"))
	view.Frame()
	ink := func(row int) image.Rectangle {
		im := view.Image()
		bounds := image.Rectangle{}
		found := false
		for y := term.v.oy + row*term.v.cellH; y < term.v.oy+(row+1)*term.v.cellH; y++ {
			for x := term.v.ox; x < term.v.ox+4*term.v.cellW; x++ {
				c := im.RGBAAt(x, y)
				if c.R > 160 && c.G < 40 && c.B < 40 {
					p := image.Rect(x, y, x+1, y+1)
					if !found {
						bounds = p
						found = true
					} else {
						bounds = bounds.Union(p)
					}
				}
			}
		}
		return bounds
	}
	a, b := ink(0), ink(1)
	if a.Empty() || b.Dx() < a.Dx()*2-2 || b.Dx() > a.Dx()*2+2 || abs(b.Dy()-a.Dy()) > 2 {
		t.Fatalf("glyph scaling: single=%v double=%v", a, b)
	}
	term.Feed([]byte("\x1b[?25h\x1b[2;3H"))
	view.Frame()
	v := term.v
	cw, ch := float32(v.cellW)/v.scale, float32(v.cellH)/v.scale
	r, _, _, _ := v.cursorRect(0, 0, cw, ch)
	if r.X != 4*cw || r.W != 2*cw || r.H != ch {
		t.Fatal("cursor does not follow double cells", r)
	}
}

func TestTerminalDoubleLineHitAndSelection(t *testing.T) {
	loadLib(t)
	term, err := New(Options{Conn: newPipe(), NoBlink: true})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	view := ui.NewTester(func(c *ui.Context) { View(c, term).Fill().AutoFocus() }, 500, 200)
	term.Feed([]byte("\x1b[2;1H\x1b#6alpha beta"))
	view.Frame()
	v := term.v
	x, y := float32(v.ox)+14.5*float32(v.cellW), float32(v.oy)+1.5*float32(v.cellH)
	term.mu.Lock()
	col, row := v.cellAt(x/v.scale, y/v.scale)
	term.mu.Unlock()
	if col != 7 || row != 1 {
		t.Fatal("double-width hit", col, row)
	}
	view.ClickAt(x/v.scale, y/v.scale)
	view.ClickAt(x/v.scale, y/v.scale)
	view.Frame()
	if got := term.SearchSelection(); got != "beta" {
		t.Fatal("double-click selected wrong word", got)
	}
	if v.lines[1].mode != vt.LineDoubleWidth {
		t.Fatal("row cache lost display mode")
	}
}

func TestTerminalDoubleLineGraphicsAndSynchronizedUpdate(t *testing.T) {
	loadLib(t)
	term, err := New(Options{Conn: newPipe(), NoBlink: true})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	view := ui.NewTester(func(c *ui.Context) { View(c, term).Fill() }, 400, 220)
	term.Feed([]byte("\x1b]11;#000000\a\x1b[?25l\x1b#6"))
	feedGraphic(term, "a=T,f=24,s=2,v=1,i=61,c=2,r=1,C=1", []byte{255, 0, 0, 0, 255, 0})
	view.Frame()
	red, green, black := imagecolor.RGBA{255, 0, 0, 255}, imagecolor.RGBA{0, 255, 0, 255}, imagecolor.RGBA{0, 0, 0, 255}
	graphicColor(t, term, view, 0, 0, red)
	graphicColor(t, term, view, 3, 0, green)
	term.Feed([]byte("\x1b[?2026h\x1b#5"))
	view.Frame()
	graphicColor(t, term, view, 3, 0, green)
	term.Feed([]byte("\x1b[?2026l"))
	view.Frame()
	graphicColor(t, term, view, 3, 0, black)
	term.Feed([]byte("\x1b[2J\x1b[2;1H\x1b#3"))
	feedGraphic(term, "a=T,f=24,s=1,v=2,i=62,c=1,r=1,C=1", []byte{255, 0, 0, 0, 255, 0})
	term.Feed([]byte("\x1b[3;1H\x1b#4"))
	feedGraphic(term, "a=T,f=24,s=1,v=2,i=63,c=1,r=1,C=1", []byte{255, 0, 0, 0, 255, 0})
	view.Frame()
	// Sample away from the bilinear transition at the two source pixels.
	x := term.v.ox + term.v.cellW/2
	for _, sample := range []struct {
		y    int
		want imagecolor.RGBA
	}{
		{term.v.oy + term.v.cellH + term.v.cellH/4, red},
		{term.v.oy + 2*term.v.cellH + 3*term.v.cellH/4, green},
	} {
		if got := view.Image().RGBAAt(x, sample.y); got != sample.want {
			t.Fatalf("double-height image pixel=%v want=%v", got, sample.want)
		}
	}
	graphicColor(t, term, view, 0, 3, black)
}

func TestTerminalDoubleLineMouseReporting(t *testing.T) {
	loadLib(t)
	conn := newPipe()
	term, err := New(Options{Conn: conn, NoBlink: true})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	view := ui.NewTester(func(c *ui.Context) { View(c, term).Fill().AutoFocus() }, 400, 200)
	term.Feed([]byte("\x1b[2;1H\x1b#6\x1b[?1002h\x1b[?1006h"))
	view.Frame()
	x, y := cellCenter(term, 8, 1)
	view.Press(x, y)
	want := "\x1b[<0;5;2M"
	if got := conn.take(len(want)); got != want {
		t.Fatalf("mouse press=%q want=%q", got, want)
	}
	view.Release(x, y)
	want = "\x1b[<0;5;2m"
	if got := conn.take(len(want)); got != want {
		t.Fatalf("mouse release=%q want=%q", got, want)
	}
}

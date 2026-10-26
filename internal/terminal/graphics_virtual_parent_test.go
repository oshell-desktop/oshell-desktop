package terminal

import (
	imagecolor "image/color"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestTerminalVirtualParentRendering(t *testing.T) {
	loadLib(t)
	term, err := New(Options{Conn: newPipe(), NoBlink: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { term.Close() })
	view := ui.NewTester(func(c *ui.Context) { View(c, term).Fill() }, 400, 220)
	term.Feed([]byte("\x1b[?25l\x1b[2;2H"))
	feedGraphic(term, "a=T,f=24,s=1,v=1,i=7,p=1,U=1", []byte{255, 0, 0})
	view.Frame()
	if len(term.v.graphics) != 1 || !term.v.graphics[0].Virtual || len(term.v.graphicBitmaps) != 0 {
		t.Fatal("virtual metadata without placeholder cells created a display bitmap")
	}
	feedGraphic(term, "a=T,f=24,s=1,v=1,i=8,p=1,P=7,Q=1,H=1,V=1,c=2,r=2", []byte{0, 0, 255})
	view.Frame()
	if len(term.v.graphics) != 2 || len(term.v.graphicBitmaps) != 1 {
		t.Fatal("missing child of virtual parent")
	}
	graphicColor(t, term, view, 2, 2, imagecolor.RGBA{0, 0, 255, 255})
	graphicColor(t, term, view, 3, 3, imagecolor.RGBA{0, 0, 255, 255})
	feedGraphic(term, "a=d,d=i,i=7,p=1", nil)
	view.Frame()
	if len(term.v.graphics) != 1 {
		t.Fatal("parent deletion removed attached child")
	}
	graphicColor(t, term, view, 2, 2, imagecolor.RGBA{0, 0, 255, 255})
	feedGraphic(term, "a=d,d=i,i=8,p=1", nil)
	view.Frame()
	if len(term.v.graphics) != 0 || len(term.v.graphicBitmaps) != 0 {
		t.Fatal("explicit child deletion retained bitmap")
	}
}

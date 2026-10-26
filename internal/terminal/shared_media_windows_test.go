package terminal

import (
	imagecolor "image/color"
	"testing"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/testsupport/winshm"
)

func TestI22WindowsSharedMemoryActuallyDraws(t *testing.T) {
	loadLib(t)
	term, err := New(Options{Conn: newPipe(), NoBlink: true})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	view := ui.NewTester(func(c *ui.Context) { View(c, term).Fill() }, 400, 220)
	term.Feed([]byte("\x1b[?25l"))
	m := winshm.New(t, []byte{255, 0, 0}, 0, 0)
	feedGraphic(term, "a=T,t=s,f=24,s=1,v=1,i=71,c=2,r=2,C=1", []byte("/"+m.Name))
	m.Close()
	if winshm.Exists(m.Name) {
		t.Fatal("view retains peer mapping")
	}
	view.Frame()
	graphicColor(t, term, view, 0, 0, imagecolor.RGBA{255, 0, 0, 255})
	graphicColor(t, term, view, 1, 1, imagecolor.RGBA{255, 0, 0, 255})
	// Same ID replacement and delete travel through the normal image lifecycle.
	n := winshm.New(t, []byte{0, 255, 0}, 0, 0)
	feedGraphic(term, "a=T,t=s,f=24,s=1,v=1,i=71,c=2,r=2,C=1", []byte(n.Name))
	n.Close()
	view.Frame()
	graphicColor(t, term, view, 0, 0, imagecolor.RGBA{0, 255, 0, 255})
	feedGraphic(term, "a=d,d=I,i=71", nil)
	view.Frame()
	if len(term.term.Graphics(term.size.cellW, term.size.cellH)) != 0 {
		t.Fatal("deleted shared image still retained")
	}
}

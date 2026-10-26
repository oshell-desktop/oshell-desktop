package terminal

import (
	imagecolor "image/color"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestTerminalRectangleCopyRedraw(t *testing.T) {
	loadLib(t)
	term, err := New(Options{Conn: newPipe(), NoBlink: true})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	view := ui.NewTester(func(c *ui.Context) { View(c, term).Fill() }, 640, 260)
	term.Feed([]byte("\x1b]11;#000000\a\x1b[?25l\x1b[38;2;255;0;0m\x1b[H█"))
	view.Frame()
	red, black := imagecolor.RGBA{255, 0, 0, 255}, imagecolor.RGBA{0, 0, 0, 255}
	graphicColor(t, term, view, 0, 0, red)
	graphicColor(t, term, view, 2, 2, black)
	term.Feed([]byte("\x1b[1;1;1;1;1;3;3;1;0$v"))
	view.Frame()
	graphicColor(t, term, view, 2, 2, red)
	term.Feed([]byte("\x1b[1;1;1;1$z"))
	view.Frame()
	graphicColor(t, term, view, 0, 0, black)
	graphicColor(t, term, view, 2, 2, red)
}

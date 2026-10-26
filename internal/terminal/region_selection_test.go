package terminal

import (
	"fmt"
	"testing"
)

func TestRegionScrollWordAndLineDrag(t *testing.T) {
	for _, clicks := range []int{2, 3} {
		t.Run(fmt.Sprint(clicks), func(t *testing.T) {
			term, tt, _ := bidiViewFixture(t)
			tt.Frame()
			term.Feed([]byte("\x1b[2J\x1b[H000\r\n111\r\naaa bbb\r\n333\r\n444\x1b[2;4r"))
			tt.Frame()
			x, y := cellCenter(term, 1, 2)
			for i := 1; i < clicks; i++ {
				tt.ClickAt(x, y)
			}
			tt.Press(x, y)
			term.Feed([]byte("\x1b[S"))
			tt.Frame()
			want := "aaa bbb"
			x, y = cellCenter(term, 5, 1)
			if clicks == 3 {
				x, y = cellCenter(term, 2, 2)
				want = "aaa bbb\n333"
			}
			tt.Move(x, y)
			if got := term.SearchSelection(); got != want {
				t.Fatalf("continued drag = %q, want %q", got, want)
			}
			tt.Release(x, y)
		})
	}
}

func TestRegionScrollViewCopyAndDrag(t *testing.T) {
	term, tt, _ := bidiViewFixture(t)
	tt.Frame()
	term.Feed([]byte("\x1b[2J\x1b[H000000\r\n111111\r\n222222\r\n333333\r\n444444\x1b[2;4r"))
	tt.Frame()
	x0, y0 := cellCenter(term, 0, 2)
	x3, y3 := cellCenter(term, 3, 2)
	beginCharacterDrag(tt, x0, y0)
	tt.Move(x3, y3)
	if got := term.SearchSelection(); got != "222" {
		t.Fatal("selection fixture", got)
	}
	term.Feed([]byte("\x1b[S"))
	tt.Frame()
	term.v.copy()
	if got := tt.Clipboard(); got != "222" {
		t.Fatal("scroll copied replacement text", got)
	}
	x5, y5 := cellCenter(term, 5, 1)
	tt.Move(x5, y5)
	if got := term.SearchSelection(); got != "22222" {
		t.Fatal("drag anchor did not move", got)
	}
	term.Feed([]byte("\x1b[S"))
	tt.Frame()
	tt.Move(x3, y3)
	term.v.copy()
	if got := term.SearchSelection(); got != "" {
		t.Fatal("discarded region selection revived", got)
	}
	if got := tt.Clipboard(); got != "222" {
		t.Fatal("cancelled copy replaced clipboard", got)
	}
	tt.Release(x3, y3)
}

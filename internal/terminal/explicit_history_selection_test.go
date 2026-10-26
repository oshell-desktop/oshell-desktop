package terminal

import (
	"fmt"
	"testing"
)

func TestExplicitHistoryClearViewCopyAndDrag(t *testing.T) {
	term, tt, _ := bidiViewFixture(t)
	tt.Frame()
	_, rows := term.Size()
	for i := 0; i < rows+4; i++ {
		term.Feed([]byte(fmt.Sprintf("%03dabc\r\n", i)))
	}
	term.mu.Lock()
	bar := term.term.Scrollbar()
	history := int(bar.Total - bar.Len)
	term.term.ScrollToTop()
	term.mu.Unlock()
	tt.Frame()
	x0, y0 := cellCenter(term, 0, 2)
	x3, y3 := cellCenter(term, 3, 2)
	beginCharacterDrag(tt, x0, y0)
	tt.Move(x3, y3)
	if got := term.SearchSelection(); got != "002" {
		t.Fatal("fixture", got)
	}
	term.Feed([]byte("\x1b[3J"))
	tt.Frame()
	want := fmt.Sprintf("%03d", history+2)
	term.v.copy()
	if got := tt.Clipboard(); got != want {
		t.Fatal("copy did not use logical row", got, want)
	}
	x5, y5 := cellCenter(term, 5, 2)
	tt.Move(x5, y5)
	term.v.copy()
	if got := tt.Clipboard(); got != want+"ab" {
		t.Fatal("drag used physical anchor", got, want+"ab")
	}
	tt.Release(x5, y5)
}

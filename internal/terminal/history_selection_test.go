package terminal

import (
	"fmt"
	"testing"
)

func TestHistoryRecycleCancelsViewCopyAndDrag(t *testing.T) {
	term, tt, _ := bidiViewFixture(t)
	tt.Frame()
	term.mu.Lock()
	_, rows := term.term.Size()
	term.term.SetHistoryLines(2)
	term.mu.Unlock()
	for i := 0; i < rows+1; i++ {
		term.Feed([]byte(fmt.Sprintf("%03d\r\n", i)))
	}
	term.mu.Lock()
	term.term.ScrollToTop()
	term.mu.Unlock()
	tt.Frame()
	x0, y0 := cellCenter(term, 0, 0)
	x1, y1 := cellCenter(term, 3, 1)
	beginCharacterDrag(tt, x0, y0)
	tt.Move(x1, y1)
	if got := term.SearchSelection(); got != "000\n001" {
		t.Fatal("drag fixture", got)
	}
	term.v.copy()
	copied := tt.Clipboard()
	if copied != "000\n001" {
		t.Fatal("copy fixture", copied)
	}
	term.Feed([]byte("next\r\n"))
	tt.Frame()
	if got := term.SearchSelection(); got != "" {
		t.Fatal("view retained discarded selection", got)
	}
	tt.Move(x1+float32(term.v.cellW), y1)
	term.v.copy()
	if got := term.SearchSelection(); got != "" {
		t.Fatal("pointer move revived discarded selection", got)
	}
	if got := tt.Clipboard(); got != copied {
		t.Fatal("copy replaced clipboard with recycled text", got)
	}
	tt.Release(x1, y1)
}

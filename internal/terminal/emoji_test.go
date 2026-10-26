package terminal

import (
	"github.com/egoist/mygo/ui"
	"testing"
)

func TestEmojiPreferenceKeepsGeometryAndContent(t *testing.T) {
	loadLib(t)
	term, e := New(Options{Conn: newPipe(), NoBlink: true})
	if e != nil {
		t.Fatal(e)
	}
	defer term.Close()
	term.mu.Lock()
	term.term.Write([]byte("A😀 ©\ufe0f 中\r\nunchanged history"))
	term.mu.Unlock()
	tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill() }, 480, 240)
	before := term.Text()
	cols, rows := term.Size()
	cw, ch, base := term.v.cellW, term.v.cellH, term.v.baseline
	for _, prefer := range []bool{true, false, true} {
		term.SetPreferColorEmoji(prefer)
		tt.Frame()
		c, r := term.Size()
		if c != cols || r != rows || term.v.cellW != cw || term.v.cellH != ch || term.v.baseline != base {
			t.Fatal("emoji preference changed geometry")
		}
		if term.Text() != before {
			t.Fatal("emoji preference changed terminal content")
		}
	}
}

func TestManagedClipboardShortcutDoesNotOverrideApp(t *testing.T) {
	loadLib(t)
	term, e := New(Options{Conn: newPipe(), ManagedClipboardBindings: true})
	if e != nil {
		t.Fatal(e)
	}
	defer term.Close()
	term.Feed([]byte("selected text"))
	tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill().AutoFocus() }, 400, 200)
	x0, y0 := cellCenter(term, 0, 0)
	x1, y1 := cellCenter(term, 7, 0)
	beginCharacterDrag(tt, x0-2, y0)
	tt.Move(x1, y1)
	tt.Release(x1+2, y1)
	tt.SetClipboard("sentinel")
	tt.Key(ui.Super, ui.KeyC)
	if tt.Clipboard() != "sentinel" {
		t.Fatal("widget retained application-disabled copy shortcut")
	}
	term.Edit("copy")
	if tt.Clipboard() == "sentinel" {
		t.Fatal("explicit application copy action stopped working")
	}
}

package terminal

import (
	"github.com/egoist/mygo/ui"
	"testing"
)

func TestIdleInputProtectsDraftPasswordAndFullScreen(t *testing.T) {
	loadLib(t)
	term, err := New(Options{Conn: newPipe()})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill().AutoFocus() }, 400, 200)
	term.Feed([]byte("custom> "))
	if !term.IdleInputSafe() {
		t.Fatal("custom prompt blocked")
	}
	tt.Type("partial")
	if term.IdleInputSafe() {
		t.Fatal("draft can receive idle text")
	}
	tt.Key(ui.Ctrl, ui.KeyC)
	if !term.IdleInputSafe() {
		t.Fatal("cancel did not resume idle")
	}
	term.Feed([]byte("\rPassword: "))
	if term.IdleInputSafe() {
		t.Fatal("password prompt can receive idle text")
	}
	term.Feed([]byte("\x1b[2J\x1b[H\x1b[?1049h"))
	if term.IdleInputSafe() {
		t.Fatal("full screen can receive idle text")
	}
	term.Feed([]byte("\x1b[?1049l\x1b[2J\x1b[Hcustom> "))
	if !term.IdleInputSafe() {
		t.Fatal("normal prompt not restored")
	}
	term.Feed([]byte("\x1b[?2004h"))
	term.Paste("echo unfinished\n")
	if term.IdleInputSafe() {
		t.Fatal("bracketed paste with newline was treated as a submitted command")
	}
	tt.Key(0, ui.KeyEnter)
	if !term.IdleInputSafe() {
		t.Fatal("Enter did not release draft protection")
	}
	term.Send([]byte("echo first\nunfinished"))
	if term.IdleInputSafe() {
		t.Fatal("draft after a pasted newline was not protected")
	}
}

func TestSelectionSurvivesUnrelatedProgressOutput(t *testing.T) {
	term, tt := wordClickFixture(t, "keep this selected\r\nprogress: 0%", 500)
	x, y := cellCenter(term, 0, 0)
	// End after the final character using Swift's half-open logical range.
	x1, y1 := cellCenter(term, 18, 0)
	beginCharacterDrag(tt, x-2, y)
	tt.Move(x1+3, y1)
	for i := 0; i < 10; i++ {
		term.Feed([]byte("\rprogress: 10%"))
		tt.Frame()
	}
	tt.Release(x1+3, y1)
	if got := tt.Clipboard(); got != "keep this selected" {
		t.Fatalf("output interrupted selection: %q", got)
	}
}

package desktop

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"oshell-desktop/internal/terminal"
)

func TestMovingTerminalRebindsFocusReports(t *testing.T) {
	first := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	second := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	p, conn := operatorPane(t, first, "focus")
	original := p.term
	first.current = 0
	content := func(app *App) func(*ui.Context) {
		return func(c *ui.Context) {
			app.refreshPaneVisibility()
			if len(app.tabs) > 0 {
				terminal.View(c, p.term).Bind(&p.focus).Fill().AutoFocus()
			} else {
				ui.Text(c, "empty window")
			}
		}
	}
	left := ui.NewTester(content(first), 400, 200)
	left.Frame()
	check := func(want string) {
		t.Helper()
		p.term.Send([]byte("|"))
		waitOperator(t, p.owner.Load(), func() bool { return len(conn.text()) >= len(want)+1 })
		if got := conn.text(); got != want+"|" {
			t.Fatalf("got %q want %q", got, want+"|")
		}
		conn.mu.Lock()
		conn.sent = nil
		conn.mu.Unlock()
	}
	p.term.Feed([]byte("\x1b[?1004h"))
	check("\x1b[I")
	first.transferTab(first.tabs[0], second)
	left.Frame()
	check("\x1b[O")
	if p.term != original || p.closed || p.owner.Load() != second {
		t.Fatal("move replaced or closed terminal")
	}
	right := ui.NewTester(content(second), 400, 200)
	right.Frame()
	check("\x1b[I")
	// The old window no longer owns this terminal's focus.
	left.SetFocused(false)
	left.SetFocused(true)
	check("")
	right.SetFocused(false)
	check("\x1b[O")
	p.term.Feed([]byte("\x1b[?1004h"))
	check("\x1b[O")
	right.SetFocused(true)
	check("\x1b[I")
}

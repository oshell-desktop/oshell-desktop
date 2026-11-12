package desktop

import (
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"oshell-desktop/internal/terminal"
	"runtime"
	"testing"
	"time"
)

func TestTerminalControlShortcutsReachFocusedTerminal(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	p, conn := operatorPane(t, a, "editing")
	a.current = 0
	// Existing user overrides must not steal terminal editing keys either.
	a.config.Settings.KeyboardShortcuts.Overrides = map[string]core.ShortcutOverride{
		"find":      {},
		"closePane": {Shortcut: &core.KeyboardShortcut{KeyCode: 13, Modifiers: 4}},
		"selectAll": {Shortcut: &core.KeyboardShortcut{KeyCode: 0, Modifiers: 4}},
	}
	tt := ui.NewTester(func(c *ui.Context) {
		terminal.View(c, p.term).Bind(&p.focus).Fill().AutoFocus()
		a.shortcuts(c)
	}, 500, 240)
	for _, key := range []ui.Key{ui.KeyW, ui.KeyA, ui.KeyE, ui.KeyU, ui.KeyK, ui.KeyR, ui.KeyD, ui.KeyF} {
		tt.Key(ui.Ctrl, key)
	}
	want := "\x17\x01\x05\x15\x0b\x12\x04\x06"
	deadline := time.Now().Add(time.Second)
	for len(conn.text()) < len(want) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if conn.text() != want || len(a.tabs) != 1 || a.features.sessionClose != nil || a.hasOpenTerminalSearch() || a.authOpen {
		t.Fatalf("editing intercepted: sent %q", conn.text())
	}
}

func TestTerminalSearchAndCommandEnterShortcuts(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("Windows/Linux workspace bindings")
	}
	cfg := core.DefaultConfig()
	cfg.Settings.QuickSendVisible = true
	a := newApp(&core.Store{Dir: t.TempDir()}, cfg)
	p, conn := operatorPane(t, a, "editing")
	a.selectTab(a.tabs[0])
	p.term.Feed([]byte("search-fixture"))
	tt := ui.NewTester(a.view, 1150, 780)
	if err := tt.Click("Terminal"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Ctrl, ui.KeyF)
	tt.Frame()
	if _, ok := tt.Find("搜索终端文本"); !ok {
		t.Fatal("Ctrl+F did not open terminal search")
	}
	tt.Type("search-fixture")
	if p.search.query != "search-fixture" {
		t.Fatal("search input did not receive focus")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if p.search.open {
		t.Fatal("Escape did not close search")
	}
	tt.Key(ui.Shift, ui.KeyEnter)
	tt.Key(ui.Ctrl, ui.KeyEnter)
	p.term.Send([]byte("|"))
	waitOperator(t, a, func() bool { return conn.text() == "\r\r|" })
	for _, mod := range []ui.Modifiers{ui.Ctrl, ui.Shift} {
		if err := tt.Click("快捷命令"); err != nil {
			t.Fatal(err)
		}
		tt.Type("quick")
		tt.Key(mod, ui.KeyEnter)
		if a.quick != "" {
			t.Fatal("quick send did not submit the command")
		}
	}
	a.openComposer("multi", true)
	tt.Frame()
	if err := tt.Click("撰写命令内容"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Ctrl, ui.KeyEnter)
	tt.Key(ui.Shift, ui.KeyEnter)
	p.term.Send([]byte("|"))
	want := "\r\r|quick\rquick\rmulti\rmulti\r|"
	waitOperator(t, a, func() bool { return conn.text() == want })
	if a.features.operators.composerText != "multi" || len(a.tabs) != 1 || a.authOpen {
		t.Fatal("modified Enter edited the command or opened another session")
	}
}

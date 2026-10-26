package desktop

import (
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"oshell-desktop/internal/terminal"
	"testing"
	"time"
)

func TestTerminalControlShortcutsReachFocusedTerminal(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	p, conn := operatorPane(t, a, "editing")
	a.current = 0
	// Existing user overrides must not steal terminal editing keys either.
	a.config.Settings.KeyboardShortcuts.Overrides = map[string]core.ShortcutOverride{
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

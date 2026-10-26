package desktop

import (
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"strings"
	"testing"
)

func TestLockedAuxiliaryWindowsHideConnectionData(t *testing.T) {
	cfg := core.DefaultConfig()
	profile := core.NewProfile()
	profile.Name = "PRIVATE_SESSION_NAME"
	profile.Host = "private-host.invalid"
	cfg.Profiles = []core.Profile{profile}
	a := newApp(&core.Store{Dir: t.TempDir()}, cfg)
	if e := a.vault.SetMaster("synthetic-lock-test"); e != nil {
		t.Fatal(e)
	}
	p := &pane{id: core.NewID(), profile: profile, status: "PRIVATE_REMOTE_DIRECTORY"}
	a.tabs = []*tab{{id: core.NewID(), name: profile.Name, active: p, root: &node{pane: p}}}
	a.current = 0
	a.prepareSettings()
	a.vault.Lock()
	a.workspaceAuthorized = false // Simulate a fresh, not-yet-authorized startup.
	for name, view := range map[string]func(*ui.Context){"workspace": a.view, "files": a.fileWindowView, "manager": a.managerView, "settings": a.settingsView} {
		tt := ui.NewTester(view, 1000, 700)
		text := strings.Join(tt.Texts(), "\n")
		if !strings.Contains(text, "OShell 已锁定") || strings.Contains(text, "PRIVATE_") || strings.Contains(text, profile.Host) {
			t.Fatalf("%s leaked protected data: %s", name, text)
		}
	}
	if e := a.vault.Unlock("synthetic-lock-test"); e != nil {
		t.Fatal(e)
	}
	tt := ui.NewTester(a.fileWindowView, 1000, 700)
	if !tt.HasText("private-host.invalid") || !tt.HasText("PRIVATE_REMOTE_DIRECTORY") {
		t.Fatal("unlock did not restore file view", tt.Texts())
	}
}

func TestLockedWorkspaceMarksTerminalInvisibleWithoutDroppingOutput(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	p, _ := operatorPane(t, a, "fixture")
	a.refreshPaneVisibility()
	if !p.visible.Load() {
		t.Fatal("active pane unexpectedly invisible")
	}
	if e := a.vault.SetMaster("fixture-lock-password"); e != nil {
		t.Fatal(e)
	}
	a.vault.Lock()
	a.workspaceAuthorized = false // Simulate a fresh, not-yet-authorized startup.
	a.refreshPaneVisibility()
	if p.visible.Load() || len(a.visiblePanes()) != 0 {
		t.Fatal("locked pane still considered visible")
	}
	p.term.Feed([]byte("OUTPUT_WHILE_LOCKED"))
	if !strings.Contains(p.term.Text(), "OUTPUT_WHILE_LOCKED") {
		t.Fatal("hidden terminal dropped output")
	}
	if e := a.vault.Unlock("fixture-lock-password"); e != nil {
		t.Fatal(e)
	}
	a.refreshPaneVisibility()
	if !p.visible.Load() {
		t.Fatal("unlock failed to restore visibility")
	}
}

func TestLockedApplicationStillAllowsWindowAndQuitActions(t *testing.T) {
	for _, action := range []string{"quit", "hide", "minimize"} {
		if !lockedWindowAction(action) {
			t.Fatal("locked application traps window action", action)
		}
	}
	for _, action := range []string{"newSession", "files", "copy", "paste", "closePane", "sessionManager"} {
		if lockedWindowAction(action) {
			t.Fatal("protected action bypasses lock", action)
		}
	}
}

package desktop

import (
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"strings"
	"testing"
	"time"
)

func TestQuickSwitchScopeAndStableSelection(t *testing.T) {
	a := parityApp(t)
	first, _ := operatorPane(t, a, "opened one")
	second, _ := operatorPane(t, a, "opened two")
	profile := core.NewProfile()
	profile.Name = "saved fixture"
	profile.Host = "example.invalid"
	a.config.Profiles = []core.Profile{profile}
	a.showQuickSwitch()
	f := &a.features.switcher
	f.scope = "opened"
	entries := a.switchEntries()
	if len(entries) != 2 {
		t.Fatal("opened scope", entries)
	}
	f.selectionKey = entries[1].key()
	f.reconcileSelection([]switchEntry{entries[1], entries[0]})
	if f.selected != 0 {
		t.Fatal("selection did not follow identity")
	}
	f.query = "one"
	entries = a.switchEntries()
	f.reconcileSelection(entries)
	if len(entries) != 1 || entries[f.selected].Pane != first {
		t.Fatal("removed selection not reset")
	}
	f.query = ""
	f.scope = "saved"
	entries = a.switchEntries()
	if len(entries) != 1 || entries[0].Pane != nil || entries[0].Profile.ID != profile.ID {
		t.Fatal("saved scope", entries)
	}
	f.scope = "all"
	entries = a.switchEntries()
	if len(entries) != 3 || entries[1].Pane != second {
		t.Fatal("all scope", entries)
	}
}

func TestPaneZoomKeepsLayoutAndQuickSwitchRevealsHiddenGroup(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	first, _ := operatorPane(t, a, "prod.example")
	second, _ := operatorPane(t, a, "test.example")
	workspace := a.tabs[0]
	workspace.root = &node{id: core.NewID(), first: workspace.root, second: a.tabs[1].root, size: 333}
	a.tabs = a.tabs[:1]
	a.ensureGroups()
	root := workspace.root
	a.togglePaneZoom()
	if workspace.zoomed != first || workspace.root != root || root.size != 333 {
		t.Fatal("zoom changed layout")
	}
	a.cyclePane(1)
	if workspace.zoomed != second || workspace.active != second {
		t.Fatal("navigation did not follow zoom")
	}
	a.togglePaneZoom()
	if workspace.zoomed != nil || workspace.root != root || root.size != 333 {
		t.Fatal("zoom restoration lost ratios")
	}
	group, e := a.createGroup("生产分组")
	if e != nil {
		t.Fatal(e)
	}
	a.moveToGroup(workspace, group)
	a.hideGroup(group, true)
	a.features.switcher.query = "prod 生产"
	entries := a.switchEntries()
	if len(entries) != 1 || entries[0].Pane != first {
		t.Fatal("multi-term search did not include hidden panes")
	}
	a.activatePane(entries[0].Tab, entries[0].Pane)
	if group.hidden || a.activePane() != first {
		t.Fatal("quick switch did not reveal hidden group")
	}
	a.togglePaneZoom()
	second.pendingUnread.Store(true)
	a.refreshPaneVisibility()
	if !workspace.unread || second.visible.Load() {
		t.Fatal("zoom-hidden output lost unread state")
	}
	a.togglePaneZoom()
	a.refreshPaneVisibility()
	if workspace.unread || !second.visible.Load() {
		t.Fatal("visible output did not clear unread state")
	}
}
func TestClockOverlayAndTimerLifecycle(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	p, _ := operatorPane(t, a, "clock")
	a.config.Settings.TerminalClockEnabled = true
	a.clockText = "2026-10-10 01:23:45"
	p.term.Feed([]byte("terminal-only"))
	tt := ui.NewTester(a.view, 1000, 680)
	snapshot(t, tt, "terminal-clock.png")
	if strings.Contains(p.term.Text(), a.clockText) {
		t.Fatal("clock entered terminal history")
	}
	var clock workspaceClock
	clock.update(map[*App]bool{a: true})
	clock.mu.Lock()
	running := clock.stop != nil
	clock.mu.Unlock()
	if !running {
		t.Fatal("clock did not start")
	}
	clock.update(nil)
	clock.mu.Lock()
	running = clock.stop != nil
	clock.mu.Unlock()
	if running {
		t.Fatal("clock kept running without subscribers")
	}
	text := core.LocalClockText(time.Now())
	if len(text) != 19 || text[4] != '-' || text[10] != ' ' {
		t.Fatal("clock format", text)
	}
}

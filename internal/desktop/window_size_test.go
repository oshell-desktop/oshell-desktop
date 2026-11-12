package desktop

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"testing"
)

func TestStartupWindowSettingsAndRestore(t *testing.T) {
	store := &core.Store{Dir: t.TempDir()}
	a := newApp(store, core.DefaultConfig())
	if a.sidebar.shown || a.startupSize(nil) != defaultWindowSize {
		t.Fatal("fresh window defaults")
	}
	a.normalWindowSize = windowSize{994, 522}
	a.saveLastWindowSize()
	a.prepareSettings()
	a.features.settingsTab = 10
	tt := ui.NewTester(a.settingsView, 1080, 760)
	if err := tt.Click("固定启动窗口大小"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][2]string{{"759", "460"}, {"760", "459"}, {"10001", "760"}, {"760", "1.5"}} {
		a.windowDraft.width, a.windowDraft.height = bad[0], bad[1]
		a.applySettings()
		if !a.settingsOpen || a.features.settingsError == "" || a.localUI.StartupWindow != nil {
			t.Fatal("invalid size accepted", bad)
		}
	}
	a.windowDraft.width, a.windowDraft.height = "760", "460"
	a.applySettings()
	reopened := newApp(store, core.DefaultConfig())
	if size := reopened.startupSize(a); size != (windowSize{760, 460}) {
		t.Fatal("fixed size lost", size)
	}
	reopened.prepareSettings()
	reopened.windowDraft.fixed = false
	reopened.closeSettings()
	if size := reopened.startupSize(nil); size != (windowSize{760, 460}) {
		t.Fatal("cancel changed fixed size", size)
	}
	reopened.prepareSettings()
	reopened.windowDraft.fixed = false
	reopened.applySettings()
	if size := newApp(store, core.DefaultConfig()).startupSize(nil); size != a.normalWindowSize {
		t.Fatal("remembered size lost", size)
	}
	work := mygo.Rectangle{X: -1600, Y: 10, Width: 1600, Height: 900}
	got := fitWorkspaceWindow(mygo.Rectangle{X: -900, Y: 400, Width: 1016, Height: 638}, work, windowSize{1000, 600}, windowSize{10000, 10000})
	if got != work {
		t.Fatal("oversized window not clamped on secondary screen", got)
	}
}

func TestStartupSidebarPreferenceIsLocalToNewAndOwningWindows(t *testing.T) {
	store := &core.Store{Dir: t.TempDir()}
	cfg := core.DefaultConfig()
	a, b := newApp(store, cfg), newApp(store, cfg.Clone())
	a.hub = &applicationHub{windows: []*App{a, b}, store: store, config: cfg, vault: a.vault}
	a.prepareSettings()
	tt := ui.NewTester(a.settingsPage, 1080, 760)
	if err := tt.Click("展开左侧会话栏"); err != nil {
		t.Fatal(err)
	}
	a.applySettings()
	if !a.sidebar.shown || b.sidebar.shown || !newApp(store, cfg).sidebar.shown {
		t.Fatal("sidebar visibility did not respect window scope")
	}
}

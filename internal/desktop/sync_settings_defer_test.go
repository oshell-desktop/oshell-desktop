package desktop

import (
	"context"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"testing"
)

func TestSettingsOpenDuringSyncCommit(t *testing.T) {
	for _, separate := range []bool{false, true} {
		store := &core.Store{Dir: t.TempDir()}
		owner := newApp(store, core.DefaultConfig())
		editor := owner
		hub := &applicationHub{store: store, windows: []*App{owner}, syncOwner: owner}
		owner.hub = hub
		if separate {
			editor = newApp(store, owner.config)
			editor.hub = hub
			hub.windows = append(hub.windows, editor)
		}
		f := &owner.features.sharedSync
		ctx, cancel := context.WithCancel(context.Background())
		f.busy, f.plan, f.cancel = true, &core.SharedSyncPlan{}, cancel
		owner.setConfigurationBusy(true)
		editor.showPreferenceSection(3)
		if !editor.settingsOpen || !editor.settingsWaitingForSync {
			t.Fatal("settings did not open during commit")
		}
		tt := ui.NewTester(editor.settingsView, 1000, 700)
		if err := tt.Click("常规"); err != nil {
			t.Fatal(err)
		}
		original := editor.config.Settings.FontSize
		_ = tt.Click("增大终端字号")
		_ = tt.Click("应用")
		if !editor.settingsOpen || editor.features.settingsDraft.FontSize != original || editor.config.Settings.FontSize != original {
			t.Fatal("modified a read-only snapshot")
		}
		if separate {
			if err := tt.Click("取消本次同步"); err != nil || ctx.Err() != context.Canceled || !editor.settingsOpen {
				t.Fatalf("cancel while browsing: %v", err)
			}
		} else {
			cancel()
		}
		f.busy, f.cancel, f.plan = false, nil, nil
		owner.setConfigurationBusy(false)
		next := owner.config.Clone()
		next.Settings.FontSize = original + 2
		owner.applyCommittedConfig(next)
		tt.Frame()
		if editor.settingsWaitingForSync || editor.features.settingsDraft.FontSize != next.Settings.FontSize {
			t.Fatal("stale settings after commit")
		}
		if err := tt.Click("增大终端字号"); err != nil {
			t.Fatal(err)
		}
		editor.openSettings()
		if editor.features.settingsDraft.FontSize != next.Settings.FontSize+1 {
			t.Fatal("editing did not resume or reopening lost draft")
		}
		editor.closeSettings()
	}
}

func TestSyncReadResultDefersAcrossWindows(t *testing.T) {
	store := &core.Store{Dir: t.TempDir()}
	owner := newApp(store, core.DefaultConfig())
	editor := newApp(store, core.DefaultConfig())
	hub := &applicationHub{windows: []*App{owner, editor}, settingsOwner: editor}
	owner.hub = hub
	editor.hub = hub
	editor.prepareSettings()
	editor.features.settingsDraft.FontSize = 21
	owner.features.sharedSync.plan = &core.SharedSyncPlan{}
	owner.features.sharedSync.master = []byte("synthetic-master")
	if !owner.deferSyncForSettings(true) {
		t.Fatal("other-window settings not detected")
	}
	if owner.features.sharedSync.plan != nil || len(owner.features.sharedSync.master) != 0 || owner.features.sharedSync.conflictOpen {
		t.Fatal("obsolete plan retained")
	}
	if editor.features.settingsDraft.FontSize != 21 {
		t.Fatal("draft overwritten")
	}
	editor.closeSettings()
	if owner.features.sharedSync.deferred {
		t.Fatal("deferred read not resumed")
	}
	if owner.features.sharedSync.status != "请先保存并启用同步设置" {
		t.Fatal("did not re-read saved options")
	}
}

func TestSyncStartWhileSettingsOpenDoesNotRead(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	a.prepareSettings()
	a.startSharedSync()
	if !a.features.sharedSync.deferred || a.features.sharedSync.busy {
		t.Fatal("read started during editing")
	}
	a.features.sharedSync.deferred = false // Cancel This Operation.
	a.closeSettings()
	if a.features.sharedSync.status == "请先保存并启用同步设置" {
		t.Fatal("cancelled read restarted")
	}
}

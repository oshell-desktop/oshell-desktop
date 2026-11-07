package desktop

import (
	"testing"
	"time"

	"github.com/egoist/mygo"
	"oshell-desktop/internal/core"
)

func TestManagerDefersSyncAcrossWindows(t *testing.T) {
	for _, phase := range []string{"start", "prepared", "commit", "activation"} {
		t.Run(phase, func(t *testing.T) {
			store := &core.Store{Dir: t.TempDir()}
			cfg := core.DefaultConfig()
			if err := store.Save(cfg); err != nil {
				t.Fatal(err)
			}
			owner := newApp(store, cfg)
			editor := newApp(store, cfg)
			hub := &applicationHub{store: store, config: cfg, vault: owner.vault, windows: []*App{owner, editor}}
			owner.hub, editor.hub = hub, hub
			// Window presence is the production gate. No native window is created.
			editor.manager = &mygo.Window{}
			defer func() { editor.manager = nil }()
			f := &owner.features.sharedSync
			switch phase {
			case "start":
				f.background = true
				owner.startSharedSync()
			case "prepared":
				f.plan = &core.SharedSyncPlan{}
				f.master = []byte("synthetic-master")
				owner.deferSyncForSettings(true)
			case "commit":
				if !owner.syncEditingBlocked() {
					t.Fatal("manager must block commit before configuration becomes busy")
				}
				f.plan = &core.SharedSyncPlan{}
				owner.commitSharedSync()
			case "activation":
				if err := core.SaveSyncOptions(store.Dir, core.SyncOptions{Version: 1, Enabled: true, Backend: "directory", Address: t.TempDir()}); err != nil {
					t.Fatal(err)
				}
				hub.refreshOnActivation()
				if f.busy || f.status != "" || owner.configurationBusy() {
					t.Fatal("activation started sync while manager is open", f.status)
				}
				return
			}
			if !f.deferred || f.busy || owner.configurationBusy() || f.plan != nil || len(f.master) != 0 {
				t.Fatal("sync not deferred before taking configuration lock", f.status)
			}
			editor.manager = nil
			owner.resumeDeferredSyncAfterSettings()
			if f.deferred || f.status != "请先保存并启用同步设置" {
				t.Fatal("closed manager did not release deferred sync", f.status)
			}
		})
	}
}

func TestManagerPasteKeepsAutomaticSyncQueued(t *testing.T) {
	p := core.DefaultSyncPolicy()
	p.CheckInterval = 0
	a, h := policyFixture(t, p)
	a.manager = &mygo.Window{}
	defer func() { a.manager = nil }()
	h.armSharedSync(time.Millisecond)
	first := h.syncTimer
	waitOperator(t, a, func() bool { return h.syncTimer != first })
	if h.syncTimer == nil || a.features.sharedSync.busy || a.configurationBusy() {
		t.Fatal("automatic sync interrupted manager instead of retaining queued changes")
	}
}

func TestManagerCloseClearsRenameAndPreservesReplacement(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	old, replacement := &mygo.Window{}, &mygo.Window{}
	a.manager = replacement
	rename := &managerRenameState{name: "unsaved"}
	a.managerRename = rename
	a.features.sharedSync.deferred = true
	a.managerClosed(old)
	if a.manager != replacement || a.managerRename != rename || !a.features.sharedSync.deferred {
		t.Fatal("stale close disturbed replacement manager")
	}
	a.managerClosed(replacement)
	if a.manager != nil || a.managerRename != nil || a.features.sharedSync.deferred {
		t.Fatal("native close stranded rename or deferred sync")
	}
}

func TestManagerCloseToEditorKeepsSyncDeferred(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	w := &mygo.Window{}
	a.manager = nil // closeManager detaches before the deferred native close.
	a.editing = true
	a.features.sharedSync.deferred = true
	a.managerClosed(w)
	if !a.features.sharedSync.deferred || a.features.sharedSync.busy {
		t.Fatal("manager-to-editor transition started sync")
	}
}

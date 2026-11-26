package desktop

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

type pausedSessionUpload struct {
	core.DirectoryTransport
	started, release chan struct{}
}

func TestManagerLegacyProfileIDsRemainCaseSensitive(t *testing.T) {
	p := core.NewProfile()
	p.ID, p.Host = "legacy", "fixture.invalid"
	second := p
	second.ID = "LEGACY"
	a := &App{config: core.Config{Profiles: []core.Profile{p, second}}}
	if got, ok := a.profileByID(second.ID); !ok || got.ID != second.ID {
		t.Fatal("selected another legacy profile")
	}
	if _, ok := a.profileByID("Legacy"); ok {
		t.Fatal("unexpected match for missing legacy ID")
	}
}

func (s pausedSessionUpload) Put(ctx context.Context, data []byte, etag string) error {
	close(s.started)
	select {
	case <-s.release:
		return s.DirectoryTransport.Put(ctx, data, etag)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestManagerSaveWhileDirectoryUploadInFlight(t *testing.T) {
	a, _ := managerClipboardFixture(t)
	p := a.config.Profiles[0]
	remote := pausedSessionUpload{core.DirectoryTransport{Directory: t.TempDir()}, make(chan struct{}), make(chan struct{})}
	master, err := a.vault.MasterSecret()
	if err != nil {
		t.Fatal(err)
	}
	selection := core.SyncContentSelection{Sessions: true}
	engine := core.SharedSyncEngine{Store: a.store, Vault: a.vault, Transport: remote, Selection: &selection,
		Replica: core.SharedReplicaStore{Directory: a.store.Dir, Target: remote.Directory}}
	plan, err := engine.Prepare(context.Background(), a.config, master, true)
	if err != nil {
		t.Fatal(err)
	}
	f := &a.features.sharedSync
	f.plan, f.master = plan, master
	a.commitSharedSync()
	defer func() {
		if f.cancel != nil {
			f.cancel()
		}
	}()
	select {
	case <-remote.started:
	case <-time.After(10 * time.Second):
		t.Fatal("sync did not reach conditional upload")
	}
	a.editManagerProfile(p)
	a.draft.Name = "edited during upload"
	a.saveDraft(false)
	if !a.editing || !a.features.sessionPassword.queuedSave {
		t.Fatal("upload blocked session editing")
	}
	close(remote.release)
	waitOperator(t, a, func() bool { return !f.busy })
	disk, err := a.store.Load()
	if err != nil || a.editing || len(disk.Profiles) != 1 || disk.Profiles[0].Name != "edited during upload" {
		t.Fatal("queued save after actual sync", f.status, a.formError, err)
	}
	if secret, err := a.vault.GetFor(disk.Profiles[0], p.Identity()); err != nil || secret != "fixture-secret" {
		t.Fatal("updated session lost saved credential", err)
	}
}

func TestManagerEditDuringSyncCommit(t *testing.T) {
	for _, action := range []string{"save", "cancel", "changed", "deleted"} {
		t.Run(action, func(t *testing.T) {
			store := &core.Store{Dir: t.TempDir()}
			cfg := core.DefaultConfig()
			p := core.NewProfile()
			p.Group, p.Host = "", "fixture.invalid"
			cfg.Profiles = []core.Profile{p}
			if err := store.Save(cfg); err != nil {
				t.Fatal(err)
			}
			owner, editor := newApp(store, cfg), newApp(store, cfg)
			hub := &applicationHub{store: store, config: cfg, vault: owner.vault, windows: []*App{owner, editor}, syncOwner: owner}
			owner.hub, editor.hub, editor.vault = hub, hub, owner.vault
			_, cancel := context.WithCancel(context.Background())
			defer cancel()
			f := &owner.features.sharedSync
			f.plan, f.busy, f.cancel = &core.SharedSyncPlan{}, true, cancel
			owner.setConfigurationBusy(true)
			manager := ui.NewTester(editor.managerView, 900, 580)
			if err := manager.Click("编辑 " + p.Name); err != nil || !editor.editing {
				t.Fatal("cannot open session properties during sync", err)
			}
			editor.draft.Name = "local edit"
			view := ui.NewTester(editor.connectionEditor, 900, 750)
			if err := view.Click("保存"); err != nil || !editor.features.sessionPassword.queuedSave {
				t.Fatal("save was not queued", err)
			}
			if action == "cancel" {
				if err := view.Click("取消"); err != nil {
					t.Fatal(err)
				}
			}
			next := cfg.Clone()
			next.Settings.FontSize = 19
			next.Profiles[0].ID = strings.ToUpper(p.ID)
			switch action {
			case "changed":
				next.Profiles[0].Host = "remote.invalid"
			case "deleted":
				next.Profiles = nil
			}
			if err := store.Save(next); err != nil {
				t.Fatal(err)
			}
			f.busy, f.cancel, f.plan = false, nil, nil
			owner.setConfigurationBusy(false)
			owner.applyCommittedConfig(next)
			owner.resumeSessionSaves(nil)
			disk, err := store.Load()
			if err != nil || disk.Settings.FontSize != 19 || editor.features.sessionPassword.queuedSave {
				t.Fatal("lost publication or left save queued", err)
			}
			switch action {
			case "save":
				if editor.editing || len(disk.Profiles) != 1 || disk.Profiles[0].Name != "local edit" || disk.Profiles[0].ID != strings.ToUpper(p.ID) || !editor.managerSelection.Has(disk.Profiles[0].ID) {
					t.Fatal("local edit was not applied to the published session", editor.formError)
				}
			case "cancel":
				if editor.editing || disk.Profiles[0].Name != p.Name {
					t.Fatal("cancelled save ran")
				}
			default:
				if !editor.editing || editor.formError == "" || editor.draft.Name != "local edit" || len(disk.Profiles) != len(next.Profiles) ||
					action == "changed" && disk.Profiles[0].Host != "remote.invalid" {
					t.Fatal("lost a conflicting change or draft")
				}
			}
		})
	}
}

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

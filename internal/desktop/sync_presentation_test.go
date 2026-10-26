package desktop

import (
	"bytes"
	"github.com/egoist/mygo/ui"
	"os"
	"oshell-desktop/internal/core"
	"path/filepath"
	"testing"
	"time"
)

func TestLiveDirectorySwitchKeepsWorkingConfiguration(t *testing.T) {
	s := &core.Store{Dir: t.TempDir()}
	cfg := core.DefaultConfig()
	if err := s.Save(cfg); err != nil {
		t.Fatal(err)
	}
	a := newApp(s, cfg)
	if err := a.vault.SetMaster("fixture-master"); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(s.Dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	first := t.TempDir()
	// Exercise a path spelling that differs from its canonical directory,
	// as can also happen with a Windows runner's short-name temp root.
	for _, target := range []string{first + string(os.PathSeparator) + ".", t.TempDir()} {
		canonicalTarget, err := filepath.EvalSymlinks(target)
		if err != nil {
			t.Fatal(err)
		}
		a.prepareSettings()
		a.loadSharedSyncSettings()
		a.selectSyncMethod("directory")
		a.features.sharedSync.options.Address = target
		done := false
		a.applySyncDraft(func() { done = true })
		drainSharedSync(t, a)
		saved, err := core.LoadSyncOptions(s.Dir)
		if err != nil || !done || !saved.Enabled || saved.RequireExisting {
			t.Fatalf("live switch failed: load=%v done=%v enabled=%v requireExisting=%v settings=%q", err, done, saved.Enabled, saved.RequireExisting, a.features.settingsError)
		}
		if saved.Address != canonicalTarget {
			t.Fatalf("live switch target: got %q, want canonical %q (selected %q)", saved.Address, canonicalTarget, target)
		}
		pending, err := core.LoadPendingSyncDirectory(s.Dir)
		if err != nil || pending != nil {
			t.Fatal("live switch requires restart", err)
		}
		after, err := os.ReadFile(filepath.Join(s.Dir, "config.json"))
		if err != nil || !bytes.Equal(before, after) || a.store.Dir != s.Dir {
			t.Fatal("sync moved working configuration", err)
		}
	}
	a.selectSyncMethod("webdav")
	if a.features.sharedSync.options.Address != "" {
		t.Fatal("backend change retained old target")
	}
	a.selectSyncMethod("none")
	if a.features.sharedSync.options.Enabled {
		t.Fatal("none left backend enabled")
	}
}

func TestSyncStatusSharedAcrossWindowsAndLockedCard(t *testing.T) {
	s := &core.Store{Dir: t.TempDir()}
	a, b := newApp(s, core.DefaultConfig()), newApp(s, core.DefaultConfig())
	b.vault = a.vault
	h := &applicationHub{windows: []*App{a, b}, syncOwner: a}
	a.hub, b.hub = h, h
	f := &a.features.sharedSync
	f.loaded, f.savedEnabled, f.busy = true, true, true
	f.savedOptions = core.SyncOptions{Version: 1, Enabled: true, Backend: "directory"}
	tt := ui.NewTester(b.syncStatusCard, 800, 500)
	if !tt.HasText("正在同步") || !b.syncOperationBusy() {
		t.Fatal("window lost shared job", tt.Texts())
	}
	b.queueSavedSharedSync()
	if b.features.sharedSync.manualQueued {
		t.Fatal("queued duplicate during shared job")
	}
	f.busy = false
	now := time.Now()
	f.summary.LastSuccess = &now
	f.phase = core.SyncCompleted
	tt.Frame()
	if !tt.HasText("已与本机同步目录对齐") {
		t.Fatal("wrong directory success claim", tt.Texts())
	}
	b.loadSharedSyncSettings()
	b.features.sharedSync.options.Address = "unapplied-draft"
	tt.Frame()
	if !tt.HasText("下方设置尚未应用") {
		t.Fatal("draft confused with saved state")
	}
	if err := a.vault.SetMaster("fixture-master"); err != nil {
		t.Fatal(err)
	}
	a.vault.Lock()
	b.features.settingsTab = 3
	locked := ui.NewTester(b.settingsView, 800, 650)
	if !locked.HasText("等待解锁主密码") || !locked.HasText("解锁主密码…") || locked.HasText("WebDAV 地址") {
		t.Fatal("locked sync entry unavailable", locked.Texts())
	}
	snapshot(t, locked, "sync-locked-card.png")
}

func TestSyncMethodPickerActuallyChangesBackend(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	if err := a.vault.SetMaster("fixture-master"); err != nil {
		t.Fatal(err)
	}
	a.prepareSettings()
	tt := ui.NewTester(a.sharedSyncSettings, 800, 950)
	for _, method := range []struct {
		label, backend string
		enabled        bool
	}{
		{"同步目录", "directory", true}, {"WebDAV", "webdav", true}, {"不启用同步", "webdav", false},
	} {
		if err := tt.Click("同步方式"); err != nil {
			t.Fatal(err)
		}
		if err := tt.Click(method.label); err != nil {
			t.Fatal(err, tt.Texts())
		}
		tt.Frame()
		f := &a.features.sharedSync
		if f.options.Enabled != method.enabled || f.options.Backend != method.backend {
			t.Fatal("selection was lost across frame", method, f.options)
		}
	}
	snapshot(t, tt, "sync-single-method.png")
}

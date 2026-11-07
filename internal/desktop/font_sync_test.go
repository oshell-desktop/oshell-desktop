package desktop

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
	"oshell-desktop/internal/core"
)

func TestFontSyncSettingsImportAndDownloadedActivation(t *testing.T) {
	defer func() { _ = activateUserFonts(nil) }()
	store := &core.Store{Dir: t.TempDir()}
	cfg := core.DefaultConfig()
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	a := newApp(store, cfg)
	if err := a.vault.SetMaster("font-sync-fixture"); err != nil {
		t.Fatal(err)
	}
	a.prepareSettings()
	a.loadSharedSyncSettings()
	policy := ui.NewTester(a.syncPolicySettings, 800, 800)
	label := "字体文件（明文附件，默认关闭）"
	if !policy.HasText(label) || a.features.sharedSync.options.Policy.Content.FontFiles {
		t.Fatal("missing default-off control")
	}
	if err := policy.Click(label); err != nil {
		t.Fatal(err)
	}
	a.closeSettings()
	a.prepareSettings()
	a.loadSharedSyncSettings()
	if a.features.sharedSync.options.Policy.Content.FontFiles {
		t.Fatal("cancel saved font policy")
	}
	remote := t.TempDir()
	a.features.sharedSync.options = core.SyncOptions{Version: 1, Enabled: true, Backend: "directory", Address: remote, Policy: core.DefaultSyncPolicy()}
	a.features.sharedSync.options.Policy.Content.FontFiles = true
	a.saveSharedSyncSettings()
	drainSharedSync(t, a)
	if a.features.sharedSync.status != "同步设置已保存" {
		t.Fatal(a.features.sharedSync.status)
	}
	options, err := a.loadSyncOptions()
	if err != nil || !options.Policy.Content.FontFiles {
		t.Fatal("policy not persisted", err)
	}
	source := filepath.Join(t.TempDir(), "mono.ttf")
	if err = os.WriteFile(source, gomono.TTF, 0600); err != nil {
		t.Fatal(err)
	}
	draft, err := core.NewFontLibrary(store.Dir).PrepareImport(context.Background(), []string{source})
	if err != nil {
		t.Fatal(err)
	}
	if err = a.commitUserFonts(draft); err != nil {
		t.Fatal(err)
	}
	if !a.features.sharedSync.pendingLocal || a.features.sharedSync.phase != core.SyncQueued {
		t.Fatal("import did not queue font sync")
	}
	a.startSavedSharedSync()
	drainSharedSync(t, a)
	if a.features.sharedSync.status != "同步完成" {
		t.Fatal(a.features.sharedSync.status)
	}
	remoteLibrary := core.NewFontLibrary(remote)
	loaded, err := remoteLibrary.Load()
	if err != nil || len(loaded.Fonts) != 1 {
		t.Fatal("upload failed", err)
	}
	if err = os.WriteFile(source, goregular.TTF, 0600); err != nil {
		t.Fatal(err)
	}
	remoteDraft, err := remoteLibrary.PrepareImport(context.Background(), []string{source})
	if err != nil {
		t.Fatal(err)
	}
	if err = remoteLibrary.Commit(remoteDraft); err != nil {
		t.Fatal(err)
	}
	generation := ui.FontGeneration()
	a.startSavedSharedSync()
	drainSharedSync(t, a)
	if a.features.sharedSync.status != "同步完成" {
		t.Fatal(a.features.sharedSync.status)
	}
	if len(userFontData) != 2 || len(a.features.fontLibrary.draft.Fonts) != 2 || ui.FontGeneration() == generation {
		t.Fatal("downloaded fonts not activated")
	}
	if a.features.sharedSync.pendingLocal {
		t.Fatal("download created another local sync")
	}
	a.prepareSettings()
	a.loadSharedSyncSettings()
	policy = ui.NewTester(a.syncPolicySettings, 800, 800)
	snapshot(t, policy, "font-sync-policy.png")
	if !a.features.sharedSync.options.Policy.Content.FontFiles {
		t.Fatal("reopened settings lost policy")
	}
}

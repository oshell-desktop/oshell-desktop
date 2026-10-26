package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSyncScopeChangeArchivesConflictAndBaseline(t *testing.T) {
	dir := t.TempDir()
	o := SyncOptions{Version: 1, Enabled: true, Backend: "directory", Address: t.TempDir(), Policy: DefaultSyncPolicy()}
	if err := SaveSyncOptions(dir, o); err != nil {
		t.Fatal(err)
	}
	paths := []string{(SharedReplicaStore{Directory: dir, Target: o.Target()}).path(), (SharedConflictStore{Directory: dir, Target: o.Target()}).path()}
	for _, p := range paths {
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("synthetic archive"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	next := o
	next.Policy.Content.QuickCommands = true
	if err := SaveSyncOptions(dir, next); err != nil {
		t.Fatal(err)
	}
	if err := SaveSyncOptions(dir, o); err != nil {
		t.Fatal(err)
	}
	for _, p := range paths {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatal("old scope state revived", p, err)
		}
		matches, _ := filepath.Glob(p + ".archived-*")
		if len(matches) != 1 {
			t.Fatal("missing recovery archive", matches)
		}
	}
}

func TestSyncPolicyValidationAndScopeIdentity(t *testing.T) {
	p := DefaultSyncPolicy()
	if p.Validate() != nil || (SyncPolicy{}).Effective() != p {
		t.Fatal("legacy policy default")
	}
	o := SyncOptions{Backend: "directory", Address: t.TempDir()}
	old := o.Target()
	o.Policy = p
	if o.Target() != old {
		t.Fatal("legacy baseline changed")
	}
	p.Content.QuickCommands = true
	o.Policy = p
	if o.Target() == old {
		t.Fatal("scope must require first contact")
	}
	for _, bad := range []SyncPolicy{{Version: 2}, {Version: 1}, func() SyncPolicy { x := p; x.CheckInterval = 1; return x }()} {
		if bad.Validate() == nil {
			t.Fatal("accepted invalid policy")
		}
	}
}

// Keep all 15 original masks independent so race CI can schedule them
// across runners; no password derivation or assertion is weakened.
func TestSyncSelectedCategoriesPreserveBothSides(t *testing.T) {
	testSyncSelectedCategoriesPreserveBothSides(t, 1)
}
func TestSyncSelectedCategoriesPreserveBothSides02(t *testing.T) {
	testSyncSelectedCategoriesPreserveBothSides(t, 2)
}
func TestSyncSelectedCategoriesPreserveBothSides03(t *testing.T) {
	testSyncSelectedCategoriesPreserveBothSides(t, 3)
}
func TestSyncSelectedCategoriesPreserveBothSides04(t *testing.T) {
	testSyncSelectedCategoriesPreserveBothSides(t, 4)
}
func TestSyncSelectedCategoriesPreserveBothSides05(t *testing.T) {
	testSyncSelectedCategoriesPreserveBothSides(t, 5)
}
func TestSyncSelectedCategoriesPreserveBothSides06(t *testing.T) {
	testSyncSelectedCategoriesPreserveBothSides(t, 6)
}
func TestSyncSelectedCategoriesPreserveBothSides07(t *testing.T) {
	testSyncSelectedCategoriesPreserveBothSides(t, 7)
}
func TestSyncSelectedCategoriesPreserveBothSides08(t *testing.T) {
	testSyncSelectedCategoriesPreserveBothSides(t, 8)
}
func TestSyncSelectedCategoriesPreserveBothSides09(t *testing.T) {
	testSyncSelectedCategoriesPreserveBothSides(t, 9)
}
func TestSyncSelectedCategoriesPreserveBothSides10(t *testing.T) {
	testSyncSelectedCategoriesPreserveBothSides(t, 10)
}
func TestSyncSelectedCategoriesPreserveBothSides11(t *testing.T) {
	testSyncSelectedCategoriesPreserveBothSides(t, 11)
}
func TestSyncSelectedCategoriesPreserveBothSides12(t *testing.T) {
	testSyncSelectedCategoriesPreserveBothSides(t, 12)
}
func TestSyncSelectedCategoriesPreserveBothSides13(t *testing.T) {
	testSyncSelectedCategoriesPreserveBothSides(t, 13)
}
func TestSyncSelectedCategoriesPreserveBothSides14(t *testing.T) {
	testSyncSelectedCategoriesPreserveBothSides(t, 14)
}
func TestSyncSelectedCategoriesPreserveBothSides15(t *testing.T) {
	testSyncSelectedCategoriesPreserveBothSides(t, 15)
}

func testSyncSelectedCategoriesPreserveBothSides(t *testing.T, mask int) {
	t.Helper()
	t.Run(fmt.Sprint(mask), func(t *testing.T) {
		selection := SyncContentSelection{mask&1 != 0, mask&2 != 0, mask&4 != 0, mask&8 != 0}
		e, cfg, master := syncEngineFixture(t, t.TempDir())
		e.Selection = &selection
		cfg.Settings.FontSize = 19
		cfg.Settings.ScrollbackMB = 9
		cfg.Settings.UpdateRepository = "example/local"
		cfg.SessionDefaults.SSHPort = 2219
		cfg.QuickCommands = []QuickCommand{{ID: NewID(), Name: "local", Text: "local"}}
		if err := e.Store.Save(cfg); err != nil {
			t.Fatal(err)
		}
		if err := e.Vault.Put(cfg.Profiles[0], "synthetic-password"); err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		remote, _, err := e.Vault.ExportSharedDocument(ctx, cfg, master, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		remote.Profiles[0].Definition.Name = "remote"
		remote.Preferences["fontSize"] = json.RawMessage(`23`)
		remote.Defaults.SSHPort = 2223
		remote.QuickCommands = []QuickCommand{{ID: NewID(), Name: "remote", Text: "remote"}}
		body, err := remote.Encode(master)
		if err != nil {
			t.Fatal(err)
		}
		if err = e.Transport.Put(ctx, body, ""); err != nil {
			t.Fatal(err)
		}
		remote, err = DecodeSharedDocument(body, master)
		if err != nil {
			t.Fatal(err)
		}
		plan, err := e.Prepare(ctx, cfg, master, false)
		if err != nil {
			t.Fatal(err)
		}
		if !plan.InitialChoiceRequired {
			t.Fatal("missing confirmation")
		}
		if err = plan.ChooseInitial(false); err != nil {
			t.Fatal(err)
		}
		next, err := plan.Commit(ctx, nil, master)
		if err != nil {
			t.Fatal(err)
		}
		if (next.Profiles[0].Name == "remote") != selection.Sessions {
			t.Fatal("sessions scope")
		}
		if (next.Settings.FontSize == 23) != selection.ClientSettings || next.Settings.ScrollbackMB != 9 || next.Settings.UpdateRepository != "example/local" {
			t.Fatal("client/local settings scope", next.Settings)
		}
		if (next.SessionDefaults.SSHPort == 2223) != selection.SessionDefaults {
			t.Fatal("defaults scope")
		}
		if (next.QuickCommands[0].Name == "remote") != selection.QuickCommands {
			t.Fatal("commands scope")
		}
		if secret, err := e.Vault.GetFor(next.Profiles[0], next.Profiles[0].Identity()); err != nil || secret != "synthetic-password" {
			t.Fatal("local password lost", err)
		}
		before, _ := e.Transport.Fetch(ctx)
		plan, err = e.Prepare(ctx, next, master, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = plan.Commit(ctx, nil, master); err != nil {
			t.Fatal(err)
		}
		after, _ := e.Transport.Fetch(ctx)
		if !bytes.Equal(before.Data, after.Data) || before.ETag != after.ETag {
			t.Fatal("unchanged sync rewrote data")
		}
		// A selected local edit cannot replace unselected compatibility payload.
		next.Profiles[0].Name = "edited"
		next.Settings.FontSize = 25
		next.SessionDefaults.SSHPort = 2225
		next.QuickCommands[0].Text = "edited"
		if err = e.Store.Save(next); err != nil {
			t.Fatal(err)
		}
		plan, err = e.Prepare(ctx, next, master, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = plan.Commit(ctx, nil, master); err != nil {
			t.Fatal(err)
		}
		after, _ = e.Transport.Fetch(ctx)
		published, err := DecodeSharedDocument(after.Data, master)
		if err != nil {
			t.Fatal(err)
		}
		if !selection.Sessions && !reflect.DeepEqual(published.Profiles, remote.Profiles) {
			t.Fatal("unselected remote sessions changed")
		}
		if !selection.ClientSettings && string(published.Preferences["fontSize"]) != "23" {
			t.Fatal("unselected remote settings changed")
		}
		if !selection.QuickCommands && !reflect.DeepEqual(published.QuickCommands, remote.QuickCommands) {
			t.Fatal("unselected remote commands changed")
		}
		if !selection.SessionDefaults && published.Defaults.SSHPort != 2223 {
			t.Fatal("unselected remote defaults changed")
		}
	})
}

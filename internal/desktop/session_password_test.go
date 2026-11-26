package desktop

import (
	"github.com/egoist/mygo/ui"
	"os"
	"oshell-desktop/internal/core"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestSessionEditorSavesPreservesAndRemovesPasswordAtomically(t *testing.T) {
	store := &core.Store{Dir: t.TempDir()}
	cfg := core.DefaultConfig()
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	a := newApp(store, cfg)
	p := core.NewProfile()
	p.Kind = "ftp"
	p.Port = 21
	p.Host = "fixture.invalid"
	p.Username = "tester"
	p.Name = "Credential fixture"
	a.edit(p)
	a.features.sessionPassword.password = "editor-secret-fixture"
	a.features.sessionPassword.remember = true
	tt := ui.NewTester(a.view, 1000, 750)
	snapshot(t, tt, "session-password-editor.png")
	a.saveDraft(false)
	waitOperator(t, a, func() bool { return !a.features.sessionPassword.busy })
	if a.editing {
		t.Fatal(a.formError)
	}
	saved, ok := a.profileByID(p.ID)
	if !ok {
		t.Fatal("profile missing")
	}
	secret, err := a.vault.GetFor(saved, saved.Identity())
	if err != nil || secret != "editor-secret-fixture" {
		t.Fatal("saved credential missing", err)
	}
	for _, name := range []string{"config.json", "credentials.json"} {
		data, err := os.ReadFile(filepath.Join(store.Dir, name))
		if err != nil || strings.Contains(string(data), "editor-secret-fixture") {
			t.Fatal("plaintext persisted", name, err)
		}
	}
	a.edit(saved)
	a.draft.Name = "Renamed"
	a.saveDraft(false)
	waitOperator(t, a, func() bool { return !a.features.sessionPassword.busy })
	if a.editing {
		t.Fatal(a.formError)
	}
	saved, _ = a.profileByID(p.ID)
	secret, err = a.vault.GetFor(saved, saved.Identity())
	if err != nil || secret != "editor-secret-fixture" {
		t.Fatal("blank rename lost credential", err)
	}
	a.edit(saved)
	a.draft.Host = "different.invalid"
	a.saveDraft(false)
	waitOperator(t, a, func() bool { return !a.features.sessionPassword.busy })
	if a.editing {
		t.Fatal(a.formError)
	}
	saved, _ = a.profileByID(p.ID)
	secret, err = a.vault.GetFor(saved, saved.Identity())
	if err != nil || secret != "editor-secret-fixture" {
		t.Fatal("changed target lost saved password", err)
	}
	a.edit(saved)
	a.features.sessionPassword.remember = false
	a.saveDraft(false)
	waitOperator(t, a, func() bool { return !a.features.sessionPassword.busy })
	if a.editing || a.vault.HasStoredCredential(p.ID) {
		t.Fatal("credential not removed atomically", a.formError)
	}
	saved, _ = a.profileByID(p.ID)
	if saved.Host != "different.invalid" {
		t.Fatal("configuration not committed with removal")
	}
}
func TestManagerRenameSavedSSHWithoutResolver(t *testing.T) {
	a, manager := managerClipboardFixture(t)
	p := a.config.Profiles[0]
	before := a.vault.CredentialVersion(p)
	t.Setenv("PATH", t.TempDir())
	if err := manager.Click("编辑"); err != nil || !a.editing {
		t.Fatal("open properties", err)
	}
	a.draft.Name = "offline rename"
	view := ui.NewTester(a.connectionEditor, 1000, 750)
	if err := view.Click("保存"); err != nil {
		t.Fatal(err)
	}
	waitOperator(t, a, func() bool { return !a.features.sessionPassword.busy })
	disk, err := a.store.Load()
	if err != nil || a.editing || disk.Profiles[0].Name != "offline rename" || a.vault.CredentialVersion(p) != before {
		t.Fatal("offline property edit changed credential or failed", a.formError, err)
	}
}

func TestManagerEditIncompleteLegacyRouteAsSSH(t *testing.T) {
	store := &core.Store{Dir: t.TempDir()}
	cfg := core.DefaultConfig()
	p := core.NewProfile()
	p.Host, p.Username = "fixture.invalid", "operator#UTF-8#asset#SSH#"
	cfg.Profiles = []core.Profile{p}
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	a := newApp(store, cfg)
	a.editManagerProfile(p)
	a.draft.Name = "renamed"
	a.saveDraft(false)
	if a.editing || a.config.Profiles[0].Name != "renamed" || a.config.Profiles[0].Username != p.Username {
		t.Fatal("ordinary SSH username forced through bastion validation", a.formError)
	}
}

func TestSessionEditorCancelDropsPasswordDraft(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	p := core.NewProfile()
	p.Host = "fixture.invalid"
	a.edit(p)
	a.features.sessionPassword.password = "cancelled-secret"
	a.features.sessionPassword.remember = true
	tt := ui.NewTester(a.view, 1000, 750)
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.editing || a.features.sessionPassword.password != "" || a.vault.HasStoredCredential(p.ID) {
		t.Fatal("cancel retained or saved password")
	}
}

func TestSessionEditorTargetPasswordLifecycle(t *testing.T) {
	t.Setenv("OSHELL_TEST_IGNORE_SSH_CONFIG", "1")
	store := &core.Store{Dir: t.TempDir()}
	cfg := core.DefaultConfig()
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	a := newApp(store, cfg)
	p := core.NewProfile()
	p.Host, p.Username = "gateway.invalid", "operator#UTF-8#fixture-asset#SSH#worker"
	a.config.Profiles = append(a.config.Profiles, p)
	a.edit(p)
	f := &a.features.sessionPassword
	view := ui.NewTester(a.view, 1000, 750)
	if !view.HasText("服务器登录密码") || !view.HasText("堡垒机登录密码") {
		t.Fatal("routed session fields missing")
	}
	save := func() {
		t.Helper()
		a.saveDraft(false)
		waitOperator(t, a, func() bool { return !f.busy })
		if a.editing {
			t.Fatal(a.formError)
		}
	}
	target, _ := p.BastionTarget()
	check := func(want string) {
		t.Helper()
		got, err := a.vault.GetFor(target.Credential, p.Identity())
		if err != nil || got != want {
			t.Fatal("unexpected target credential", err)
		}
	}
	f.targetPassword = "first-fixture"
	save()
	check("first-fixture")
	if a.vault.HasStoredCredential(p.ID) || len(a.tabs) != 0 {
		t.Fatal("saving target required a connection or wrote the gateway credential")
	}
	a.edit(p)
	if f.targetPassword != "" || f.targetVersion == "" {
		t.Fatal("saved state is missing or revealed")
	}
	a.draft.Name = "renamed"
	save()
	check("first-fixture")
	a.edit(p)
	f.targetPassword = "replacement-fixture"
	save()
	check("replacement-fixture")
	a.edit(p)
	f.targetPassword = "cancelled-fixture"
	a.cancelSessionPassword()
	if f.targetPassword != "" {
		t.Fatal("cancel retained draft")
	}
	check("replacement-fixture")
	a.edit(p)
	f.forgetTarget = true
	if err := a.vault.PutFor(target.Credential, "other-window-fixture", p.Identity()); err != nil {
		t.Fatal(err)
	}
	a.saveDraft(false)
	if !a.editing || f.busy || a.formError == "" {
		t.Fatal("stale editor overwrote a later change")
	}
	check("other-window-fixture")
	a.edit(p)
	f.forgetTarget = true
	save()
	check("")
	a.edit(p)
	f.targetPassword = "old-asset-fixture"
	a.draft.Username = "anotheruser"
	view.Frame()
	if f.targetPassword != "" {
		t.Fatal("unrelated account inherited target editor")
	}
	a.saveDraft(false)
	if !a.editing || a.formError == "" {
		t.Fatal("accepted malformed bastion login")
	}
}

func TestBastionEditorComposesLoginInEitherOrder(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	const prefix = "operator#UTF-8#fixture-asset#SSH#"
	p := a.newProfile(bastionSSH)
	a.editManagerProfile(p)
	f := &a.features.sessionPassword
	f.serverAccount = "worker"
	a.draft.Username = prefix
	check := func(account string) {
		t.Helper()
		got, err := a.editorLogin()
		if err != nil || got != prefix+account || a.draft.Username != got || f.serverAccount != account {
			t.Fatal("incorrect composed account", err)
		}
	}
	check("worker")
	a.draft.Username = prefix + "admin"
	check("admin")
	f.serverAccount = "other"
	check("other")
	f.serverAccount = ""
	if _, err := a.editorLogin(); err == nil || a.draft.Username != prefix {
		t.Fatal("cleared account was reused")
	}
	a.draft.Username = prefix + "invalid#extra"
	if _, err := a.editorLogin(); err == nil {
		t.Fatal("invalid route accepted")
	}
}

func TestOrdinarySSHPropertiesKeepProtocolAndIgnoreHiddenPassword(t *testing.T) {
	store := &core.Store{Dir: t.TempDir()}
	cfg := core.DefaultConfig()
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	a := newApp(store, cfg)
	p := a.newProfile("ssh")
	p.Host, p.Username = "gateway.invalid", "operator#UTF-8#fixture#SSH#worker"
	a.editManagerProfile(p)
	view := ui.NewTester(a.connectionEditor, 1000, 750)
	checkPlain := func() {
		t.Helper()
		view.Frame()
		if a.features.editorChoice != "ssh" || !view.HasText("用户名") || !view.HasText("保存密码") ||
			view.HasText("快速登录字符串") || view.HasText("服务器登录密码") || view.HasText("堡垒机登录密码") {
			t.Fatal("ordinary SSH did not retain its original fields")
		}
	}
	checkPlain()
	a.features.editorChoice = bastionSSH
	view.Frame()
	if !view.HasText("服务器登录密码") {
		t.Fatal("missing bastion fields")
	}
	a.features.sessionPassword.targetPassword = "discarded-fixture"
	a.features.editorChoice = "ssh"
	checkPlain()
	a.saveDraft(false)
	if a.editing {
		t.Fatal(a.formError)
	}
	target, _ := p.BastionTarget()
	if a.vault.HasStoredCredential(target.Credential.ID) {
		t.Fatal("saved hidden target password")
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	a = newApp(store, loaded)
	p, ok := a.profileByID(p.ID)
	if !ok || a.sessionType(p) != "ssh" {
		t.Fatal("explicit SSH choice lost after reload")
	}
	a.edit(p)
	view = ui.NewTester(a.connectionEditor, 1000, 750)
	checkPlain()
	a.features.editorChoice = bastionSSH
	a.saveDraft(false)
	if a.editing || a.sessionType(p) != bastionSSH {
		t.Fatal("explicit bastion selection not restored", a.formError)
	}
}

func TestSessionEditorLateCancelNeverStartsConnection(t *testing.T) {
	store := &core.Store{Dir: t.TempDir()}
	cfg := core.DefaultConfig()
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	a := newApp(store, cfg)
	p := core.NewProfile()
	p.Kind = "ftp"
	p.Port = 21
	p.Host = "fixture.invalid"
	p.Username = "tester"
	p.Name = "Late cancel fixture"
	a.edit(p)
	a.features.sessionPassword.password = "fixture-password"
	a.features.sessionPassword.remember = true
	a.saveDraft(true)
	// Do not drain the UI queue until the background transaction is committed.
	// This reproduces cancellation after storage succeeded but before its UI
	// completion can execute the requested follow-up connection.
	deadline := time.Now().Add(3 * time.Second)
	committed := false
	for time.Now().Before(deadline) {
		saved, err := store.Load()
		if err == nil {
			for _, profile := range saved.Profiles {
				if profile.ID == p.ID {
					committed = true
				}
			}
		}
		if committed {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !committed {
		t.Fatal("transaction did not finish")
	}
	a.cancelSessionPassword()
	waitOperator(t, a, func() bool { return !a.features.sessionPassword.busy })
	if a.authOpen || len(a.tabs) != 0 || len(a.authQueue) != 0 {
		t.Fatal("late cancellation initiated authentication/connection")
	}
	if a.status != "配置已保存，已取消连接" {
		t.Fatal("committed cancellation misreported", a.status)
	}
	if a.features.sessionPassword.password != "" {
		t.Fatal("password draft retained")
	}
	if !a.vault.HasStoredCredential(p.ID) {
		t.Fatal("already committed credential disappeared")
	}
}

func TestSessionEditorCancellationStopsIdentityResolutionBeforeCommit(t *testing.T) {
	binaryDir := t.TempDir()
	started := filepath.Join(binaryDir, "started")
	if runtime.GOOS == "windows" {
		exe, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(exe)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(binaryDir, "ssh.exe"), data, 0700); err != nil {
			t.Fatal(err)
		}
	} else {
		script := "#!/bin/sh\n: > \"$OSHELL_SSH_EDIT_STARTED\"\nexec /bin/sleep 30\n"
		if err := os.WriteFile(filepath.Join(binaryDir, "ssh"), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", binaryDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("OSHELL_SSH_EDIT_STARTED", started)
	store := &core.Store{Dir: t.TempDir()}
	cfg := core.DefaultConfig()
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	a := newApp(store, cfg)
	p := core.NewProfile()
	p.Host = "fixture.invalid"
	p.Username = "tester"
	a.edit(p)
	a.features.sessionPassword.password = "fixture-only"
	a.features.sessionPassword.remember = true
	a.saveDraft(true)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(started); err == nil {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := os.Stat(started); err != nil {
		a.cancelSessionPassword()
		t.Fatal("resolver fixture did not start")
	}
	tt := ui.NewTester(a.view, 1000, 750)
	tt.Key(0, ui.KeyEscape)
	waitOperator(t, a, func() bool { return !a.features.sessionPassword.busy })
	saved, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Profiles) != 0 || a.vault.HasStoredCredential(p.ID) || a.authOpen || len(a.tabs) != 0 {
		t.Fatal("cancelled resolution saved data or started authentication")
	}
	if a.editing || a.configurationBusy() || a.status != "已取消保存" || a.features.sessionPassword.password != "" {
		t.Fatal("cancel state not released", a.status)
	}
}

func TestSessionEditorChangesProtectionWithoutReplacingPassword(t *testing.T) {
	store := &core.Store{Dir: t.TempDir()}
	cfg := core.DefaultConfig()
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	a := newApp(store, cfg)
	if err := a.vault.SetMaster("editor-master-password"); err != nil {
		t.Fatal(err)
	}
	p := core.NewProfile()
	p.Kind = "ftp"
	p.Port = 21
	p.Host = "fixture.invalid"
	p.Username = "tester"
	a.edit(p)
	a.features.sessionPassword.remember = true
	a.features.sessionPassword.password = "entry-password"
	a.features.sessionPassword.protection = "local"
	a.saveDraft(false)
	waitOperator(t, a, func() bool { return !a.features.sessionPassword.busy })
	if a.editing || a.vault.CredentialProtection(p.ID) != "local" {
		t.Fatal("local save failed", a.formError)
	}
	saved, _ := a.profileByID(p.ID)
	a.edit(saved)
	a.features.sessionPassword.protection = "master"
	a.saveDraft(false)
	waitOperator(t, a, func() bool { return !a.features.sessionPassword.busy })
	if a.editing || a.vault.CredentialProtection(p.ID) != "master" {
		t.Fatal("master conversion failed", a.formError)
	}
	secret, err := a.vault.GetFor(saved, saved.Identity())
	if err != nil || secret != "entry-password" {
		t.Fatal("conversion lost secret", err)
	}
	a.edit(saved)
	a.features.sessionPassword.protection = "local"
	a.saveDraft(false)
	waitOperator(t, a, func() bool { return !a.features.sessionPassword.busy })
	if a.editing {
		t.Fatal(a.formError)
	}
	a.vault.Lock()
	secret, err = a.vault.GetFor(saved, saved.Identity())
	if err != nil || secret != "entry-password" {
		t.Fatal("reverse conversion lost local access", err)
	}
}

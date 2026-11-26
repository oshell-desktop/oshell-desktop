package core

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestBastionTargetPasswordProtectionAndScope(t *testing.T) {
	dir := t.TempDir()
	v, err := OpenVault(dir)
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	p := NewProfile()
	p.Host, p.Username = "gateway.invalid", "tester#UTF-8#asset-id#SSH#demo"
	cfg.Profiles = []Profile{p}
	b, ok := p.BastionTarget()
	if !ok || b.Asset != "asset-id" || b.Account != "demo" {
		t.Fatal("target identity unresolved")
	}
	const secret = "fixture-terminal-only-secret"
	identity := p.Identity()
	identity.Host = "resolved.invalid"
	if err = v.SetMaster("fixture-master"); err != nil {
		t.Fatal(err)
	}
	if err = v.PutForRevision(b.Credential, secret, identity, v.Revision()); err != nil {
		t.Fatal(err)
	}
	if raw, err := os.ReadFile(filepath.Join(dir, "credentials.json")); err != nil || bytes.Contains(raw, []byte(secret)) {
		t.Fatal("unexpected vault contents")
	}
	v, err = OpenVault(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = v.GetFor(b.Credential, identity); !errors.Is(err, ErrVaultLocked) {
		t.Fatal("master lock bypassed")
	}
	if err = v.Unlock("fixture-master"); err != nil {
		t.Fatal(err)
	}
	if got, err := v.GetFor(b.Credential, identity); err != nil || got != secret {
		t.Fatal("persistent target credential missing")
	}
	if got, err := v.GetFor(b.Credential, p.Identity()); err != nil || got != "" {
		t.Fatal("different resolved gateway accepted")
	}
	if got, err := v.GetFor(p, identity); err != nil || got != "" {
		t.Fatal("target password entered gateway credential slot")
	}
	doc, _, err := v.ExportSharedDocument(context.Background(), cfg, []byte("fixture-export"), nil, nil)
	if err != nil || len(doc.Profiles) != 1 || doc.Profiles[0].Password != nil || doc.Profiles[0].ProxyPassword != nil {
		t.Fatal("unexpected shared credential", err)
	}
	other := p
	other.Username = "tester#UTF-8#other-id#SSH#demo"
	next, _ := other.BastionTarget()
	if got, err := v.GetFor(next.Credential, identity); err != nil || got != "" {
		t.Fatal("another asset reused the saved password")
	}
	store := &Store{Dir: dir}
	cfg.Profiles = []Profile{other}
	if err = store.CommitCredentials(context.Background(), cfg, v, nil, ""); err != nil {
		t.Fatal(err)
	}
	if v.HasStoredCredential(b.Credential.ID) {
		t.Fatal("old target survived identity edit")
	}
}

func TestBastionTargetPasswordPrompts(t *testing.T) {
	b := BastionTarget{Account: "demo"}
	for _, s := range []string{"Password:", "请输入服务器密码：", "demo@fixture.invalid's password:", "(demo@fixture.invalid) Password:"} {
		if !b.PasswordPrompt(s) {
			t.Errorf("missing prompt %q", s)
		}
	}
	for _, s := range []string{"Verification code:", "请输入动态密码：", "New password:", "other@fixture.invalid's password:", "shell$ ", "Password"} {
		if b.PasswordPrompt(s) {
			t.Errorf("unexpected password prompt %q", s)
		}
	}
}

func TestBastionTargetPasswordAudit(t *testing.T) {
	dir := t.TempDir()
	if _, err := OpenMCPClients(dir); err != nil {
		t.Fatal(err)
	}
	audit, err := NewAgentAudit(dir, AgentAuditIndex{Session: NewID(), Client: NewID(), Title: "fixture"}, nil, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	s := &AgentSession{audit: audit, changed: make(chan struct{})}
	password := []byte("fixture-target-secret")
	writes := 0
	write := func() error { writes++; return nil }
	s.owner.Store(true)
	if s.GatePasswordInput(password, write) == nil {
		t.Fatal("automated owner accepted manual fill")
	}
	s.owner.Store(false)
	if err = s.GatePasswordInput(password, write); err != nil || writes != 1 {
		t.Fatal("manual fill not delivered", err)
	}
	s.Observe(password[:5])
	s.Observe(password[5:])
	if err = audit.Close(); err != nil {
		t.Fatal(err)
	}
	var output []byte
	err = ReadAgentAudit(context.Background(), dir, audit.ID(), func(e AgentAuditEvent) error {
		if bytes.Contains(e.Bytes, password) {
			t.Fatal("saved password present in audit")
		}
		if e.Kind == "output" {
			output = append(output, e.Bytes...)
		}
		return nil
	})
	if err != nil || !bytes.Contains(output, []byte("[敏感输入]")) {
		t.Fatal("missing echo redaction", err)
	}
}

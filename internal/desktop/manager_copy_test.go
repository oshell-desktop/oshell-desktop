package desktop

import (
	"oshell-desktop/internal/core"
	"testing"
)

func TestManagerCopyCommitsCredentialModeAndSelectsDuplicate(t *testing.T) {
	store := &core.Store{Dir: t.TempDir()}
	cfg := core.DefaultConfig()
	p := core.NewProfile()
	p.Group = "team"
	p.Name = "source"
	p.Host = "fixture.invalid"
	p.Username = "operator#UTF-8#fixture#SSH#worker"
	cfg.Profiles = []core.Profile{p}
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	a := newApp(store, cfg)
	if err := a.vault.SetMaster("fixture-master-password"); err != nil {
		t.Fatal(err)
	}
	if err := a.vault.PutForRevision(p, "saved-secret", p.Identity(), a.vault.Revision(), "local"); err != nil {
		t.Fatal(err)
	}
	target, _ := p.BastionTarget()
	if err := a.vault.PutFor(target.Credential, "target-fixture", p.Identity()); err != nil {
		t.Fatal(err)
	}
	a.managerAll = true
	a.query = "source"
	a.managerSelection.Add(p.ID)
	a.duplicateManagerSelection()
	waitOperator(t, a, func() bool { return !a.configurationBusy() })
	if len(a.config.Profiles) != 2 {
		t.Fatal(a.status)
	}
	duplicate := a.config.Profiles[1]
	if a.managerGroup != "team" || a.managerAll {
		t.Fatal("search copy was not revealed in its directory")
	}
	if duplicate.ID == p.ID || duplicate.Name != "source - 副本" || a.selected != duplicate.ID || !a.managerSelection.Has(duplicate.ID) {
		t.Fatal("copy identity/selection wrong")
	}
	if secret, err := a.vault.GetFor(duplicate, p.Identity()); err != nil || secret != "saved-secret" || a.vault.CredentialProtection(duplicate.ID) != "local" {
		t.Fatal("credential mode or binding lost", err)
	}
	disk, err := store.Load()
	copyTarget, _ := duplicate.BastionTarget()
	if secret, err := a.vault.GetFor(copyTarget.Credential, p.Identity()); err != nil || secret != "target-fixture" {
		t.Fatal("target credential not rebound to copied profile", err)
	}
	if err != nil || len(disk.Profiles) != 2 {
		t.Fatal("copy not persisted", err)
	}
}

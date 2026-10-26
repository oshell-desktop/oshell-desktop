package desktop

import (
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"testing"
)

func TestP1ExternalAuthenticationCannotPersistEvenAfterRetry(t *testing.T) {
	for _, tc := range []struct{ external, retry bool }{{true, false}, {true, true}, {false, false}} {
		a, p, _ := savedConnectionFixture(t)
		before := a.vault.Revision()
		cred := core.Credentials{ExternalLaunch: tc.external, SavePassword: true}
		if tc.retry {
			cred.Password = "incorrect-once"
		}
		a.open(p, cred, 0)
		waitOperator(t, a, func() bool { return len(a.challenges) == 1 })
		q := a.challenges[0]
		if q.allowSave == tc.external {
			t.Fatal("source save policy lost")
		}
		q.answers[0] = "saved-fixture-password"
		// Even a stale UI selection cannot bypass the origin policy.
		q.remember = true
		a.finishChallenge(q, false)
		waitOperator(t, a, func() bool { return len(a.tabs) > 0 && a.tabs[0].active.term != nil })
		if a.vault.HasStoredCredential(p.ID) == tc.external {
			t.Fatal("wrong persisted credential result")
		}
		if tc.external && a.vault.Revision() != before {
			t.Fatal("one-shot authentication changed vault")
		}
	}
}

func TestP1AuthenticatedSaveSkipsClearedMasterCache(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	p := core.NewProfile()
	p.Host = "fixture.invalid"
	a.config.Profiles = []core.Profile{p}
	if err := a.vault.SetMaster("fixture-master"); err != nil {
		t.Fatal(err)
	}
	revision := a.vault.Revision()
	a.vault.Lock()
	if canSaveSessionPassword(p, core.Credentials{}, a.vault) {
		t.Fatal("locked vault offered save")
	}
	if a.saveAuthenticatedSessionPassword(p, "fixture-secret", p.Identity(), a.vault, revision, true) != nil {
		t.Fatal("saved with cleared cache")
	}
	if a.vault.Revision() != revision || a.authOpen || a.features.vaultOpen {
		t.Fatal("save changed vault or nested unlock prompt")
	}
}

func TestP1ManagerClipboardRejectsRestoredTokenWithoutIntermediateRead(t *testing.T) {
	a, tt := managerClipboardFixture(t)
	tt.Key(ui.Ctrl, ui.KeyC)
	tt.Frame()
	token := tt.Clipboard()
	if a.managerClipboard == nil {
		t.Fatal("copy failed")
	}
	tt.SetClipboard("replacement")
	tt.SetClipboard(token)
	tt.Key(ui.Ctrl, ui.KeyV)
	tt.Frame()
	if a.managerClipboard != nil || a.configurationBusy() || len(a.config.Profiles) != 1 {
		t.Fatal("restored opaque token resurrected snapshot")
	}
	// A fresh copy after invalidation remains usable.
	tt.Key(ui.Ctrl, ui.KeyC)
	tt.Frame()
	tt.Key(ui.Ctrl, ui.KeyV)
	tt.Frame()
	waitOperator(t, a, func() bool { return !a.configurationBusy() })
	if len(a.config.Profiles) != 2 {
		t.Fatal("fresh copy no longer usable", a.status)
	}
}

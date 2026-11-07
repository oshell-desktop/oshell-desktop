package desktop

import (
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"testing"
)

func TestClearMasterRequiresCurrentPasswordEvenWhenUnlocked(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	if err := a.vault.SetMaster("fixture-master-password"); err != nil {
		t.Fatal(err)
	}
	a.features.vaultAction = "remove"
	a.features.vaultOpen = true
	tt := ui.NewTester(a.vaultDialog, 700, 500)
	tt.Type("wrong-password")
	tt.Key(0, ui.KeyEnter)
	if !a.features.vaultBusy {
		t.Fatal("Enter did not submit removal verification")
	}
	waitOperator(t, a, func() bool { return !a.features.vaultBusy })
	tt.Frame()
	if master, _ := a.vault.State(); !master || a.features.vaultError == "" {
		t.Fatal("cleared without verification")
	}
	tt.Type("fixture-master-password")
	tt.Key(0, ui.KeyEnter)
	waitOperator(t, a, func() bool { return !a.features.vaultBusy })
	if master, _ := a.vault.State(); master || a.features.vaultError != "" {
		t.Fatal("verified clear failed", a.features.vaultError)
	}
	if a.features.masterCurrent != "" {
		t.Fatal("dialog retained current password")
	}
}

func TestSetMasterPasswordFormEnter(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	a.features.vaultAction = "set"
	a.features.vaultOpen = true
	tt := ui.NewTester(a.vaultDialog, 700, 500)
	tt.Type("fixture-new-master")
	tt.Key(0, ui.KeyEnter)
	if a.features.vaultBusy || a.features.vaultError != "两次主密码不一致" {
		t.Fatal("Enter bypassed confirmation validation")
	}
	// Tab from the new password to its confirmation, then submit that field.
	tt.Key(0, ui.KeyTab)
	tt.Type("fixture-new-master")
	tt.Key(0, ui.KeyEnter)
	if !a.features.vaultBusy {
		t.Fatal("confirmation Enter did not submit the new master password")
	}
	tt.Key(0, ui.KeyEnter)
	waitOperator(t, a, func() bool { return !a.features.vaultBusy })
	if master, locked := a.vault.State(); !master || locked || a.features.vaultError != "" {
		t.Fatal("Enter did not enable protection", a.features.vaultError)
	}
	if a.features.master != "" || a.features.masterAgain != "" {
		t.Fatal("successful submission retained password drafts")
	}
}

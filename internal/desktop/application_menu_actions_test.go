package desktop

import (
	"oshell-desktop/internal/core"
	"testing"
)

func TestPreferenceMenuReusesOwnerWithoutDiscardingDraft(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	owner := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	owner.prepareSettings()
	owner.features.settingsDraft.FontSize = 19
	a.hub = &applicationHub{settingsOwner: owner}
	a.showPreferenceSection(5)
	if owner.features.settingsTab != 5 || owner.features.settingsDraft.FontSize != 19 || a.settingsOpen {
		t.Fatal("menu replaced owner or draft")
	}
	a.showPreferenceSection(4)
	if owner.features.settingsTab != 4 {
		t.Fatal("update settings menu selected wrong section")
	}
}
func TestPasswordMenuRoutesThroughUnlockAndExistingActions(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	a.passwordMenuAction("remove")
	if a.features.vaultOpen {
		t.Fatal("offered removal without master")
	}
	a.passwordMenuAction("set")
	if a.features.vaultAction != "set" || !a.features.vaultOpen {
		t.Fatal("set menu not connected")
	}
	if err := a.vault.SetMaster("fixture-master"); err != nil {
		t.Fatal(err)
	}
	a.features.vaultOpen = false
	a.passwordMenuAction("lock")
	if !a.passwordLocked() || a.workspaceLocked() {
		t.Fatal("lock menu did not lock")
	}
	a.passwordMenuAction("remove")
	if a.features.vaultAction != "unlock" || a.features.vaultMenuAfterUnlock != "remove" {
		t.Fatal("remove bypassed unlock")
	}
	if err := a.vault.Unlock("fixture-master"); err != nil {
		t.Fatal(err)
	}
	a.afterVaultUnlock()
	if a.features.vaultAction != "remove" || !a.features.vaultOpen || a.features.vaultMenuAfterUnlock != "" {
		t.Fatal("unlock did not resume requested action")
	}
	master, _ := a.vault.State()
	if !master {
		t.Fatal("menu removed master without confirmation")
	}
}

package clientalias

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows/registry"
)

// All paths, including the per-user Environment key, are relative to a
// disposable registry subtree. No installed client or real PATH is touched.
func fixture(t *testing.T) manager {
	t.Helper()
	path := fmt.Sprintf(`Software\OShellAliasTest-%d-%s`, os.Getpid(), t.Name())
	root, existed, err := registry.CreateKey(registry.CURRENT_USER, path, registry.ALL_ACCESS)
	if err != nil || existed {
		t.Fatal("create isolated hive", err)
	}
	t.Cleanup(func() {
		var clear func(registry.Key)
		clear = func(k registry.Key) {
			names, err := k.ReadSubKeyNames(-1)
			if err != nil {
				t.Error(err)
				return
			}
			for _, name := range names {
				child, err := registry.OpenKey(k, name, registry.ALL_ACCESS)
				if err != nil {
					t.Error(err)
					continue
				}
				clear(child)
				child.Close()
				if err := registry.DeleteKey(k, name); err != nil {
					t.Error(err)
				}
			}
		}
		clear(root)
		root.Close()
		if err := registry.DeleteKey(registry.CURRENT_USER, path); err != nil {
			t.Error(err)
		}
	})
	return manager{root, filepath.Join(t.TempDir(), "便携 程序.exe"), t.TempDir(), []byte("fixture launcher")}
}

func regValue(t *testing.T, root registry.Key, path, name string, set *regString) regString {
	t.Helper()
	k, _, err := registry.CreateKey(root, path, registry.READ|registry.WRITE)
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	if set != nil {
		if err := set.write(k, name); err != nil {
			t.Fatal(err)
		}
	}
	value, err := readString(k, name)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func toggle(t *testing.T, m manager, want [2]bool) State {
	t.Helper()
	state, err := load(m.root)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.apply(state, want); err != nil {
		t.Fatal(err)
	}
	state, err = load(m.root)
	if err != nil {
		t.Fatal(err)
	}
	for i := range want {
		if state.Enabled(i) != want[i] {
			t.Fatal("switch state", i)
		}
	}
	return state
}

func TestRegisterAndRestoreClientNames(t *testing.T) {
	m := fixture(t)
	putty := appPaths + `\putty.exe`
	original := regString{`%ProgramFiles%\PuTTY\putty.exe`, registry.EXPAND_SZ}
	regValue(t, m.root, putty, "", &original)
	marker := regString{"vendor setting", registry.SZ}
	regValue(t, m.root, putty, "Path", &marker)
	state := toggle(t, m, [2]bool{true, true})
	for i, name := range Names {
		if got := regValue(t, m.root, appPaths+`\`+name, "", nil); got.Text != state[i].Launcher {
			t.Fatal("App Paths", name)
		}
		if got, err := targetFor(m.root, state[i].Launcher); err != nil || got != m.target {
			t.Fatal("launcher dispatch", name, err)
		}
	}
	if _, err := targetFor(m.root, filepath.Join(t.TempDir(), Names[0])); err == nil {
		t.Fatal("unregistered launcher accepted")
	}
	toggle(t, m, [2]bool{false, true})
	if got := regValue(t, m.root, putty, "", nil); got != original {
		t.Fatal("original target/type lost", got)
	}
	if got := regValue(t, m.root, putty, "Path", nil); got != marker {
		t.Fatal("vendor settings changed")
	}
	if got := regValue(t, m.root, "Environment", "Path", nil); got.Text != filepath.Dir(state[1].Launcher) {
		t.Fatal("independent PATH", got)
	}
	toggle(t, m, [2]bool{})
	if got := regValue(t, m.root, "Environment", "Path", nil); got.Kind != 0 {
		t.Fatal("previously absent PATH not restored", got)
	}
	if k, err := registry.OpenKey(m.root, appPaths+`\xshell.exe`, registry.READ); !errors.Is(err, registry.ErrNotExist) {
		k.Close()
		t.Fatal("created key retained", err)
	}
	for i, name := range Names {
		if _, err := os.Stat(state[i].Launcher); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("owned file retained", name)
		}
		if _, err := targetFor(m.root, state[i].Launcher); err == nil {
			t.Fatal("disabled launcher accepted", name)
		}
	}
}

func TestClientChangesSurviveRestore(t *testing.T) {
	m := fixture(t)
	prior := regString{`C:\existing;;%USERPROFILE%\bin;`, registry.EXPAND_SZ}
	regValue(t, m.root, "Environment", "Path", &prior)
	state := toggle(t, m, [2]bool{true, false})
	updated := regValue(t, m.root, "Environment", "Path", nil)
	updated.Text += "extra"
	regValue(t, m.root, "Environment", "Path", &updated)
	other := regString{`D:\new\putty.exe`, registry.SZ}
	regValue(t, m.root, appPaths+`\putty.exe`, "", &other)
	if err := os.WriteFile(state[0].Launcher, []byte("another file"), 0600); err != nil {
		t.Fatal(err)
	}
	toggle(t, m, [2]bool{})
	if got := regValue(t, m.root, "Environment", "Path", nil); got.Text != prior.Text+"extra" || got.Kind != prior.Kind {
		t.Fatal("PATH edit lost", got)
	}
	if got := regValue(t, m.root, appPaths+`\putty.exe`, "", nil); got != other {
		t.Fatal("external registration lost")
	}
	if b, err := os.ReadFile(state[0].Launcher); err != nil || string(b) != "another file" {
		t.Fatal("foreign file lost")
	}
}

func TestClientRegistrationRollsBackBothSwitches(t *testing.T) {
	m := fixture(t)
	digest := fmt.Sprintf("%x", sha256.Sum256(m.helper))
	collision := m.launcher(Names[1], digest)
	if err := os.MkdirAll(filepath.Dir(collision), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(collision, []byte("existing client"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.apply(State{}, [2]bool{true, true}); err == nil {
		t.Fatal("file collision accepted")
	}
	if s, err := load(m.root); err != nil || s != (State{}) {
		t.Fatal("partial registration", err)
	}
	if _, err := os.Stat(m.launcher(Names[0], digest)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("orphaned launcher")
	}
	if got := regValue(t, m.root, "Environment", "Path", nil); got.Kind != 0 {
		t.Fatal("partial PATH change")
	}
}

func TestTransactedCurrentUserHandle(t *testing.T) {
	m := fixture(t)
	path := fmt.Sprintf(`Software\OShellAliasTest-%d-%s\probe`, os.Getpid(), t.Name())
	if err := transact(registry.CURRENT_USER, func(tx transaction) error {
		key, _, err := tx.key(path)
		if err != nil {
			return err
		}
		defer key.Close()
		return key.SetStringValue("", "fixture")
	}); err != nil {
		t.Fatal(err)
	}
	if got := regValue(t, m.root, "probe", "", nil); got.Text != "fixture" {
		t.Fatal("value missing")
	}
}

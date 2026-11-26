package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"oshell-desktop/internal/clientalias"
	"oshell-desktop/internal/core"
)

// Run the real launcher entry point in a fresh process with an isolated HKCU.
// The destination process records only synthetic arguments in the temp dir.
func TestMain(m *testing.M) {
	if key := os.Getenv("OSHELL_FIXTURE_ALIAS_HIVE"); key != "" {
		exe, err := os.Executable()
		if err != nil {
			os.Exit(2)
		}
		if slices.Contains(clientalias.Names[:], strings.ToLower(filepath.Base(exe))) {
			root, err := registry.OpenKey(registry.CURRENT_USER, key, registry.READ)
			if err != nil {
				os.Exit(3)
			}
			override := windows.NewLazySystemDLL("advapi32.dll").NewProc("RegOverridePredefKey")
			predefined := registry.Key(registry.CURRENT_USER)
			if r, _, _ := override.Call(uintptr(int32(predefined)), uintptr(root)); r != 0 {
				fmt.Fprintln(os.Stderr, "RegOverridePredefKey", r)
				os.Exit(4)
			}
			main()
			os.Exit(0)
		}
		data, err := json.Marshal(os.Args[1:])
		if err != nil || os.WriteFile(os.Getenv("OSHELL_FIXTURE_ALIAS_OUTPUT"), data, 0600) != nil {
			os.Exit(5)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestClientForwardsSSHArguments(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "程序 空格")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "OShell Desktop.exe")
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	targetData := data
	portable := os.Getenv("OSHELL_TEST_PORTABLE_CLIENT")
	received := make(chan core.ExternalLaunchRequest, 2)
	if portable != "" {
		targetData, err = os.ReadFile(portable)
		if err != nil {
			t.Fatal(err)
		}
		server, err := core.StartLaunchServer(dir, func(_ context.Context, r core.ExternalLaunchRequest) error {
			received <- r
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		defer server.Close()
	}
	if err := os.WriteFile(dest, targetData, 0700); err != nil {
		t.Fatal(err)
	}
	hive := fmt.Sprintf(`Software\OShell-Launcher-Fixture-%d`, os.Getpid())
	subkeys := []string{hive, hive + `\Software`, hive + `\Software\OShell Desktop`, hive + `\Software\OShell Desktop\ClientAliases`}
	root, exists, err := registry.CreateKey(registry.CURRENT_USER, hive, registry.ALL_ACCESS)
	if err != nil || exists {
		t.Fatal("isolated hive", err)
	}
	root.Close()
	t.Cleanup(func() {
		for _, subkey := range slices.Backward(subkeys) {
			if err := registry.DeleteKey(registry.CURRENT_USER, subkey); err != nil {
				t.Error(err)
			}
		}
	})
	k, _, err := registry.CreateKey(registry.CURRENT_USER, subkeys[len(subkeys)-1], registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	requests := [][]string{
		{"-ssh", "-l", "test", "-pw", `fixture "%@&" ` + "\\", "-P", "2222", "example.test"},
		{"-url", "ssh://test:fixture%40@example.test:2202", "-newtab", "测试 资产"},
	}
	for i, name := range clientalias.Names {
		alias := filepath.Join(dir, name)
		if err := os.WriteFile(alias, data, 0700); err != nil {
			t.Fatal(err)
		}
		var s clientalias.State
		s[i].Target, s[i].Launcher, s[i].Digest = dest, alias, strings.Repeat("0", 64)
		record, err := json.Marshal(s[i])
		if err != nil {
			t.Fatal(err)
		}
		if err := k.SetStringValue(name, string(record)); err != nil {
			t.Fatal(err)
		}
		out := filepath.Join(dir, name+".json")
		cmd := exec.Command(alias, requests[i]...)
		cmd.Env = append(os.Environ(), "OSHELL_FIXTURE_ALIAS_HIVE="+hive, "OSHELL_FIXTURE_ALIAS_OUTPUT="+out)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatal(name, err, string(output))
		}
		if portable != "" {
			select {
			case r := <-received:
				expected, err := core.ParseBastionSSHLaunch(requests[i])
				v := r.Terminal
				if err != nil || v == nil || v.Password == nil || expected.Password == nil || *v.Password != *expected.Password ||
					v.Host != expected.Host || v.Username != expected.Username || v.Port != expected.Port || v.Title != expected.Title || !v.BastionSSH {
					t.Fatal(name, "portable SSH request mismatch")
				}
			default:
				t.Fatal(name, "portable did not forward the request")
			}
			continue
		}
		got, err := os.ReadFile(out)
		var args []string
		if err != nil || json.Unmarshal(got, &args) != nil || !slices.Equal(args, append([]string{"--oshell-bastion-client"}, requests[i]...)) {
			t.Fatal(name, "arguments did not reach the destination intact")
		}
	}
}

package core

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestP2I10LiteralArgumentsAndByteLimit(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ssh.exe"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	command, err := ParseToolCommand(`ssh target | && ; > >> < &`)
	want := []string{"target", "|", "&&", ";", ">", ">>", "<", "&"}
	if err != nil || !reflect.DeepEqual(command.Arguments, want) {
		t.Fatal("literal argv changed", command.Arguments, err)
	}
	if _, err = ParseToolCommand("ssh " + strings.Repeat("x", 8188)); err != nil {
		t.Fatal("8192-byte boundary", err)
	}
	if _, err = ParseToolCommand("ssh " + strings.Repeat("x", 8189)); err == nil {
		t.Fatal("8193 bytes accepted")
	}
	if _, err = ParseToolCommand("powershell whoami"); err == nil {
		t.Fatal("tool whitelist widened")
	}
}

func TestWindowsToolPathsAndArguments(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"ssh.exe", "CURL.EXE"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	for _, tc := range []struct {
		line string
		want []string
	}{
		{`ssh -i .\keys\id_ed25519 user@host`, []string{"-i", `.\keys\id_ed25519`, "user@host"}},
		{`ssh -i "\\server\share\private key" user@host`, []string{"-i", `\\server\share\private key`, "user@host"}},
		{`CURL.EXE --output "C:\下载目录\" https://example.invalid`, []string{"--output", `C:\下载目录\`, "https://example.invalid"}},
	} {
		t.Run(tc.line, func(t *testing.T) {
			got, err := ParseToolCommand(tc.line)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.Arguments, tc.want) {
				t.Fatalf("arguments = %#v, want %#v", got.Arguments, tc.want)
			}
		})
	}
}

func TestWindowsSpecialTransferNames(t *testing.T) {
	for _, name := range []string{"COM¹.txt", "LPT²", "com³.log", "CONIN$", "CONOUT$", "NUL .txt"} {
		if SafeTransferName(name) {
			t.Errorf("Windows device name accepted: %q", name)
		}
	}
}

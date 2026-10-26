package terminal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestP2ToolArgvChild(t *testing.T) {
	if os.Getenv("OSHELL_P2_ARGV_CHILD") != "1" {
		return
	}
	var args []string
	for i, arg := range os.Args {
		if arg == "--" {
			args = os.Args[i+1:]
			break
		}
	}
	data, _ := json.Marshal(args)
	if err := os.WriteFile(os.Getenv("OSHELL_P2_ARGV_RESULT"), data, 0600); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}
func TestP2I10ProductionToolReceivesLiteralArgv(t *testing.T) {
	loadLib(t)
	dir := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "ssh.exe"), data, 0700); err != nil {
		t.Fatal(err)
	}
	result := filepath.Join(dir, "argv.json")
	t.Setenv("PATH", dir)
	t.Setenv("OSHELL_P2_ARGV_CHILD", "1")
	t.Setenv("OSHELL_P2_ARGV_RESULT", result)
	console := NewToolsConsole()
	term, err := New(Options{Conn: console, NoBlink: true})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	deadline := time.Now().Add(3 * time.Second)
	for !strings.Contains(term.Text(), "oshell>") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	term.Send([]byte("ssh -test.run=^TestP2ToolArgvChild$ -- \"|\" \"&&\" \";\" \"a>b\"\r"))
	deadline = time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		data, err = os.ReadFile(result)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	var args []string
	if json.Unmarshal(data, &args) != nil || strings.Join(args, "\x00") != "|\x00&&\x00;\x00a>b" {
		t.Fatalf("literal argv not received: %q (%v)", data, err)
	}
}

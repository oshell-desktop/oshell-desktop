package terminal

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"oshell-desktop/internal/core"
)

func TestMCPExistingBashPTYApprovalExecutionAndRevoke(t *testing.T) {
	loadLib(t)
	bash := os.Getenv("OSHELL_TEST_BASH")
	if bash == "" {
		if runtime.GOOS == "windows" {
			t.Skip("set OSHELL_TEST_BASH for real Bash PTY acceptance")
		}
		bash = "/bin/bash"
	}
	root := t.TempDir()
	clients, err := core.OpenMCPClients(root)
	if err != nil {
		t.Fatal(err)
	}
	client, _, err := clients.Add("PTY fixture")
	if err != nil {
		t.Fatal(err)
	}
	coordinator := core.NewAgentCoordinator(root)
	defer coordinator.Close()
	s := coordinator.Register(core.NewID(), "isolated Bash")
	term, err := New(Options{Command: []string{bash, "--noprofile", "--norc", "-i"}, Dir: root, Env: []string{"PS1=fixture> ", "HISTFILE=/dev/null"}, Agent: s, NoBlink: true})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	s.Bind(term)
	wait := func(name string, condition func() bool) {
		t.Helper()
		deadline := time.Now().Add(8 * time.Second)
		for !condition() {
			if time.Now().After(deadline) {
				t.Fatalf("%s: state %+v screen %q", name, s.State(), term.Text())
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	wait("initial prompt", func() bool { return strings.Contains(term.Text(), "fixture>") })
	if err = s.Grant(client.ID, "ask"); err != nil {
		t.Fatal(err)
	}
	if err = s.Integrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	wait("integration", func() bool { return s.State().ShellReady })
	call := func(name string, args map[string]any) any {
		t.Helper()
		b, _ := json.Marshal(args)
		v, err := coordinator.Call(context.Background(), client.ID, name, b)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return v
	}
	v := s.State()
	task := call("task_start", map[string]any{"session_id": v.ID, "generation": v.Generation}).(map[string]any)["task_id"].(string)
	args := map[string]any{"task_id": task, "command": "printf 'mcp-fixture-output\\n'", "idempotency_key": core.NewID()}
	op := call("command_submit", args).(core.AgentOperation)
	if op.State != "pending_approval" {
		t.Fatal(op)
	}
	if strings.Contains(term.Text(), "mcp-fixture-output") {
		t.Fatal("unapproved command sent")
	}
	if err = s.Approve(op.ID, op.Command+" extra", true); err == nil {
		t.Fatal("changed approval accepted")
	}
	if err = s.Approve(op.ID, op.Command, true); err != nil {
		t.Fatal(err)
	}
	wait("command completion", func() bool { return s.State().Operation.State == "completed" })
	result := call("operation_read", map[string]any{"operation_id": op.ID, "limit": 65536}).(map[string]any)
	data, err := base64.StdEncoding.DecodeString(result["output_base64"].(string))
	if err != nil || !strings.Contains(string(data), "mcp-fixture-output") {
		t.Fatal(string(data), err)
	}
	again := call("command_submit", args).(core.AgentOperation)
	if again.ID != op.ID || again.State != "completed" {
		t.Fatal("idempotency failed", again)
	}
	call("task_end", map[string]any{"task_id": task})
	// Queued input is checked again by the real input worker after revocation.
	task = call("task_start", map[string]any{"session_id": v.ID, "generation": v.Generation}).(map[string]any)["task_id"].(string)
	op = call("command_submit", map[string]any{"task_id": task, "command": "printf should-not-run", "idempotency_key": core.NewID()}).(core.AgentOperation)
	s.Revoke()
	if err = s.Approve(op.ID, op.Command, true); err == nil {
		t.Fatal("revoked approval executed")
	}
	term.Send([]byte("printf 'manual-after-revoke\\n'\r"))
	wait("manual connection preserved", func() bool { return strings.Contains(term.Text(), "manual-after-revoke") })
	if err = s.EndArchive(); err != nil {
		t.Fatal(err)
	}
	archives, _, err := core.ListAgentAudits(root)
	if err != nil || len(archives) != 1 || !archives[0].Sealed {
		t.Fatal(archives, err)
	}
	if err = core.ReadAgentAudit(context.Background(), root, archives[0].ID, nil); err != nil {
		t.Fatal(err)
	}
}
func TestMCPReadOnlyReplayAndScreenBaseline(t *testing.T) {
	loadLib(t)
	term, err := NewReplay(Font{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	term.Resize(30, 4)
	term.Feed([]byte(strings.Repeat("private-old-history\r\n", 20) + "visible-now"))
	screen, text, _, _, _ := term.AgentSnapshot()
	if strings.Count(text, "private-old-history") > 4 {
		t.Fatal("baseline includes scrollback")
	}
	restored, err := NewReplay(Font{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	restored.Resize(30, 4)
	restored.Feed(screen)
	if !strings.Contains(restored.Text(), "visible-now") {
		t.Fatal("baseline not replayable", restored.Text())
	}
	if term.proc != nil || term.launch != nil {
		t.Fatal("replay started process")
	}
	term.mu.Lock()
	sent := term.sendInputLocked([]byte("must not send"))
	term.mu.Unlock()
	if sent {
		t.Fatal("replay accepted input")
	}
	term.Feed([]byte("\x1b]52;c;c2VjcmV0\x07\x1b[6n"))
	term.in.mu.Lock()
	queued := len(term.in.queue)
	term.in.mu.Unlock()
	if queued != 0 {
		t.Fatal("replay emitted terminal response")
	}
}

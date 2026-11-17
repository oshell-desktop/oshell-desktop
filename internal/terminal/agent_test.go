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
	client, credential, err := clients.Add("PTY fixture")
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
	if runtime.GOOS == "windows" && term.AgentRawOutputOrdered() {
		t.Fatal("started ConPTY incorrectly reported ordered output")
	}
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
	if state := s.State(); !state.ControlActive || !state.ShellReady || state.Task != "" {
		t.Fatal("task_end lost access", state)
	}
	stateOnly := call("session_state", map[string]any{"session_id": v.ID, "include_screen": false}).(map[string]any)
	if _, exists := stateOnly["screen"]; exists || stateOnly["can_exec"] != true {
		t.Fatal(stateOnly)
	}
	shortcutArgs := map[string]any{"session_id": v.ID, "generation": v.Generation, "command": "printf 'alpha\\r\\nbeta中文\\n'", "idempotency_key": core.NewID()}
	shortcut := call("session_exec", shortcutArgs).(core.AgentOperation)
	if shortcut.State != "pending_approval" {
		t.Fatal(shortcut)
	}
	if err := s.Approve(shortcut.ID, shortcut.Command, true); err != nil {
		t.Fatal(err)
	}
	view := "text"
	if runtime.GOOS == "windows" {
		view = "raw"
	}
	textResult := call("operation_read", map[string]any{"operation_id": shortcut.ID, "view": view, "format": "compact", "wait_for": "completion", "wait_ms": 3000}).(map[string]any)
	if textResult["completed"] != true {
		t.Fatal(textResult)
	}
	if view == "text" {
		if textResult["output_text"] != "alpha\nbeta中文\n" || textResult["body_ended"] != true {
			t.Fatal(textResult)
		}
		query := call("operation_read", map[string]any{"operation_id": shortcut.ID, "view": "text", "tail_lines": 1, "contains": "beta"}).(map[string]any)
		if query["output_text"] != "beta中文\n" {
			t.Fatal(query)
		}
	} else {
		request, _ := json.Marshal(map[string]any{"operation_id": shortcut.ID, "view": "text"})
		if _, err := coordinator.Call(context.Background(), client.ID, "operation_read", request); err == nil || !strings.Contains(err.Error(), "ConPTY") {
			t.Fatal(err)
		}
		wait("rendered command body", func() bool {
			return strings.Contains(term.Text(), "alpha\n") && strings.Contains(term.Text(), "beta中文\n")
		})
		raw := call("operation_read", map[string]any{"operation_id": shortcut.ID, "format": "compact"}).(map[string]any)
		if !strings.Contains(raw["output_text"].(string), "alpha\r\nbeta中文") {
			t.Fatal(raw)
		}
	}
	if s.State().Task != "" || !s.Busy() {
		t.Fatal("automatic task not released independently")
	}

	if again := call("session_exec", shortcutArgs).(core.AgentOperation); again.ID != shortcut.ID {
		t.Fatal("shortcut replayed")
	}
	if err := s.ChangePermission("assist"); err != nil {
		t.Fatal(err)
	}
	server, err := core.StartMCPServer(root, clients, coordinator.Call, coordinator.Disconnected)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	bridge := core.NewMCPBridge(root, credential)
	defer bridge.Close()
	request, _ := json.Marshal(map[string]any{"session_id": v.ID, "generation": v.Generation, "command": "pwd", "idempotency_key": core.NewID(), "view": view, "wait_ms": 3000})
	bridgeOutput, err := bridge.Call(context.Background(), "session_exec", request)
	if err != nil {
		t.Fatal(err)
	}
	if result := bridgeOutput.(map[string]any); result["completed"] != true || result["encoding"] != "utf8" {
		t.Fatal(result)
	}
	if err := s.ChangePermission("ask"); err != nil {
		t.Fatal(err)
	}
	// A released task retains a running operation until the real shell end marker.
	task = call("task_start", map[string]any{"session_id": v.ID, "generation": v.Generation}).(map[string]any)["task_id"].(string)
	stopping := call("command_submit", map[string]any{"task_id": task, "command": "sleep 0.3; printf 'after-stop\\n'", "idempotency_key": core.NewID()}).(core.AgentOperation)
	if err := s.Approve(stopping.ID, stopping.Command, true); err != nil {
		t.Fatal(err)
	}
	wait("started before stop", func() bool { return s.State().Operation.State == "running" })
	call("task_stop", map[string]any{"task_id": task})
	if !s.Busy() || s.State().Operation.State != "running" {
		t.Fatal("stop changed command result", s.State())
	}
	stopped := call("operation_read", map[string]any{"operation_id": stopping.ID, "view": view, "format": "compact", "wait_ms": 3000, "wait_for": "completion"}).(map[string]any)
	if stopped["completed"] != true || view == "text" && stopped["output_text"] != "after-stop\n" {
		t.Fatal(stopped)
	}
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

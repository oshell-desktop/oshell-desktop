package core

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// The peer isolates the external shell; coordinator, audit, transport and query
// implementations remain real. Output is supplied with explicit PTY boundaries.
type agentShellPeer struct{ session *AgentSession }

func (p *agentShellPeer) AgentSnapshot() ([]byte, string, int, int, bool) {
	return []byte("fixture> "), "fixture> ", 80, 24, true
}
func (p *agentShellPeer) AgentSend(_ context.Context, data []byte, ticket string) error {
	return p.session.Gate(ticket, data, func() error { return nil })
}

func TestMCPV067OrderedOutputApprovalAndWait(t *testing.T) {
	root := t.TempDir()
	if _, err := OpenMCPClients(root); err != nil {
		t.Fatal(err)
	}
	coordinator := NewAgentCoordinator(root)
	defer coordinator.Close()
	var notifications atomic.Int64
	s := coordinator.Register(NewID(), "synthetic remote shell", func() { notifications.Add(1) })
	s.Bind(&agentShellPeer{s})
	client := NewID()
	if err := s.Grant(client, "ask"); err != nil {
		t.Fatal(err)
	}
	s.Observe([]byte("\x1b]779;OSHELL;" + s.token + ";ready\a"))
	call := func(name string, args map[string]any) any {
		t.Helper()
		data, _ := json.Marshal(args)
		result, err := coordinator.Call(context.Background(), client, name, data)
		if err != nil {
			t.Fatal(name, err)
		}
		return result
	}
	args := map[string]any{"session_id": s.id, "generation": s.generation, "command": "printf fixture", "idempotency_key": NewID()}
	before := notifications.Load()
	op := call("session_exec", args).(AgentOperation)
	if notifications.Load() == before {
		t.Fatal("approval did not notify the UI without terminal output")
	}
	result := call("operation_read", map[string]any{"operation_id": op.ID, "format": "compact", "wait_for": "approval", "wait_ms": 1}).(map[string]any)
	if result["approval"] == nil || result["next_action"] != "approve_in_oshell" {
		t.Fatal(result)
	}
	if err := s.Approve(op.ID, op.Command, true); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	data, _ := json.Marshal(map[string]any{"operation_id": op.ID, "wait_ms": 10000})
	if _, err := coordinator.Call(ctx, client, "operation_read", data); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	done := make(chan any, 1)
	go func() {
		data, _ := json.Marshal(map[string]any{"operation_id": op.ID, "view": "text", "wait_for": "completion", "wait_ms": 3000})
		v, err := coordinator.Call(context.Background(), client, "operation_read", data)
		if err != nil {
			done <- err
		} else {
			done <- v
		}
	}()
	output := "echo\r\n\x1b]779;OSHELL;" + s.token + ";start\a" + "\x1b[31malpha\x1b[0m\r\nbeta中文\n" + "\x1b]779;OSHELL;" + s.token + ";end;0\a" + "fixture> "
	// Split control sequences and Unicode, as a real transport is allowed to do.
	for i := range len(output) {
		s.Observe([]byte(output[i : i+1]))
	}
	select {
	case v := <-done:
		result, ok := v.(map[string]any)
		if !ok || result["output_text"] != "alpha\nbeta中文\n" || result["completed"] != true || result["body_ended"] != true {
			t.Fatal(v)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("completion waiter did not wake")
	}
	if s.State().Task != "" || !s.State().ControlActive {
		t.Fatal("automatic task revoked access")
	}
	query := call("operation_read", map[string]any{"operation_id": op.ID, "view": "text", "tail_lines": 1, "contains": "beta"}).(map[string]any)
	if query["output_text"] != "beta中文\n" || query["cursor_unit"] != "raw_bytes" {
		t.Fatal(query)
	}
	if again := call("session_exec", args).(AgentOperation); again.ID != op.ID || again.State != "completed" {
		t.Fatal(again)
	}
	args["idempotency_key"] = NewID()
	pending := call("session_exec", args).(AgentOperation)
	call("task_stop", map[string]any{"task_id": pending.Task})
	if err := s.Approve(pending.ID, pending.Command, true); err == nil {
		t.Fatal("cancelled approval executed")
	}
	if !s.Busy() {
		t.Fatal("task_stop revoked access")
	}
}

func TestMCPV067TextChunksAndPromptSpacer(t *testing.T) {
	for _, width := range []int{1, 7, 4096} {
		f := agentTextFilter{token: "fixture"}
		f.spacer.ending = []byte("\r \r\x1b]779;OSHELL;fixture;end;")
		data := []byte("echo\r\n\x1b]779;OSHELL;fixture;start\a\x1b[31m中文\x1b[0m\r\nlast\x1b[1m\x1b[7m%\x1b[27m\x1b[1m\x1b[0m    \r \r\x1b]779;OSHELL;fixture;end;0\auser> ")
		var text []byte
		for len(data) > 0 {
			n := min(width, len(data))
			text = append(text, f.consume(data[:n])...)
			data = data[n:]
		}
		if string(text) != "中文\nlast" || !f.ended {
			t.Fatalf("width %d: %q %+v", width, text, f)
		}
	}
}

func TestMCPV067QueryPolicyAndValidation(t *testing.T) {
	for _, command := range []string{"free -h", "free --total --wide", "docker ps -a --last 10", "docker ps --filter status=running --format '{{.Names}}\\t{{.Status}}'"} {
		if !agentQueryAllowed(command) {
			t.Fatal("query not accepted", command)
		}
	}
	for _, command := range []string{"free -s 1", "docker ps --format '{{exec \"sh\"}}'", "docker ps --filter name=$(whoami)", "docker ps; pwd", "docker ps --last 1001", "free | sh"} {
		if agentQueryAllowed(command) {
			t.Fatal("non-query accepted", command)
		}
	}
	for _, invalid := range []string{`{"operation_id":"x","wait_ms":10001}`, `{"operation_id":"x","view":"text","format":"legacy"}`, `{"operation_id":"x","contains":"word"}`, `{"operation_id":"x","view":"text","tail_lines":0}`} {
		if ValidateMCPTool("operation_read", json.RawMessage(invalid)) == nil {
			t.Fatal(invalid)
		}
	}
	e := NewAgentIntegrationEcho("fixture command")
	var visible []byte
	for _, chunk := range []string{"late prompt> fixt", "ure command\r\n", "result\r\n"} {
		visible = append(visible, e.Consume([]byte(chunk))...)
	}
	visible = append(visible, e.Flush()...)
	if strings.Contains(string(visible), "fixture command") || !strings.Contains(string(visible), "late prompt> ") || !strings.Contains(string(visible), "result") {
		t.Fatal(string(visible))
	}
	e = NewAgentIntegrationEcho("fixture command")
	visible = e.Consume([]byte("fixture unexpected output"))
	visible = append(visible, e.Flush()...)
	if string(visible) != "fixture unexpected output" {
		t.Fatal(string(visible))
	}
}

func TestMCPV067BridgeRestartWaitAndReceipt(t *testing.T) {
	root := t.TempDir()
	clients, err := OpenMCPClients(root)
	if err != nil {
		t.Fatal(err)
	}
	_, token, err := clients.Add("fixture")
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	handler := func(ctx context.Context, _, name string, args json.RawMessage) (any, error) {
		switch name {
		case "sessions_list":
			return []any{}, nil
		case "session_exec":
			return map[string]any{"operation_id": "receipt", "state": "dispatched"}, nil
		case "operation_read":
			var params struct {
				Operation string `json:"operation_id"`
			}
			json.Unmarshal(args, &params)
			if params.Operation == "receipt" {
				return nil, errors.New("fixture read unavailable")
			}
			close(entered)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-release:
				return map[string]any{}, nil
			}
		}
		return nil, errors.New("unexpected tool")
	}
	server, err := StartMCPServer(root, clients, handler)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { server.Close() }()
	bridge := NewMCPBridge(root, token)
	defer bridge.Close()
	if _, err = bridge.Call(context.Background(), "sessions_list", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	if err = server.Close(); err != nil {
		t.Fatal(err)
	}
	server, err = StartMCPServer(root, clients, handler)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = bridge.Call(context.Background(), "sessions_list", json.RawMessage(`{}`)); err != nil {
		t.Fatal("idle connection did not reconnect", err)
	}
	waitDone := make(chan error, 1)
	go func() {
		_, err := bridge.Call(context.Background(), "operation_read", json.RawMessage(`{"operation_id":"wait","wait_ms":10000}`))
		waitDone <- err
	}()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err = bridge.Call(ctx, "sessions_list", json.RawMessage(`{}`)); err != nil {
		t.Fatal("wait blocked control connection", err)
	}
	close(release)
	if err = <-waitDone; err != nil {
		t.Fatal(err)
	}
	result, err := bridge.Call(context.Background(), "session_exec", json.RawMessage(`{"session_id":"session","generation":"generation","command":"pwd","idempotency_key":"key"}`))
	if err != nil {
		t.Fatal(err)
	}
	receipt := result.(map[string]any)
	if receipt["operation_id"] != "receipt" || receipt["state"] != "dispatched" || receipt["read_error"] == nil {
		t.Fatal("post-read lost execution receipt", result)
	}
}

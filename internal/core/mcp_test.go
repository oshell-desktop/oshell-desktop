package core

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMCPClientEncryptionRevokeAndMissingKey(t *testing.T) {
	root := t.TempDir()
	clients, err := OpenMCPClients(root)
	if err != nil {
		t.Fatal(err)
	}
	c, token, err := clients.Add("fixture")
	if err != nil {
		t.Fatal(err)
	}
	if c.Digest != "" || len(token) < 40 {
		t.Fatal("invalid public credential result")
	}
	b, _ := os.ReadFile(filepath.Join(root, "agent", "clients.bin"))
	if bytes.Contains(b, []byte(token)) || bytes.Contains(b, []byte("fixture")) {
		t.Fatal("plaintext registry")
	}
	again, err := OpenMCPClients(root)
	if err != nil {
		t.Fatal(err)
	}
	if id, err := again.Authenticate(token); err != nil || id != c.ID {
		t.Fatal(id, err)
	}
	if err = again.Revoke(c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = again.Authenticate(token); err == nil {
		t.Fatal("revoked token accepted")
	}
	if _, _, err = again.Add("\n"); err == nil {
		t.Fatal("invalid name accepted")
	}
	for i := 0; i < 32; i++ {
		if _, _, err = again.Add("fixture"); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err = again.Add("overflow"); err == nil {
		t.Fatal("client bound missing")
	}
	if err = os.Remove(filepath.Join(root, "agent", "credential-key")); err != nil {
		t.Fatal(err)
	}
	if _, err = OpenMCPClients(root); err == nil {
		t.Fatal("missing key reset existing clients")
	}
}
func TestMCPRealPrivateTransportAndRevocation(t *testing.T) {
	root := t.TempDir()
	clients, err := OpenMCPClients(root)
	if err != nil {
		t.Fatal(err)
	}
	client, token, _ := clients.Add("transport fixture")
	launch, err := StartLaunchServer(root, func(context.Context, ExternalLaunchRequest) error { return errors.New("unexpected launch") })
	if err != nil {
		t.Fatal(err)
	}
	defer launch.Close()
	server, err := StartMCPServer(root, clients, func(ctx context.Context, id, tool string, args json.RawMessage) (any, error) {
		if id != client.ID || tool != "sessions_list" {
			return nil, errors.New("wrong identity")
		}
		return []any{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	bridge := NewMCPBridge(root, token)
	defer bridge.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err = bridge.Call(ctx, "sessions_list", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	if _, err = bridge.Call(ctx, "sessions_list", json.RawMessage(`{"extra":1}`)); err == nil {
		t.Fatal("unknown fields accepted")
	}
	if err = clients.Revoke(client.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = bridge.Call(ctx, "sessions_list", json.RawMessage(`{}`)); err == nil {
		t.Fatal("open connection retained revoked access")
	}
	done := make(chan error, 1)
	go func() { done <- server.Close() }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown blocked on idle peer")
	}
}
func TestMCPStdioLifecycleToolsAndValidation(t *testing.T) {
	in, send := io.Pipe()
	receive, out := io.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- ServeMCPStdio(context.Background(), in, out, "1.2.3", func(ctx context.Context, name string, args json.RawMessage) (any, error) {
			return map[string]string{"fixture": "ok"}, nil
		})
		out.Close()
	}()
	defer in.Close()
	defer receive.Close()
	enc, dec := json.NewEncoder(send), json.NewDecoder(receive)
	request := func(id int, method string, params any) map[string]any {
		t.Helper()
		if err := enc.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
			t.Fatal(err)
		}
		var result map[string]any
		if err := dec.Decode(&result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	if r := request(0, "tools/list", nil); r["error"] == nil {
		t.Fatal("pre-init request accepted")
	}
	r := request(1, "initialize", map[string]any{"protocolVersion": "2025-06-18", "clientInfo": map[string]string{"name": "test", "version": "1"}, "capabilities": map[string]any{"experimental": map[string]any{"nested": map[string]bool{"supported": true}}}})
	if r["error"] != nil {
		t.Fatal(r)
	}
	if version := r["result"].(map[string]any)["serverInfo"].(map[string]any)["version"]; version != "1.2.3" {
		t.Fatalf("MCP advertised version %v", version)
	}
	enc.Encode(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	r = request(2, "tools/list", nil)
	tools := r["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 7 {
		t.Fatal(tools)
	}
	r = request(3, "tools/call", map[string]any{"name": "sessions_list", "arguments": map[string]any{}})
	if r["result"].(map[string]any)["isError"] != false {
		t.Fatal(r)
	}
	r = request(4, "tools/call", map[string]any{"name": "operation_read", "arguments": map[string]any{"operation_id": "x", "limit": 65537}})
	if r["error"] == nil {
		t.Fatal("over-limit output accepted")
	}
	send.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("EOF did not close helper")
	}
}
func TestMCPAuditIntegrityRedactionAndExport(t *testing.T) {
	root := t.TempDir()
	if _, err := OpenMCPClients(root); err != nil {
		t.Fatal(err)
	}
	a, err := NewAgentAudit(root, AgentAuditIndex{Session: NewID(), Client: NewID(), Title: "fixture"}, []byte("baseline"), 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	op := NewID()
	if err = a.Append(AgentAuditEvent{Kind: "output", Operation: op, Bytes: []byte("audit-only-fixture")}); err != nil {
		t.Fatal(err)
	}
	if err = ReadAgentAudit(context.Background(), root, a.ID(), nil); !errors.Is(err, ErrAgentAuditUnsealed) {
		t.Fatal("live archive treated as sealed", err)
	}
	if err = a.Close(); err != nil {
		t.Fatal(err)
	}
	count := 0
	err = ReadAgentAudit(context.Background(), root, a.ID(), func(e AgentAuditEvent) error { count++; return nil })
	if err != nil || count != 3 {
		t.Fatal(count, err)
	}
	hits, err := SearchAgentAudits(context.Background(), root, "audit-only", "", "", "", time.Time{}, time.Time{})
	if err != nil || len(hits) != 1 || !strings.Contains(hits[0].Preview, "audit-only-fixture") || hits[0].Problem != "" {
		t.Fatal("search preview", hits, err)
	}
	path, _ := agentAuditPath(root, a.ID())
	data, _ := os.ReadFile(path)
	if bytes.Contains(data, []byte("audit-only-fixture")) {
		t.Fatal("plaintext audit")
	}
	export := filepath.Join(root, "fixture.export")
	if err = ExportAgentAudit(context.Background(), root, a.ID(), "synthetic-password-123", export); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(export)
	if !bytes.HasPrefix(b, []byte(agentExportMagic)) {
		t.Fatal("wrong export magic")
	}
	n := int(binary.BigEndian.Uint32(b[len(agentExportMagic):]))
	envelope := b[len(agentExportMagic)+4 : len(agentExportMagic)+4+n]
	if _, err = OpenShared(envelope, []byte("wrong")); err == nil {
		t.Fatal("wrong export password accepted")
	}
	key, err := OpenShared(envelope, []byte("synthetic-password-123"))
	if err != nil {
		t.Fatal(err)
	}
	defer clear(key)
	if err = readAgentAudit(context.Background(), bytes.NewReader(b[len(agentExportMagic)+4+n:]), a.ID(), key, nil); err != nil {
		t.Fatal(err)
	}
	data[len(data)-1] ^= 1
	os.WriteFile(path, data, 0600)
	if err = ReadAgentAudit(context.Background(), root, a.ID(), nil); err == nil {
		t.Fatal("modified audit accepted")
	}
	hits, err = SearchAgentAudits(context.Background(), root, "audit-only", "", "", "", time.Time{}, time.Time{})
	if err != nil || len(hits) != 1 || hits[0].Problem == "" || hits[0].Sealed {
		t.Fatal("search hid corrupt archive", hits, err)
	}
	r := agentRedactor{values: [][]byte{[]byte("sensitive-value")}}
	var text []byte
	for _, part := range []string{"before sens", "itive-", "value after"} {
		text = append(text, r.consume([]byte(part), false)...)
	}
	text = append(text, r.consume(nil, true)...)
	if string(text) != "before [敏感输入] after" {
		t.Fatal(string(text))
	}
}
func TestMCPCommandPolicy(t *testing.T) {
	for _, tc := range []struct{ command, permission, want string }{{"pwd", "ask", "approval"}, {"pwd", "assist", "automatic"}, {"date -s tomorrow", "full", "approval"}, {"cat /tmp/a", "assist", "automatic"}, {"mkdir test", "full", "automatic"}, {"mkdir test", "assist", "approval"}, {"echo $(pwd)", "full", "approval"}, {"pwd; whoami", "full", "approval"}, {"sudo ls", "full", "human"}, {"rm -rf /", "ask", "deny"}, {"dd if=/dev/zero", "full", "deny"}, {"ls\x00", "full", "deny"}} {
		decision, _ := AgentCommandDecision(tc.command, tc.permission)
		if decision != tc.want {
			t.Errorf("%q got %s want %s", tc.command, decision, tc.want)
		}
	}
	// Configuration must quote Windows paths as literal TOML/JSON strings.
	cfg := MCPClientConfiguration(`C:\OShell folder\oshell.exe`, `D:\data`, "fixture-token", "Codex")
	if !strings.Contains(cfg, `C:\\OShell folder\\oshell.exe`) {
		t.Fatal("configuration path not escaped")
	}
}

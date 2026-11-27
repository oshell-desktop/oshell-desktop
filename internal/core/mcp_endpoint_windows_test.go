package core

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestMCPWindowsEndpointExclusiveAndRestartable(t *testing.T) {
	root := t.TempDir()
	clients, err := OpenMCPClients(root)
	if err != nil {
		t.Fatal(err)
	}
	_, token, err := clients.Add("fixture")
	if err != nil {
		t.Fatal(err)
	}
	handler := func(context.Context, string, string, json.RawMessage) (any, error) { return []any{}, nil }
	server, err := StartMCPServer(root, clients, handler)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { server.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	bridge := NewMCPBridge(root, token)
	defer bridge.Close()
	if _, err := bridge.Call(ctx, "sessions_list", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	previous, err := os.ReadFile(mcpEndpoint(root) + ".endpoint")
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := StartMCPServer(root, clients, handler)
	if err == nil {
		duplicate.Close()
		t.Fatal("second server replaced a live endpoint")
	}
	after, err := os.ReadFile(mcpEndpoint(root) + ".endpoint")
	if err != nil || string(previous) != string(after) {
		t.Fatal("duplicate changed discovery", err)
	}
	for range 3 {
		if err := server.Close(); err != nil {
			t.Fatal(err)
		}
		next, err := StartMCPServer(root, clients, handler)
		if err != nil {
			t.Fatal("restart with old client handle", err)
		}
		server = next
		after, err := os.ReadFile(mcpEndpoint(root) + ".endpoint")
		if err != nil || string(previous) == string(after) {
			t.Fatal("endpoint reused", err)
		}
		previous = after
		if _, err := bridge.Call(ctx, "sessions_list", json.RawMessage(`{}`)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMCPWindowsRejectsInvalidDiscovery(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(root+"/agent", 0700); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{"", "../other", "00000000-0000-0000-0000-00000000000/", string(make([]byte, 4096))} {
		if err := os.WriteFile(mcpEndpoint(root)+".endpoint", []byte(invalid), 0600); err != nil {
			t.Fatal(err)
		}
		conn, err := dialMCP(context.Background(), root)
		if conn != nil {
			conn.Close()
		}
		if err == nil {
			t.Fatal("invalid discovery accepted")
		}
	}
}

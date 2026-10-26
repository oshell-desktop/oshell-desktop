//go:build darwin

package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLaunchIPCRefusesOccupiedEndpoint(t *testing.T) {
	config := t.TempDir()
	dir, e := launchDirectory(config)
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(dir, "s")
	if e = os.WriteFile(path, []byte("preserve"), 0600); e != nil {
		t.Fatal(e)
	}
	defer os.Remove(path)
	if server, e := StartLaunchServer(config, func(context.Context, ExternalLaunchRequest) error { return nil }); e == nil {
		server.Close()
		t.Fatal("replaced non-socket")
	}
	data, e := os.ReadFile(path)
	if e != nil || string(data) != "preserve" {
		t.Fatal("occupied endpoint changed")
	}
}

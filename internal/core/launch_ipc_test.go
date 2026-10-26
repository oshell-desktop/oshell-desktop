package core

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
)

func TestLaunchIPCDeduplicatesAndNeverWritesPasswords(t *testing.T) {
	config := t.TempDir()
	var count atomic.Int32
	server, e := StartLaunchServer(config, func(ctx context.Context, r ExternalLaunchRequest) error {
		count.Add(1)
		if r.Terminal.Password == nil || *r.Terminal.Password != "synthetic-once" {
			t.Error("lost credential")
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	defer server.Close()
	r, e := ParseZOCLaunch([]string{"/SSH:alice:synthetic-once@example.invalid"})
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		if e = SendExternalLaunch(context.Background(), config, ExternalLaunchRequest{Terminal: &r}); e != nil {
			t.Fatal(e)
		}
	}
	if count.Load() != 1 {
		t.Fatal("duplicate request opened twice")
	}
	if other, e := StartLaunchServer(config, func(context.Context, ExternalLaunchRequest) error { return nil }); e == nil {
		other.Close()
		t.Fatal("second server acquired same config")
	}
	files, e := os.ReadDir(config)
	if e != nil || len(files) != 0 {
		t.Fatal("one-shot data persisted")
	}
}

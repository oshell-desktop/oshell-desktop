package core

import (
	"context"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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

func TestExternalLaunchTimeoutExplainsUnlockWait(t *testing.T) {
	config := t.TempDir()
	received := make(chan struct{})
	server, err := StartLaunchServer(config, func(ctx context.Context, _ ExternalLaunchRequest) error {
		close(received)
		<-ctx.Done()
		return ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	r, err := ParseZOCLaunch([]string{"/SSH:fixture@example.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	err = SendExternalLaunch(ctx, config, ExternalLaunchRequest{Terminal: &r})
	select {
	case <-received:
	default:
		t.Fatal("request did not reach the server")
	}
	if err == nil || !strings.Contains(err.Error(), "主密码解锁") || !strings.Contains(err.Error(), "重新发起连接") {
		t.Fatalf("timeout omitted recovery instructions: %v", err)
	}
}

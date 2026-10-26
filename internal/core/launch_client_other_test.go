//go:build windows || linux

package core

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestExternalLaunchReusesServerAndPreservesRejection(t *testing.T) {
	dir := t.TempDir()
	var calls atomic.Int32
	server, err := StartLaunchServer(dir, func(context.Context, ExternalLaunchRequest) error {
		calls.Add(1)
		return errors.New("synthetic refusal")
	})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	r, err := ParseZOCLaunch([]string{"/SSH:tester@example.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = LaunchApplicationAndSend(ctx, dir, filepath.Join(dir, "must-not-start.exe"), ExternalLaunchRequest{Terminal: &r})
	if err == nil || calls.Load() != 1 {
		t.Fatal("server rejection was lost or a second app was launched", err, calls.Load())
	}
	cancel()
	if err := LaunchApplicationAndSend(ctx, t.TempDir(), "must-not-start.exe", ExternalLaunchRequest{Terminal: &r}); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled launch continued", err)
	}
}

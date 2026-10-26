package mygo

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestOpenContextAlreadyCancelledDoesNotRequireApp(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	paths, err := Dialog.OpenContext(ctx, OpenDialogOptions{})
	if len(paths) != 0 || !errors.Is(err, context.Canceled) {
		t.Fatalf("%v %v", paths, err)
	}
}

func TestDialogCancellationDispatchAndLifetime(t *testing.T) {
	for _, released := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		queued := make(chan func(), 1)
		calls := 0
		stop := bindDialogCancellation(ctx, func(fn func()) bool { queued <- fn; return true }, func() { calls++ })
		cancel()
		var callback func()
		select {
		case callback = <-queued:
		case <-time.After(time.Second):
			t.Fatal("close not scheduled")
		}
		if calls != 0 {
			t.Fatal("native close ran outside dispatcher")
		}
		if released {
			stop()
		}
		callback()
		if released && calls != 0 || !released && calls != 1 {
			t.Fatalf("released=%v calls=%d", released, calls)
		}
		stop()
	}
}

package core

import (
	"context"
	"errors"
	"time"
)

func lockLaunchStart(ctx context.Context, configuration string) (func(), error) {
	dir, err := launchDirectory(configuration)
	if err != nil {
		return nil, err
	}
	for {
		lock, err := launchLock(dir, "launch.lock")
		if err == nil {
			return func() { _ = lock.Close() }, nil
		}
		if !errors.Is(err, ErrLaunchBusy) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

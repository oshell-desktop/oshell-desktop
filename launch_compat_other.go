//go:build !darwin

package main

import (
	"context"
	"os"
	"oshell-desktop/internal/core"
	"time"
)

func forwardExternalLaunchIfRequested() error {
	args := os.Args[1:]
	helper := core.CompatibilityLauncher(args)
	if helper == "" {
		return nil
	}
	r, err := core.ParseExternalLauncher(helper, args)
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	configuration, err := core.LauncherConfigurationDirectory()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	if err = core.LaunchApplicationAndSend(ctx, configuration, executable, r); err != nil {
		return err
	}
	os.Exit(0)
	return nil
}

//go:build !darwin

package main

import (
	"context"
	"os"
	"oshell-desktop/internal/core"
	"oshell-desktop/internal/diagnostics"
	"runtime"
	"time"
)

func forwardExternalLaunchIfRequested() error {
	args := os.Args[1:]
	helper := core.CompatibilityLauncher(args)
	if runtime.GOOS == "windows" && core.IsBastionSSHLaunch(args) {
		helper = "OShell-Bastion"
	}
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
	diagnostics.Exit("external_launch_forwarded", 0)
	os.Exit(0)
	return nil
}

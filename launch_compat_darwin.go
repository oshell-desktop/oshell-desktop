//go:build darwin

package main

import (
	"fmt"
	"os"
	"oshell-desktop/internal/core"
	"oshell-desktop/internal/diagnostics"
	"path/filepath"
	"syscall"
)

func forwardExternalLaunchIfRequested() error {
	args := os.Args[1:]
	helper := core.CompatibilityLauncher(args)
	if helper == "" {
		return nil
	}
	// Validate before handing off, and never echo ticket-bearing arguments.
	if _, err := core.ParseExternalLauncher(helper, args); err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("无法定位应用启动入口")
	}
	main, err := core.LaunchApplicationExecutable(executable)
	if err != nil {
		return err
	}
	path := filepath.Join(filepath.Dir(filepath.Dir(main)), "Resources", helper)
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		return fmt.Errorf("连接助手缺失，请使用完整 OShell Desktop 应用包")
	}
	diagnostics.Exit("external_launch_exec", 0)
	if err := syscall.Exec(path, append([]string{path}, args...), os.Environ()); err != nil {
		return fmt.Errorf("无法启动连接助手，请检查应用包")
	}
	return nil
}

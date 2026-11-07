//go:build !windows

package diagnostics

import "os/exec"

func hideMonitor(*exec.Cmd)                   {}
func processStats() map[string]any            { return nil }
func observeProcess(int) (func() any, func()) { return func() any { return nil }, func() {} }

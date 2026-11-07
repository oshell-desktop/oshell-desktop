package main

import (
	"cmp"
	"context"
	"fmt"
	"github.com/egoist/mygo"
	"os"
	"oshell-desktop/internal/bundled"
	"oshell-desktop/internal/core"
	"oshell-desktop/internal/desktop"
	"oshell-desktop/internal/diagnostics"
	"oshell-desktop/internal/terminal"
	"path/filepath"
)

func main() {
	// SsoTool deletes its temporary session five seconds after starting us.
	// Capture it before diagnostics, updates or the native UI can delay the read.
	if core.IsSSOToolLaunch(os.Args[1:]) {
		if err := forwardExternalLaunchIfRequested(); err != nil {
			showStartupError(err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "--rsync-stdio" {
		if err := core.RunRsyncStdio(context.Background(), os.Args[2:], os.Stdin, os.Stdout, os.Stderr); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		return
	}
	if core.MCPStdioRequested(os.Args) {
		root, token := os.Getenv("OSHELL_AGENT_ROOT"), os.Getenv("OSHELL_AGENT_TOKEN")
		if root == "" || token == "" {
			fmt.Fprintln(os.Stderr, "请从 OShell MCP 控制中心复制客户端接入配置")
			os.Exit(2)
		}
		bridge := core.NewMCPBridge(root, token)
		err := core.ServeMCPStdio(context.Background(), os.Stdin, os.Stdout, cmp.Or(mygo.App.Version(), "dev"), bridge.Call)
		bridge.Close()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		return
	}
	if diagnostics.HandleMonitor() {
		return
	}
	exe, _ := os.Executable()
	base := os.Getenv("OSHELL_DATA_DIR")
	_, marker := os.Stat(filepath.Join(filepath.Dir(exe), core.PortableMarker))
	if bundled.Enabled() || marker == nil {
		base = filepath.Dir(exe)
	}
	if base == "" {
		base, _ = os.UserConfigDir()
		base = filepath.Join(base, "oshell-desktop")
	}
	recorder, err := diagnostics.Start(filepath.Join(base, "logs", "diagnostics"), mygo.App.Version())
	if err != nil {
		fallback, _ := os.UserCacheDir()
		recorder, err = diagnostics.Start(filepath.Join(fallback, "oshell-desktop", "diagnostics"), mygo.App.Version())
		if err == nil {
			diagnostics.Event("log_location_fallback", nil)
		}
	}
	code, reason := runApplication()
	if recorder != nil {
		recorder.Close(reason, code)
	}
	if code != 0 {
		os.Exit(code)
	}
}

func startupFailure(stage string, err error) (int, string) {
	diagnostics.Failure(stage, err)
	showStartupError(err)
	return 1, stage
}

func runApplication() (int, string) {
	diagnostics.Event("startup_stage", map[string]any{"stage": "portable_update"})
	if handled, err := core.HandlePortableUpdate(); err != nil {
		return startupFailure("portable_update", err)
	} else if handled {
		return 0, "portable_update_helper"
	}
	if err := forwardExternalLaunchIfRequested(); err != nil {
		return startupFailure("external_launch", err)
	}
	if bundled.Enabled() {
		diagnostics.Event("startup_stage", map[string]any{"stage": "native_terminal_load"})
		if err := terminal.Load(); err != nil {
			return startupFailure("native_terminal_load", err)
		}
	}
	if err := desktop.Run(); err != nil {
		return startupFailure("desktop_run", err)
	}
	return 0, "desktop_run_returned"
}

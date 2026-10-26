//go:build windows || linux

package core

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func LauncherConfigurationDirectory() (string, error) {
	dir, _, err := defaultStorageDirectory()
	return dir, err
}
func ParseExternalLauncher(name string, args []string) (ExternalLaunchRequest, error) {
	if strings.Contains(strings.ToLower(filepath.Base(name)), "filezilla") {
		r, err := ParseFileLaunch(args, ReadFileZillaSites)
		return ExternalLaunchRequest{File: &r}, err
	}
	r, err := ParseZOCLaunch(args)
	return ExternalLaunchRequest{Terminal: &r}, err
}
func LaunchApplicationAndSend(ctx context.Context, configuration, executable string, request ExternalLaunchRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := request.Validate(); err != nil {
		return err
	}
	release, err := lockLaunchStart(ctx, configuration)
	if err != nil {
		return err
	}
	defer release()
	available := func() bool {
		probe, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
		defer cancel()
		conn, e := dialLaunch(probe, configuration)
		if e != nil {
			return false
		}
		defer conn.Close()
		return verifyLaunchConnection(conn, true) == nil
	}
	if !available() {
		if err := ctx.Err(); err != nil {
			return err
		}
		null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
		if err != nil {
			return err
		}
		defer null.Close()
		command := exec.Command(executable, "--external-launch-service")
		prepareCommand(command)
		command.Stdin = null
		command.Stdout = null
		command.Stderr = null
		if err = command.Start(); err != nil {
			return fmt.Errorf("无法启动 OShell Desktop")
		}
		ended := make(chan struct{})
		go func() { _ = command.Wait(); close(ended) }()
		for !available() {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ended:
				return fmt.Errorf("应用已退出，启动通道未就绪")
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
	return SendExternalLaunch(ctx, configuration, request)
}

// SPDX-License-Identifier: GPL-3.0-only
package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type SSHIdentity struct {
	Host string `json:"host"`
	User string `json:"user"`
	Port int    `json:"port"`
}

func (p Profile) Identity() SSHIdentity {
	return SSHIdentity{strings.Trim(p.Host, "[]"), p.Username, p.Port}
}

type ResolvedSSH struct {
	Profile      Profile
	ProxyCommand string
	OriginalHost string
}

func ResolveSSH(ctx context.Context, p Profile) (ResolvedSSH, error) {
	result := ResolvedSSH{Profile: p, OriginalHost: p.Host}
	if p.Kind != "ssh" && p.Kind != "sftp" {
		return result, nil
	}
	binary, e := exec.LookPath("ssh")
	if e != nil && (!errors.Is(e, exec.ErrNotFound) || !literalSSHWithoutConfig(p.Host)) {
		return result, fmt.Errorf("无法解析 SSH 配置：未找到 ssh；不能绕过主机别名或配置路由")
	}
	if e == nil {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		args := []string{"-G"}
		if p.Port > 0 {
			args = append(args, "-p", strconv.Itoa(p.Port))
		}
		if p.Username != "" {
			args = append(args, "-l", p.Username)
		}
		if p.IdentityFile != "" {
			args = append(args, "-i", expandHome(p.IdentityFile))
		}
		if os.Getenv("OSHELL_TEST_NO_AGENT") == "1" || os.Getenv("OSHELL_TEST_IGNORE_SSH_CONFIG") == "1" {
			// OpenSSH's sentinel works with both native Windows and MSYS/Git
			// builds; the latter interprets Windows' NUL as a missing filename.
			args = append(args, "-F", "none")
		}
		args = append(args, "--", p.Host)
		command := exec.CommandContext(ctx, binary, args...)
		prepareCommand(command)
		var out limitedOutput
		command.Stdout = &out
		if e = command.Run(); e == nil {
			applyResolvedSSH(&result, out.String())
		} else if ctx.Err() != nil {
			return result, ctx.Err()
		} else {
			return result, fmt.Errorf("无法解析 SSH 配置（ssh -G 失败）")
		}
	}
	if result.Profile.Port == 0 {
		result.Profile.Port = 22
	}
	if result.Profile.Username == "" {
		if u, e := user.Current(); e == nil {
			result.Profile.Username = u.Username
			if i := strings.LastIndex(result.Profile.Username, `\`); i >= 0 {
				result.Profile.Username = result.Profile.Username[i+1:]
			}
		}
	}
	if !validHost(result.Profile.Host) || result.Profile.Username == "" {
		return result, fmt.Errorf("无法解析 SSH 主机或用户名")
	}
	return result, nil
}

// Without ssh -G, only a literal IP with no standard SSH configuration can
// safely use the profile directly. Even Host * can route literal addresses.
func literalSSHWithoutConfig(host string) bool {
	if net.ParseIP(strings.Trim(host, "[]")) == nil {
		return false
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	global := "/etc/ssh/ssh_config"
	if runtime.GOOS == "windows" {
		base := os.Getenv("ProgramData")
		if base == "" {
			return false
		}
		global = filepath.Join(base, "ssh", "ssh_config")
	}
	for _, path := range []string{filepath.Join(home, ".ssh", "config"), global} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			return false
		}
	}
	return true
}

type limitedOutput struct{ bytes.Buffer }

func (b *limitedOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 1<<20 {
		return 0, fmt.Errorf("SSH 配置输出过大")
	}
	return b.Buffer.Write(p)
}
func applyResolvedSSH(result *ResolvedSSH, output string) {
	p := &result.Profile
	p.resolvedIdentityFiles = nil
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch key {
		case "hostname":
			p.Host = value
		case "user":
			p.Username = value
		case "port":
			if port, e := strconv.Atoi(value); e == nil {
				p.Port = port
			}
		case "identityfile":
			if value != "none" {
				candidate := expandHome(value)
				if _, e := os.Stat(candidate); e == nil {
					p.resolvedIdentityFiles = append(p.resolvedIdentityFiles, candidate)
				}
			}
		case "proxyjump":
			if p.ProxyID == "" && p.JumpHost == "" && result.ProxyCommand == "" && (p.Proxy.Kind == "" || p.Proxy.Kind == "none") && value != "none" {
				p.JumpHost = value
			}
		case "proxycommand":
			if p.ProxyID == "" && p.JumpHost == "" && (p.Proxy.Kind == "" || p.Proxy.Kind == "none") && value != "none" {
				result.ProxyCommand = value
			}
		}
	}
}

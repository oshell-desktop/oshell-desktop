// SPDX-License-Identifier: GPL-3.0-only
package core

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
)

var LocalTools = []string{"ping", "ping6", "traceroute", "traceroute6", "tracert", "pathping", "mtr", "dig", "nslookup", "host", "dscacheutil", "nc", "ncat", "telnet", "ssh", "netstat", "ss", "lsof", "curl", "openssl", "ifconfig", "ip", "ipconfig", "route", "arp", "ndp", "scutil", "networksetup", "tcpdump", "nmap", "networkQuality", "iperf3", "whois"}

type LocalToolCommand struct {
	Name, Path string
	Arguments  []string
}

func ParseToolCommand(text string) (LocalToolCommand, error) {
	var out LocalToolCommand
	if len(text) > 8192 || strings.ContainsFunc(text, func(r rune) bool { return unicode.IsControl(r) && r != '\t' }) {
		return out, fmt.Errorf("命令过长或包含控制字符")
	}
	var words []string
	var word strings.Builder
	quote := rune(0)
	escaped, started := false, false
	for _, c := range text {
		if escaped {
			if quote == '"' && c != '\\' && c != '"' && c != '$' && c != '`' {
				word.WriteRune('\\')
			}
			word.WriteRune(c)
			escaped = false
			started = true
			continue
		}
		if c == '\\' && quote != '\'' {
			if runtime.GOOS == "windows" {
				// Backslashes are path separators, including relative and UNC
				// paths. They must not silently consume the following character.
				word.WriteRune(c)
			} else {
				escaped = true
			}
			started = true
			continue
		}
		if quote != 0 {
			if c == quote {
				quote = 0
			} else {
				word.WriteRune(c)
			}
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			started = true
			continue
		}
		if c == ' ' || c == '\t' {
			if started {
				words = append(words, word.String())
				word.Reset()
				started = false
			}
			continue
		}
		word.WriteRune(c)
		started = true
	}
	if escaped || quote != 0 {
		return out, fmt.Errorf("引号或反斜杠未结束")
	}
	if started {
		words = append(words, word.String())
	}
	if len(words) == 0 {
		return out, fmt.Errorf("请输入工具名称")
	}
	name := filepath.Base(words[0])
	if strings.EqualFold(filepath.Ext(name), ".exe") {
		name = name[:len(name)-4]
	}
	found := false
	for _, n := range LocalTools {
		if strings.EqualFold(name, n) {
			out.Name = n
			found = true
			break
		}
	}
	if !found {
		return out, fmt.Errorf("不支持的工具；输入 help 查看工具列表")
	}
	executable := words[0]
	if strings.ContainsAny(executable, "/\\") && !filepath.IsAbs(executable) {
		return out, fmt.Errorf("工具路径必须是绝对路径")
	}
	path, e := resolveLocalTool(executable, out.Name)
	if e != nil {
		return out, fmt.Errorf("本机未找到 %s，请先安装该工具", out.Name)
	}
	info, e := os.Stat(path)
	if e != nil || !info.Mode().IsRegular() {
		return out, fmt.Errorf("工具不是普通可执行文件")
	}
	out.Path = path
	out.Arguments = words[1:]
	return out, nil
}
func ToolAvailability() string {
	var lines []string
	for _, n := range LocalTools {
		path, e := resolveLocalTool(n, n)
		if e != nil {
			path = "未安装"
		}
		lines = append(lines, n+"  "+path)
	}
	return strings.Join(lines, "\r\n")
}
func (c LocalToolCommand) ManagedInputAllowed() bool {
	switch strings.ToLower(c.Name) {
	case "ssh", "telnet", "nc", "ncat", "curl", "openssl", "nslookup", "iperf3", "scutil":
		return false
	}
	return true
}

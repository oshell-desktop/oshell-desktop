package core

import (
	"fmt"
	"path"
	"strings"
	"unicode"
)

func AgentCommandDecision(command, permission string) (string, error) {
	if strings.TrimSpace(command) == "" || len(command) > 16384 {
		return "deny", fmt.Errorf("命令为空或超过 16 KiB")
	}
	for _, r := range command {
		if r < 32 && r != '\n' && r != '\t' || r == 127 {
			return "deny", fmt.Errorf("命令包含控制字符")
		}
	}
	words := strings.FieldsFunc(command, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune("_-/.", r)
	})
	danger := map[string]bool{"mkfs": true, "fdisk": true, "diskutil": true, "reboot": true, "shutdown": true, "halt": true, "poweroff": true, "dd": true}
	human := map[string]bool{"sudo": true, "su": true, "ssh": true, "sftp": true, "ftp": true, "vim": true, "vi": true, "nano": true, "less": true, "more": true, "top": true, "htop": true, "passwd": true, "read": true, "tmux": true, "screen": true, "exec": true, "exit": true, "logout": true, "bash": true, "zsh": true, "sh": true, "fish": true, "ksh": true, "csh": true, "tcsh": true}
	interactive := false
	for _, w := range words {
		name := path.Base(w)
		if danger[name] || strings.HasPrefix(name, "mkfs.") || strings.HasPrefix(name, "mkfs_") {
			return "deny", fmt.Errorf("已知危险操作需要人工处理")
		}
		if name == "rm" {
			for _, arg := range words {
				if arg == "/" || strings.HasPrefix(arg, "-") && strings.ContainsAny(arg, "rR") {
					return "deny", fmt.Errorf("递归或根目录删除需要人工处理")
				}
			}
		}
		interactive = interactive || human[name]
	}
	if interactive {
		return "human", fmt.Errorf("交互程序、密码提示或主机上下文变化需要人工接管")
	}
	if permission == "ask" {
		return "approval", nil
	}
	for _, r := range command {
		if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_./- \t", r) {
			return "approval", nil
		}
	}
	args := strings.Fields(command)
	if len(args) == 0 || strings.Contains(args[0], "/") {
		return "approval", nil
	}
	query := map[string]bool{"pwd": true, "whoami": true, "id": true, "uname": true, "uptime": true, "date": true, "ls": true, "cat": true, "head": true, "tail": true, "wc": true, "df": true, "du": true, "ps": true, "hostname": true, "sw_vers": true}
	if query[args[0]] {
		if (args[0] == "date" || args[0] == "hostname") && len(args) > 1 {
			return "approval", nil
		}
		if args[0] == "uname" {
			for _, arg := range args[1:] {
				if !strings.Contains(" -a -s -n -r -v -m -p -i -o ", " "+arg+" ") {
					return "approval", nil
				}
			}
		}
		return "automatic", nil
	}
	if permission == "full" && (args[0] == "mkdir" || args[0] == "touch" || args[0] == "cp" || args[0] == "mv") {
		return "automatic", nil
	}
	return "approval", nil
}
func agentShellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
func agentIntegrationCommand(token string) []byte {
	return []byte("if [ -n \"${BASH_VERSION-}${ZSH_VERSION-}\" ]; then printf '\\033]779;OSHELL;" + token + ";ready\\007'; else printf '\\033]779;OSHELL;" + token + ";unsupported\\007'; fi\n")
}
func agentExecutionCommand(token, id, command string) []byte {
	return []byte("printf '\\033]779;OSHELL;" + token + ";start;" + id + "\\007'; eval -- " + agentShellQuote(command) + "; printf '\\033]779;OSHELL;" + token + ";end;" + id + ";%s\\007' \"$?\"\n")
}

// Keep only a suffix which could be the start of a sensitive value. This
// preserves ordinary output latency and removes secrets split across reads.
type agentRedactor struct {
	values  [][]byte
	pending []byte
}

func (r *agentRedactor) consume(data []byte, final bool) []byte {
	r.pending = append(r.pending, data...)
	out := []byte{}
	for len(r.pending) > 0 {
		matched := 0
		partial := false
		for _, value := range r.values {
			if len(r.pending) >= len(value) && string(r.pending[:len(value)]) == string(value) {
				matched = max(matched, len(value))
			}
			if len(r.pending) < len(value) && string(r.pending) == string(value[:len(r.pending)]) {
				partial = true
			}
		}
		if matched > 0 {
			out = append(out, []byte("[敏感输入]")...)
			r.pending = r.pending[matched:]
			continue
		}
		if partial && !final {
			break
		}
		// At EOF an incomplete sensitive prefix is hidden, never flushed verbatim.
		if partial && final {
			out = append(out, []byte("[敏感输入]")...)
			r.pending = nil
			break
		}
		out = append(out, r.pending[0])
		r.pending = r.pending[1:]
	}
	return out
}
func (r *agentRedactor) text(s string) string {
	for _, v := range r.values {
		s = strings.ReplaceAll(s, string(v), "[敏感输入]")
	}
	return s
}

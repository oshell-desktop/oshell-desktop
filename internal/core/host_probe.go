// SPDX-License-Identifier: GPL-3.0-only
package core

import (
	"fmt"
	"regexp"
	"strings"
)

var authenticationPrompt = regexp.MustCompile(`(?i)(password|passphrase|verification code|\botp\b|验证码|密码|口令)[^\r\n]*[:：?]\s*$`)

func IsAuthenticationPrompt(text string) bool {
	return len(text) <= 2048 && HostnameFromPrompt(text) == "" && authenticationPrompt.MatchString(text)
}
func HostProbeCommand(token string) (string, error) {
	if token == "" || strings.ContainsFunc(token, func(r rune) bool {
		return !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == ':')
	}) {
		return "", fmt.Errorf("invalid probe token")
	}
	script := `h=$(hostname 2>/dev/null); set -- ${SSH_CONNECTION-}; a=${3-}; b=; if [ -z "$a" ]; then b=$(hostname -I 2>/dev/null); [ -n "$b" ] || b=$( (ip -o addr show scope global 2>/dev/null || /sbin/ip -o addr show scope global 2>/dev/null) | awk '{print $4}'); [ -n "$b" ] || b=$(/sbin/ifconfig 2>/dev/null | awk '$1=="inet" || $1=="inet6" {print $2}'); fi; b=$(printf '%s' "$b" | tr '\n' ' '); printf '\033]2;` + token + `%s|%s|%s\007' "$h" "$a" "$b"`
	return " sh -c '" + strings.ReplaceAll(script, "'", "'\\''") + "'", nil
}

// HostProbeEcho suppresses only a verified echo of our own complete command.
// Unknown output, mode changes and partial echoes are replayed unchanged.
type HostProbeEcho struct {
	shellEditing, matched, afterCR                  bool
	csiParameters, preservedModes                   []byte
	expected, pending                               []byte
	index                                           int
	escape, csi, pendingSpace, awaitingLF, finished bool
}

func NewHostProbeEcho(command string) *HostProbeEcho {
	return &HostProbeEcho{expected: []byte(command)}
}
func (h *HostProbeEcho) Flush() []byte {
	h.finished = true
	data := h.pending
	h.pending = nil
	return data
}
func (h *HostProbeEcho) Consume(data []byte) []byte {
	if h.finished {
		return data
	}
	out := make([]byte, 0, len(data))
	reject := func() { h.finished = true; out = append(out, h.pending...); h.pending = nil }
	for _, b := range data {
		if h.awaitingLF {
			h.awaitingLF = false
			h.finished = true
			if b == '\n' {
				continue
			}
		}
		if h.finished {
			out = append(out, b)
			continue
		}
		h.pending = append(h.pending, b)
		if len(h.pending) > 8192 {
			reject()
			continue
		}
		paddingWrap := h.pendingSpace && (b == 13 || b == 10)
		if h.pendingSpace {
			h.pendingSpace = false
			if b != '\r' && b != '\n' {
				if h.index >= len(h.expected) || h.expected[h.index] != ' ' {
					reject()
					continue
				}
				h.index++
			}
		}
		if b == ' ' && !h.escape && !h.csi {
			h.pendingSpace = true
			continue
		}
		if h.escape {
			h.escape = false
			if b == '[' {
				h.csi = true
				h.csiParameters = nil
				continue
			}
			reject()
			continue
		}
		if h.csi {
			if b >= 0x40 && b <= 0x7e {
				h.csi = false
				if h.shellEditing && (b == 'h' || b == 'l') && string(h.csiParameters) == "?2004" {
					h.preservedModes = append(h.preservedModes, 27, '[')
					h.preservedModes = append(h.preservedModes, h.csiParameters...)
					h.preservedModes = append(h.preservedModes, b)
					continue
				}
				if !strings.ContainsRune("ABCDGHJKfm", rune(b)) {
					reject()
				}
			} else {
				h.csiParameters = append(h.csiParameters, b)
			}
			continue
		}
		if b == 27 {
			h.escape = true
			continue
		}
		if h.shellEditing && b == 8 && h.index > 0 {
			h.index--
			h.afterCR = false
			continue
		}
		if h.index == len(h.expected) {
			if b == '\r' || b == '\n' {
				h.matched = true
				out = append(out, h.preservedModes...)
				out = append(out, '\r', 27, '[', '2', 'K')
				h.awaitingLF = b == '\r'
				h.finished = !h.awaitingLF
				h.pending = nil
			} else {
				reject()
			}
		} else if h.shellEditing && h.afterCR && h.index > 0 && b == h.expected[h.index-1] {
			h.afterCR = false
		} else if b == h.expected[h.index] {
			h.index++
			h.afterCR = false
		} else if b == 13 {
			h.afterCR = !paddingWrap
		} else if b != '\r' && b != '\n' {
			reject()
		}
	}
	return out
}

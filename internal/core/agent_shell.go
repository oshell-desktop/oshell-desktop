// SPDX-License-Identifier: GPL-3.0-only
package core

import (
	_ "embed"
	"encoding/base64"
	"strings"
)

//go:embed agent_integration.sh
var agentIntegrationScript string

func agentIntegrationCommand(token string) []byte {
	// The loader restores tty settings on every exit. Frames are shell no-ops
	// if they arrive after cancellation and never enter interactive history.
	loader := `s=$(command stty -g) || exit; trap 'command stty "$s"' EXIT; trap 'exit 130' HUP INT TERM; command stty -echo || exit; p=; i=0; while [ "$i" -lt 64 ]; do builtin printf '\033]779;OSHELL;` + token + `;load;%s\007' "$i" >&2; IFS= builtin read -r -t 5 l || exit; if [ "$l" = ` + agentShellQuote(": OSHELL_END_"+token) + ` ]; then d=$(builtin printf %s "$p" | command base64 -d) || exit; builtin printf %s "$d"; exit; fi; [ "${l:0:2}" = ': ' ] || exit 1; p=$p${l:2}; i=$((i+1)); done; exit 1`
	return []byte(`if [ "${__oshell_mcp_version-}" = 2 ] && typeset -f __oshell_mcp_attach >/dev/null; then __oshell_mcp_attach ` + agentShellQuote(token) + `; else eval -- "$( ` + loader + ` )"; fi` + "\n")
}

func agentIntegrationFrames(token string) [][]byte {
	script := strings.ReplaceAll(strings.TrimSpace(strings.ReplaceAll(agentIntegrationScript, "\r\n", "\n")), "@TOKEN@", agentShellQuote(token))
	script = strings.ReplaceAll(script, "\n", " ")
	encoded := base64.StdEncoding.EncodeToString([]byte(script))
	var frames [][]byte
	for len(encoded) > 0 {
		n := min(len(encoded), 384)
		frames = append(frames, []byte(": "+encoded[:n]+"\n"))
		encoded = encoded[n:]
	}
	return append(frames, []byte(": OSHELL_END_"+token+"\n"))
}

func agentExecutionCommand(_, _, command string) []byte {
	if strings.Contains(command, "\n") {
		return []byte("{\n" + command + "\n}\n")
	}
	return []byte(command + "\n")
}

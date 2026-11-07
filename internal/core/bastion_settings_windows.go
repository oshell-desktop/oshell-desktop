package core

import (
	"golang.org/x/sys/windows/registry"
	"strings"
)

// USM writes only display options here; the actual SSH destination and ticket
// always come from the launch request.
func applyUSMPuTTYSettings(r *TerminalLaunchRequest) {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\SimonTatham\PuTTY\Sessions\USMSSO%20Default`, registry.QUERY_VALUE)
	if err != nil {
		return
	}
	defer key.Close()
	if term, _, err := key.GetStringValue("TerminalType"); err == nil {
		switch term = strings.ToLower(term); term {
		case "vt100", "xterm", "linux":
			r.TerminalType = term
		}
	}
	if title, _, err := key.GetStringValue("WinTitle"); err == nil && r.Title == "" {
		r.Title = title
	}
}

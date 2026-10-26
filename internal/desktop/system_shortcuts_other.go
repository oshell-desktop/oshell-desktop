//go:build !windows

package desktop

import (
	"context"
	"os/exec"
	"oshell-desktop/internal/core"
	"runtime"
	"sort"
	"strings"
	"time"
)

func probeSystemShortcut(key core.KeyboardShortcut) systemShortcutResult {
	if runtime.GOOS != "linux" {
		return systemShortcutResult{}
	}
	// GNOME exposes its configured window-manager shortcuts via GSettings.
	// Other desktops/read failures remain explicitly unknown.
	name := core.ShortcutKeyNames[key.KeyCode]
	mods := key.Modifiers
	accelerator := ""
	if mods&(1|4) != 0 {
		accelerator += "<Control>"
	}
	if mods&2 != 0 {
		accelerator += "<Alt>"
	}
	if mods&8 != 0 {
		accelerator += "<Shift>"
	}
	accelerator += name
	for _, setting := range []string{"close", "switch-windows", "switch-applications", "switch-panels", "show-desktop", "switch-input-source", "switch-to-workspace-left", "switch-to-workspace-right"} {
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		output, err := exec.CommandContext(ctx, "gsettings", "get", "org.gnome.desktop.wm.keybindings", setting).Output()
		cancel()
		if err != nil {
			return systemShortcutResult{}
		}
		bindings := strings.ReplaceAll(strings.ToLower(string(output)), "<primary>", "<control>")
		for index, binding := range strings.Split(bindings, "'") {
			if index%2 == 1 && normalizeSystemAccelerator(binding) == normalizeSystemAccelerator(accelerator) {
				return systemShortcutResult{setting, true}
			}
		}
	}
	return systemShortcutResult{readable: true}
}

func normalizeSystemAccelerator(value string) string {
	value = strings.ReplaceAll(strings.ToLower(value), "<primary>", "<control>")
	var modifiers []string
	for strings.HasPrefix(value, "<") {
		end := strings.IndexByte(value, '>')
		if end < 0 {
			return value
		}
		modifiers = append(modifiers, value[:end+1])
		value = value[end+1:]
	}
	sort.Strings(modifiers)
	switch value {
	case "return":
		value = "enter"
	case "page_up":
		value = "pageup"
	case "page_down":
		value = "pagedown"
	}
	return strings.Join(modifiers, "") + value
}

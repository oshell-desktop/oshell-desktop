// SPDX-License-Identifier: GPL-3.0-only
package core

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

type KeyboardShortcut struct {
	KeyCode   uint16 `json:"keyCode"`
	Modifiers int    `json:"modifiers"`
}
type ShortcutOverride struct {
	Shortcut *KeyboardShortcut `json:"shortcut"`
}
type KeyboardShortcuts struct {
	Overrides map[string]ShortcutOverride `json:"overrides"`
}
type ShortcutAction struct {
	ID       string             `json:"id"`
	Title    string             `json:"title"`
	Defaults []KeyboardShortcut `json:"defaults"`
}

func TerminalEditingAction(id string) bool {
	return id == "terminalDeleteWord" || id == "terminalLineStart"
}

//go:embed shortcuts-presets.json
var shortcutPresetJSON []byte
var ShortcutActions = func() []ShortcutAction {
	var out []ShortcutAction
	if e := json.Unmarshal(shortcutPresetJSON, &out); e != nil {
		panic(e)
	}
	return out
}()
var ShortcutKeyNames = map[uint16]string{0: "A", 11: "B", 8: "C", 2: "D", 14: "E", 3: "F", 5: "G", 4: "H", 34: "I", 38: "J", 40: "K", 37: "L", 46: "M", 45: "N", 31: "O", 35: "P", 12: "Q", 15: "R", 1: "S", 17: "T", 32: "U", 9: "V", 13: "W", 7: "X", 16: "Y", 6: "Z", 29: "0", 18: "1", 19: "2", 20: "3", 21: "4", 23: "5", 22: "6", 26: "7", 28: "8", 25: "9", 43: ",", 47: ".", 44: "/", 41: ";", 39: "'", 33: "[", 30: "]", 42: "\\", 27: "-", 24: "=", 50: "`", 48: "Tab", 36: "Enter", 49: "Space", 51: "Backspace", 53: "Escape", 123: "Left", 124: "Right", 125: "Down", 126: "Up", 115: "Home", 119: "End", 116: "PageUp", 121: "PageDown", 122: "F1", 120: "F2", 99: "F3", 118: "F4", 96: "F5", 97: "F6", 98: "F7", 100: "F8", 101: "F9", 109: "F10", 103: "F11", 111: "F12", 105: "F13", 107: "F14", 113: "F15", 106: "F16", 64: "F17", 79: "F18", 80: "F19", 90: "F20"}

func (s KeyboardShortcut) Valid() bool {
	name, ok := ShortcutKeyNames[s.KeyCode]
	return ok && s.Modifiers >= 0 && s.Modifiers < 16 && (s.Modifiers&7 != 0 || len(name) > 1 && name[0] == 'F' && name[1] >= '1' && name[1] <= '9')
}

func (s KeyboardShortcut) ValidFor(action, platform string) bool {
	if !TerminalEditingAction(action) {
		return s.Valid()
	}
	// Keep the original Control-letter commands available to the shell.
	n := normalizedShortcut(s, platform)
	name := ShortcutKeyNames[n.KeyCode]
	if n.Modifiers == 4 && len(name) == 1 && name[0] >= 'A' && name[0] <= 'Z' {
		return false
	}
	return s.Valid() || s.KeyCode == 115 && s.Modifiers == 0
}
func (s KeyboardShortcut) Display(platform string) string {
	var parts []string
	if s.Modifiers&4 != 0 {
		parts = append(parts, "Ctrl")
	}
	if s.Modifiers&2 != 0 {
		parts = append(parts, "Alt")
	}
	if s.Modifiers&8 != 0 {
		parts = append(parts, "Shift")
	}
	if s.Modifiers&1 != 0 {
		if platform == "darwin" {
			parts = append(parts, "Cmd")
		} else {
			parts = append(parts, "Ctrl")
		}
	}
	return strings.Join(append(parts, ShortcutKeyNames[s.KeyCode]), "+")
}
func (k KeyboardShortcuts) rawBindings(action, platform string) []KeyboardShortcut {
	if value, ok := k.Overrides[action]; ok {
		if value.Shortcut == nil {
			return nil
		}
		return []KeyboardShortcut{*value.Shortcut}
	}
	for _, a := range ShortcutActions {
		if a.ID == action {
			out := append([]KeyboardShortcut(nil), a.Defaults...)
			if platform != "windows" && TerminalEditingAction(action) {
				out = nil
			}
			if platform != "darwin" && action == "focusFullscreen" {
				out = []KeyboardShortcut{{KeyCode: 103}, {KeyCode: 36, Modifiers: 2}}
			}
			if platform != "darwin" && (action == "copy" || action == "paste" || action == "selectAll") {
				for i := range out {
					out[i].Modifiers |= 8
				}
			}
			if platform != "darwin" && action == "closePane" {
				out = []KeyboardShortcut{{KeyCode: 13, Modifiers: 3}} // Ctrl+Alt+W; Ctrl+W belongs to the terminal.
			}
			if TerminalEditingAction(action) || action == "focusFullscreen" || action == "zoomPane" || action == "nextPane" || action == "previousPane" || action == "quickSwitch" || platform != "darwin" && (action == "selectAll" || action == "closePane") {
				filtered := out[:0]
				for _, binding := range out {
					conflict := false
					for _, override := range k.Overrides {
						if override.Shortcut != nil && normalizedShortcut(*override.Shortcut, platform) == normalizedShortcut(binding, platform) {
							conflict = true
							break
						}
					}
					if !conflict {
						filtered = append(filtered, binding)
					}
				}
				out = filtered
			}
			return out
		}
	}
	return nil
}
func normalizedShortcut(s KeyboardShortcut, platform string) KeyboardShortcut {
	if platform != "darwin" && s.Modifiers&1 != 0 {
		s.Modifiers = s.Modifiers&^1 | 4
	}
	return s
}
func (k KeyboardShortcuts) Conflict(key KeyboardShortcut, except, platform string) string {
	key = normalizedShortcut(key, platform)
	for _, a := range ShortcutActions {
		if a.ID == except {
			continue
		}
		for _, other := range k.rawBindings(a.ID, platform) {
			if normalizedShortcut(other, platform) == key {
				return a.Title
			}
		}
	}
	return ""
}
func (k KeyboardShortcuts) Validate(platform string) error {
	known := map[string]bool{}
	for _, a := range ShortcutActions {
		known[a.ID] = true
		for _, s := range k.rawBindings(a.ID, platform) {
			if !s.ValidFor(a.ID, platform) {
				return fmt.Errorf("%s 的快捷键无效", a.Title)
			}
			if other := k.Conflict(s, a.ID, platform); other != "" {
				return fmt.Errorf("%s 与 %s 的快捷键冲突", a.Title, other)
			}
		}
	}
	for key := range k.Overrides {
		if !known[key] {
			return fmt.Errorf("未知快捷键操作：%s", key)
		}
	}
	return nil
}
func (k KeyboardShortcuts) Clone() KeyboardShortcuts {
	if k.Overrides == nil {
		return KeyboardShortcuts{}
	}
	out := KeyboardShortcuts{Overrides: map[string]ShortcutOverride{}}
	for id, v := range k.Overrides {
		if v.Shortcut != nil {
			s := *v.Shortcut
			v.Shortcut = &s
		}
		out.Overrides[id] = v
	}
	return out
}

// Stored shared bindings are platform-independent. Platform collisions may
// prevent execution but must never prevent reading or syncing the configuration.
func (k KeyboardShortcuts) ValidateStored() error {
	data, err := json.Marshal(k)
	if err != nil {
		return err
	}
	var value any
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	if err = decoder.Decode(&value); err != nil {
		return err
	}
	_, err = canonicalSharedValue(value, sharedField{Type: "object", Ref: "shortcuts"}, "configuration.preferences.keyboardShortcuts")
	return err
}
func (k KeyboardShortcuts) Bindings(action, platform string) []KeyboardShortcut {
	raw := k.rawBindings(action, platform)
	if len(k.Overrides) == 0 {
		return raw
	}
	var bindings []KeyboardShortcut
	_, custom := k.Overrides[action]
	for _, key := range raw {
		if !key.ValidFor(action, platform) {
			continue
		}
		conflict := false
		if custom {
			conflict = k.Conflict(key, action, platform) != ""
		} else {
			normalized := normalizedShortcut(key, platform)
			for other, override := range k.Overrides {
				if other != action && override.Shortcut != nil && normalizedShortcut(*override.Shortcut, platform) == normalized {
					conflict = true
					break
				}
			}
		}
		if !conflict {
			bindings = append(bindings, key)
		}
	}
	return bindings
}

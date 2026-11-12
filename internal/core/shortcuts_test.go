package core

import "testing"

func TestKeyboardBindingsAllPlatforms(t *testing.T) {
	for _, platform := range []string{"darwin", "windows", "linux"} {
		k := KeyboardShortcuts{}
		if e := k.Validate(platform); e != nil {
			t.Fatal(platform, e)
		}
		k.Overrides = map[string]ShortcutOverride{"newWindow": {Shortcut: &KeyboardShortcut{KeyCode: 3, Modifiers: 1}}}
		if platform != "darwin" {
			k.Overrides["newWindow"] = ShortcutOverride{Shortcut: &KeyboardShortcut{KeyCode: 3, Modifiers: 1}}
		}
		if e := k.Validate(platform); e == nil {
			t.Fatal("collision accepted", platform)
		}
	}
}

func TestNewDefaultsPreserveExistingOverrides(t *testing.T) {
	key := KeyboardShortcut{KeyCode: 31, Modifiers: 3}
	k := KeyboardShortcuts{Overrides: map[string]ShortcutOverride{"newWindow": {Shortcut: &key}}}
	if e := k.Validate("darwin"); e != nil {
		t.Fatal(e)
	}
	if len(k.Bindings("quickSwitch", "darwin")) != 0 || len(k.Bindings("newWindow", "darwin")) != 1 {
		t.Fatal("new shortcut stole existing binding")
	}
}

func TestWindowsTerminalEditingDefaults(t *testing.T) {
	k := KeyboardShortcuts{}
	for _, platform := range []string{"windows", "linux"} {
		keys := k.Bindings("focusFullscreen", platform)
		if len(keys) != 2 || keys[0] != (KeyboardShortcut{KeyCode: 103}) || keys[1] != (KeyboardShortcut{KeyCode: 36, Modifiers: 2}) {
			t.Fatalf("%s focus mode bindings: %v", platform, keys)
		}
	}
	for action, want := range map[string]KeyboardShortcut{
		"selectAll":          {KeyCode: 0, Modifiers: 9},
		"closePane":          {KeyCode: 13, Modifiers: 3},
		"closeTab":           {KeyCode: 13, Modifiers: 9},
		"terminalDeleteWord": {KeyCode: 51, Modifiers: 4},
		"terminalLineStart":  {KeyCode: 115},
	} {
		got := k.Bindings(action, "windows")
		if len(got) != 1 || got[0] != want {
			t.Fatalf("%s: %v", action, got)
		}
	}
	if err := k.Validate("windows"); err != nil {
		t.Fatal(err)
	}
	custom := KeyboardShortcut{KeyCode: 0, Modifiers: 9}
	k.Overrides = map[string]ShortcutOverride{"newSession": {Shortcut: &custom}}
	if err := k.Validate("windows"); err != nil {
		t.Fatal(err)
	}
	if len(k.Bindings("selectAll", "windows")) != 0 {
		t.Fatal("new default stole an existing override")
	}
}

func TestTerminalAliasDefaultsPreserveUserOverrides(t *testing.T) {
	key := KeyboardShortcut{KeyCode: 51, Modifiers: 4}
	k := KeyboardShortcuts{Overrides: map[string]ShortcutOverride{"newSession": {Shortcut: &key}}}
	if err := k.Validate("windows"); err != nil {
		t.Fatal(err)
	}
	if len(k.Bindings("terminalDeleteWord", "windows")) != 0 {
		t.Fatal("terminal default stole user shortcut")
	}
	if (KeyboardShortcut{KeyCode: 0, Modifiers: 4}).ValidFor("terminalDeleteWord", "windows") {
		t.Fatal("alias can replace original Ctrl+A")
	}
	if (KeyboardShortcut{KeyCode: 115}).ValidFor("newSession", "windows") {
		t.Fatal("unmodified Home accepted for a global action")
	}
}

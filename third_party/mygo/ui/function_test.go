package ui

import (
	"fmt"
	"testing"
)

func TestExtendedFunctionKeyNamesAndEditorShortcuts(t *testing.T) {
	for n := 13; n <= 35; n++ {
		key := KeyF13 + Key(n-13)
		if got := keyName(key); got != fmt.Sprintf("F%d", n) {
			t.Fatalf("F%d name %q", n, got)
		}
		for _, mods := range []Modifiers{0, Shift, Ctrl, Alt, Super} {
			if (&editor{}).wants(keyEvent{key: key, mods: mods}) {
				t.Fatalf("editor swallowed F%d modifiers %d", n, mods)
			}
		}
	}
}

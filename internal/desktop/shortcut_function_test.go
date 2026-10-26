package desktop

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"testing"
)

func TestExtendedFunctionShortcutRoundTrip(t *testing.T) {
	// These are the existing shared/native shortcut identities. No new
	// serialized key codes are introduced by extending platform input.
	for i, code := range []uint16{105, 107, 113, 106, 64, 79, 80, 90} {
		n := i + 13
		key := ui.KeyF13 + ui.Key(i)
		if core.ShortcutKeyNames[code] != fmt.Sprintf("F%d", n) || shortcutKey(code) != key {
			t.Fatalf("F%d shortcut mapping", n)
		}
		binding, ok := shortcutEvent(ui.InputEvent{Kind: ui.InputKeyDown, Key: key, Mods: ui.Shift})
		if !ok || binding.KeyCode != code || shortcutMods(binding) != ui.Shift {
			t.Fatalf("F%d round trip: %+v / %v", n, binding, ok)
		}
	}
}

package desktop

import (
	"oshell-desktop/internal/core"
	"testing"
)

func TestP2O08WindowsKnownSystemConflict(t *testing.T) {
	var f12 core.KeyboardShortcut
	for code, name := range core.ShortcutKeyNames {
		if name == "F12" {
			f12.KeyCode = code
			break
		}
	}
	result := probeSystemShortcut(f12)
	if !result.readable || result.conflict == "" {
		t.Fatal("Windows F12 reservation not reported", result)
	}
}

package terminal

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// OShell MacTerminalView.keyDown -> kittyFunctionalKey -> sendKittyEvent
// handles physical arrows even when Kitty flags are zero. Its encoder preserves
// the key identity. AppleTerminalView.sendHorizontalKey is a separate helper,
// not the normal desktop key path; mode 1243 must not remap physical keys here.
func TestBidiPhysicalArrowsPreserveProtocolIdentity(t *testing.T) {
	for _, tc := range []struct {
		name, setup, want string
		key               ui.Key
		mods              ui.Modifiers
	}{
		{"normal-left", "", "\x1b[D", ui.KeyLeft, 0},
		{"normal-right", "", "\x1b[C", ui.KeyRight, 0},
		{"application-left", "\x1b[?1h", "\x1bOD", ui.KeyLeft, 0},
		{"application-right", "\x1b[?1h", "\x1bOC", ui.KeyRight, 0},
		{"shift-left", "", "\x1b[1;2D", ui.KeyLeft, ui.Shift},
		{"control-right", "", "\x1b[1;5C", ui.KeyRight, ui.Ctrl},
		{"kitty-left", "\x1b[>3u", "\x1b[D\x1b[1;1:3D", ui.KeyLeft, 0},
		{"kitty-control-right", "\x1b[>3u", "\x1b[1;5C\x1b[1;5:3C", ui.KeyRight, ui.Ctrl},
	} {
		t.Run(tc.name, func(t *testing.T) {
			term, tt, conn := bidiViewFixture(t)
			term.Feed([]byte("אב\x1b[?1243h" + tc.setup))
			tt.Frame()
			tt.Key(tc.mods, tc.key)
			if got := conn.take(len(tc.want)); got != tc.want {
				t.Fatalf("physical key encoded %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBidiKeyIdentitySurvivesModeAndParagraphChanges(t *testing.T) {
	term, tt, conn := bidiViewFixture(t)
	term.Feed([]byte("אב\x1b[?1243h\x1b[>3u"))
	tt.Frame()
	term.v.input(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyLeft})
	// Output arrives while the physical key is held; report-events must still
	// pair repeats and release with the same physical press.
	term.Feed([]byte("\x1b[HA\x1b[?1243l"))
	term.v.input(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyLeft, Repeat: true})
	term.v.input(ui.InputEvent{Kind: ui.InputKeyUp, Key: ui.KeyLeft})
	want := "\x1b[D\x1b[1;1:2D\x1b[1;1:3D"
	if got := conn.take(len(want)); got != want {
		t.Fatalf("press/repeat/release identity %q, want %q", got, want)
	}
}

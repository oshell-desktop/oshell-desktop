package terminal

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestKeypadEventsUseTerminalProtocol(t *testing.T) {
	for _, tc := range []struct {
		name, mode, text, want string
		key                    ui.Key
		mods                   ui.Modifiers
	}{
		{"numeric", "", "1", "1", ui.Key1, 0},
		{"application", "\x1b=", "1", "\x1bOq", ui.Key1, 0},
		{"decimal-locale", "", ",", ",", ui.KeyPeriod, 0},
		{"decimal-application", "\x1b=", ",", "\x1bOn", ui.KeyPeriod, 0},
		{"add", "", "+", "+", ui.KeyAdd, 0},
		{"multiply-application", "\x1b=", "*", "\x1bOj", ui.KeyMultiply, 0},
		{"enter-application", "\x1b=", "", "\x1bOM", ui.KeyEnter, 0},
		{"home-application", "\x1b=", "", "\x1bOw", ui.KeyHome, 0},
		{"begin-application", "\x1b=", "", "\x1bOu", ui.KeyClear, 0},
		{"navigation-not-cursor-mode", "\x1b[?1h", "", "\x1b[D", ui.KeyLeft, 0},
		{"shift-page-not-scroll", "", "", "\x1b[5;2~", ui.KeyPageUp, ui.Shift},
		{"shift-digit-not-symbol", "\x1b=", "", "\x1b1", ui.Key1, ui.Shift | ui.Alt},
		{"control-digit-no-text", "", "", "1", ui.Key1, ui.Ctrl},
		{"control-alt-functional", "\x1b[=8u", "1", "\x1b[57400;7u", ui.Key1, ui.Ctrl | ui.Alt},
		{"kitty-enter", "\x1b[=3u", "", "\x1b[57414u\x1b[57414;1:3u", ui.KeyEnter, 0},
		{"kitty-shift-no-alternate", "\x1b[=30u", "+", "\x1b[57413;2;43u\x1b[57413;2:3u", ui.KeyAdd, ui.Shift},
	} {
		t.Run(tc.name, func(t *testing.T) {
			term, _, conn := bidiViewFixture(t)
			term.Feed([]byte(tc.mode))
			term.v.input(ui.InputEvent{Kind: ui.InputKeyDown, Key: tc.key, Mods: tc.mods, Keypad: true})
			if tc.text != "" {
				term.v.input(ui.InputEvent{Kind: ui.InputText, Text: tc.text})
			}
			term.v.input(ui.InputEvent{Kind: ui.InputKeyUp, Key: tc.key, Mods: tc.mods, Keypad: true})
			term.Send([]byte("END"))
			want := tc.want + "END"
			if got := conn.take(len(want)); got != want {
				t.Fatalf("got %q, want %q", got, want)
			}
		})
	}
}

func TestKeypadMappingAndReplay(t *testing.T) {
	term, _, conn := bidiViewFixture(t)
	term.Feed([]byte("\x1b[=3u"))
	keys := []ui.Key{ui.Key0, ui.Key1, ui.Key2, ui.Key3, ui.Key4, ui.Key5, ui.Key6, ui.Key7, ui.Key8, ui.Key9,
		ui.KeyPeriod, ui.KeySlash, ui.KeyMultiply, ui.KeyMinus, ui.KeyAdd, ui.KeyEnter, ui.KeyEqual, ui.KeyComma,
		ui.KeyLeft, ui.KeyRight, ui.KeyUp, ui.KeyDown, ui.KeyPageUp, ui.KeyPageDown, ui.KeyHome, ui.KeyEnd, ui.KeyInsert, ui.KeyDelete, ui.KeyClear}
	for i, key := range keys {
		term.ReplayUserInput(ui.InputEvent{Kind: ui.InputKeyDown, Key: key, Keypad: true})
		term.ReplayUserInput(ui.InputEvent{Kind: ui.InputKeyUp, Key: key, Keypad: true})
		want := fmt.Sprintf("\x1b[%du\x1b[%d;1:3u", 57399+i, 57399+i)
		if got := conn.take(len(want)); got != want {
			t.Fatalf("key %d: got %q, want %q", key, got, want)
		}
	}
	// Main Return is distinct even with the same logical UI key.
	term.ReplayUserInput(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyEnter})
	term.ReplayUserInput(ui.InputEvent{Kind: ui.InputKeyUp, Key: ui.KeyEnter})
	if got := conn.take(1); got != "\r" {
		t.Fatalf("main Return: %q", got)
	}
}

func TestKeypadAliasKeepsHeldKeyIdentity(t *testing.T) {
	alias := KeyAlias{Key: ui.KeyHome, TargetKey: ui.KeyA, TargetMods: ui.Ctrl}
	v := &view{t: &Terminal{opts: Options{KeyAliases: []KeyAlias{alias}}}}
	pad := v.aliasKeyEvent(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyHome, Keypad: true})
	if pad.Key != ui.KeyA || pad.Keypad {
		t.Fatalf("alias target: %+v", pad)
	}
	v.t.SetKeyAliases(nil)
	plain := v.aliasKeyEvent(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyHome})
	if plain.Key != ui.KeyHome {
		t.Fatalf("main key inherited held keypad alias: %+v", plain)
	}
	v.aliasKeyEvent(ui.InputEvent{Kind: ui.InputKeyUp, Key: ui.KeyHome})
	release := v.aliasKeyEvent(ui.InputEvent{Kind: ui.InputKeyUp, Key: ui.KeyHome, Keypad: true})
	if release.Key != ui.KeyA || release.Keypad || len(v.pressedAliases) != 0 {
		t.Fatalf("held keypad release: %+v", release)
	}
}

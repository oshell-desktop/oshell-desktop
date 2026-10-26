package vt

import (
	"fmt"
	"testing"
)

// Literal expectations from OShell KittyKeyboardEncoder's text/none branches.
// A nil text event and a Unicode key carrying committed text are different.
func TestOShellKeyboardTextModes(t *testing.T) {
	for _, tc := range []struct {
		name       string
		flags      int
		key        Key
		r          rune
		mods       Mods
		action     KeyAction
		text, want string
	}{
		{"events-control", 2, KeyA, 'a', ModCtrl, KeyPress, "", "\x01"},
		{"events-control-repeat", 2, KeyA, 'a', ModCtrl, KeyRepeat, "", "\x01"},
		{"events-control-release", 2, KeyA, 'a', ModCtrl, KeyRelease, "", "\x1b[97;5:3u"},
		{"alternates-alt", 4, KeyE, 'e', ModAlt, KeyPress, "", "\x1be"},
		{"events-control-shift", 2, KeyA, 'a', ModCtrl | ModShift, KeyPress, "", "\x1b[97;6u"},
		{"events-control-shift-repeat", 2, KeyA, 'a', ModCtrl | ModShift, KeyRepeat, "", "\x1b[97;6:2u"},
		{"events-committed-control", 2, KeyA, 'a', ModCtrl, KeyPress, "a", "a"},
		{"disambiguate-control", 1, KeyA, 'a', ModCtrl, KeyPress, "a", "\x1b[97;5u"},
		{"super-commit", 1, KeyA, 'a', ModSuper, KeyPress, "a", "a"},
		{"super-no-commit", 1, KeyA, 'a', ModSuper, KeyPress, "", "\x1b[97;9u"},
		{"commit-release", 3, KeyA, 'a', 0, KeyRelease, "a", ""},
		{"none-commit", 1, KeyUnidentified, 0, 0, KeyPress, "你好", "你好"},
		{"none-all", 8, KeyUnidentified, 0, 0, KeyPress, "你好", "\x1b[0u"},
		{"none-associated", 24, KeyUnidentified, 0, 0, KeyPress, "你好", "\x1b[0;;20320:22909u"},
		{"none-associated-repeat", 26, KeyUnidentified, 0, 0, KeyRepeat, "你", "\x1b[0;1:2;20320u"},
		{"none-associated-release", 26, KeyUnidentified, 0, 0, KeyRelease, "你", "\x1b[0;1:3u"},
		// Keep the four inherited Ghostty expectations tied to OShell's
		// actual text/none branches, including the report-all transitions.
		{"commit-report-all", 31, KeyUnidentified, 0, 0, KeyPress, "û", "\x1b[0;;251u"},
		{"unknown-release-plain", 23, KeyUnidentified, 0, ModShift, KeyRelease, "!", ""},
		{"unknown-release-all", 31, KeyUnidentified, 0, ModShift, KeyRelease, "!", "\x1b[0;2:3u"},
		{"shift-alternates-text", 5, KeyA, 'a', ModShift, KeyPress, "A", "A"},
		{"shift-base-matches-text", 5, KeyA, 'A', ModShift, KeyPress, "A", "A"},
		{"shift-alternates-all", 13, KeyA, 'a', ModShift, KeyPress, "A", "\x1b[97:65;2u"},
		{"all-associated-control", 24, KeyA, 'a', ModCtrl, KeyPress, "a", "\x1b[97;5u"},
		{"all-associated-shift", 24, KeyA, 'a', ModShift, KeyPress, "A", "\x1b[97;2;65u"},
		{"associated-control-filter", 24, KeyA, 'a', 0, KeyPress, "a\u0085b", "\x1b[97;;97:98u"},
		{"enter-commit", 24, KeyEnter, 0, 0, KeyPress, "中文", "中文"},
		{"enter-control-commit", 24, KeyEnter, 0, 0, KeyPress, "a\u0085b", "\x1b[13;;97:98u"},
		{"backspace-preedit", 24, KeyBackspace, 0, 0, KeyPress, "中文", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			term, _ := resetTerminal(t, Effects{})
			term.Write([]byte(fmt.Sprintf("\x1b[=%du", tc.flags)))
			enc, err := NewKeyEncoder()
			if err != nil {
				t.Fatal(err)
			}
			defer enc.Free()
			enc.Sync(term, false)
			got := string(enc.Encode(KeyEvent{Key: tc.key, Unshifted: tc.r, Mods: tc.mods, Action: tc.action, Text: tc.text}))
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestOShellPartialKittyControlMappings(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	enc, err := NewKeyEncoder()
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Free()
	for _, flags := range []int{2, 4, 6, 16, 18, 20, 22} {
		term.Write([]byte(fmt.Sprintf("\x1b[=%du", flags)))
		enc.Sync(term, false)
		mapping := map[rune]byte{' ': 0, '/': 31, '0': 48, '1': 49, '2': 0, '3': 27, '4': 28, '5': 29, '6': 30, '7': 31, '8': 127, '9': 57, '?': 127, '@': 0, '[': 27, '\\': 28, ']': 29, '^': 30, '_': 31, '~': 30, '\u0130': 9, '\u212a': 11}
		for c := 'a'; c <= 'z'; c++ {
			mapping[c] = byte(c - 'a' + 1)
			mapping[c-32] = mapping[c]
		}
		for scalar, value := range mapping {
			for _, alt := range []bool{false, true} {
				mods := ModCtrl | Mods(1<<4) | Mods(1<<5) // locks do not change legacy text
				want := string([]byte{value})
				if alt {
					mods |= ModAlt
					want = "\x1b" + want
				}
				for _, action := range []KeyAction{KeyPress, KeyRepeat} {
					got := string(enc.Encode(KeyEvent{Action: action, Key: KeyUnidentified, Unshifted: scalar, Mods: mods}))
					if got != want {
						t.Fatalf("flags=%d scalar=%U alt=%v action=%d got %q, want %q", flags, scalar, alt, action, got, want)
					}
				}
			}
		}
	}
}

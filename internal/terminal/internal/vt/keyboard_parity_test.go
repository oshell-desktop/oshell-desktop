package vt

import (
	"fmt"
	"testing"
	"unsafe"
)

// Expected sequences follow OShell KittyKeyboardEncoder.encodeFunctionalKey,
// buildCsiWithModifier and usesSs3InLegacy, including partial flag sets. These
// assertions run the real native encoder; they are not Swift execution proof.
func TestOShellKeyboardNavigationFlagMatrix(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	enc, err := NewKeyEncoder()
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Free()
	cases := 0
	for flags := 0; flags < 32; flags++ {
		term.Write([]byte(fmt.Sprintf("\x1b[=%du", flags)))
		var active uint8
		if err := term.get(8, unsafe.Pointer(&active)); err != nil || active != uint8(flags) {
			t.Fatalf("requested flags %d were not applied: %d (%v)", flags, active, err)
		}
		for _, app := range []bool{false, true} {
			term.SetMode(ModeCursorKeys, app)
			enc.Sync(term, false)
			for _, key := range []struct {
				key         Key
				letter      byte
				tilde       int
				legacySS3   bool
				legacyFinal byte
			}{
				{KeyArrowLeft, 'D', 0, app, 'D'}, {KeyArrowRight, 'C', 0, app, 'C'},
				{KeyArrowUp, 'A', 0, app, 'A'}, {KeyArrowDown, 'B', 0, app, 'B'},
				{KeyHome, 'H', 0, app, 'H'}, {KeyEnd, 'F', 0, app, 'F'},
				{KeyF1, 'P', 0, true, 'P'}, {KeyF1 + 1, 'Q', 0, true, 'Q'},
				{KeyF1 + 2, 0, 13, true, 'R'}, {KeyF1 + 3, 'S', 0, true, 'S'},
				{KeyInsert, 0, 2, false, 0}, {KeyDelete, 0, 3, false, 0},
				{KeyPageUp, 0, 5, false, 0}, {KeyPageDown, 0, 6, false, 0},
			} {
				for _, mods := range []Mods{0, ModShift, ModCtrl, ModAlt | ModCtrl} {
					// Legacy function-key modifiers use OShell's separate capability
					// table; this matrix covers the Kitty functional-key branch.
					if flags == 0 && mods != 0 && key.key >= KeyF1 {
						continue
					}
					for _, action := range []KeyAction{KeyPress, KeyRepeat, KeyRelease} {
						modifier := 1
						if mods&ModShift != 0 {
							modifier += 1
						}
						if mods&ModAlt != 0 {
							modifier += 2
						}
						if mods&ModCtrl != 0 {
							modifier += 4
						}
						typed := flags&2 != 0 && action != KeyPress
						want := ""
						if action != KeyRelease || flags&2 != 0 {
							final := key.letter
							number := ""
							if key.tilde != 0 {
								number, final = fmt.Sprint(key.tilde), '~'
							}
							ss3 := key.legacySS3 && modifier == 1 && !typed && (flags == 0 || flags&9 == 0 && key.tilde == 0)
							if ss3 {
								want = "\x1bO" + string(key.legacyFinal)
							} else {
								if modifier != 1 || typed {
									if number == "" {
										number = "1"
									}
									number += fmt.Sprintf(";%d", modifier)
									if typed {
										kind := 2
										if action == KeyRelease {
											kind = 3
										}
										number += fmt.Sprintf(":%d", kind)
									}
								}
								want = "\x1b[" + number + string(final)
							}
						}
						got := string(enc.Encode(KeyEvent{Action: action, Key: key.key, Mods: mods}))
						cases++
						if got != want {
							t.Errorf("flags=%d app=%v key=%d mods=%d action=%d: %q, want %q", flags, app, key.key, mods, action, got, want)
						}
					}
				}
			}
		}
	}
	if cases != 10680 {
		t.Fatalf("navigation matrix coverage changed: %d", cases)
	}
}

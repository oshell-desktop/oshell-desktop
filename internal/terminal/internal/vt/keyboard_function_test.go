package vt

import (
	"fmt"
	"testing"
)

// This oracle follows OShell 0.2.92 KittyKeyboardEncoder's function-key
// capability table and CSI builder. It checks the native encoder against
// source-derived expectations, not execution of Swift on this host.
func TestOShellFunctionKeyMatrix(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	enc, err := NewKeyEncoder()
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Free()
	cases, failures := 0, 0
	for flags := 0; flags < 32; flags++ {
		term.Write([]byte(fmt.Sprintf("\x1b[=%du", flags)))
		enc.Sync(term, false)
		for number := 1; number <= 35; number++ {
			for _, mods := range []Mods{0, ModShift, ModAlt, ModShift | ModAlt, ModCtrl, ModCtrl | ModShift, ModCtrl | ModAlt, ModCtrl | ModAlt | ModShift, ModSuper} {
				for _, action := range []KeyAction{KeyPress, KeyRepeat, KeyRelease} {
					want := functionKeyExpected(number, flags, mods, action)
					got := string(enc.Encode(KeyEvent{Key: functionKeyIdentity(number), Mods: mods, Action: action}))
					cases++
					if got != want {
						failures++
						if failures <= 20 {
							t.Errorf("F%d flags=%d mods=%d action=%d: %q, want %q", number, flags, mods, action, got, want)
						}
					}
				}
			}
		}
	}
	if cases != 30240 {
		t.Fatalf("case count %d", cases)
	}
	if failures != 0 {
		t.Fatalf("%d/%d function-key encodings differ", failures, cases)
	}
}

func functionKeyExpected(number, flags int, mods Mods, action KeyAction) string {
	if action == KeyRelease && flags&2 == 0 {
		return ""
	}
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
	if mods&ModSuper != 0 {
		modifier += 8
	}
	if flags == 0 {
		if mods&ModSuper != 0 {
			return ""
		}
		// Higher physical F keys name terminfo capabilities, independent
		// of extra modifiers: F13=Shift+F1, F25=Ctrl+F1, etc.
		if number > 12 {
			modifier = []int{1, 2, 5}[(number-1)/12]
			number = (number-1)%12 + 1
		}
	}
	typed := flags&2 != 0 && action != KeyPress
	if number <= 4 && modifier == 1 && !typed && (flags == 0 || flags&9 == 0 && number != 3) {
		return "\x1bO" + string("PQRS"[number-1])
	}
	var code string
	var final byte
	switch {
	case number > 12:
		code, final = fmt.Sprint(57363+number), 'u'
	case number == 3 && flags != 0:
		code, final = "13", '~'
	case number <= 4:
		final = "PQRS"[number-1]
	default:
		code, final = fmt.Sprint([]int{15, 17, 18, 19, 20, 21, 23, 24}[number-5]), '~'
	}
	if modifier != 1 || typed {
		if code == "" {
			code = "1"
		}
		code += fmt.Sprintf(";%d", modifier)
		if typed {
			event := 2
			if action == KeyRelease {
				event = 3
			}
			code += fmt.Sprintf(":%d", event)
		}
	}
	return "\x1b[" + code + string(final)
}

func functionKeyIdentity(number int) Key {
	if number > 25 {
		return KeyF26 + Key(number-26)
	}
	return KeyF1 + Key(number-1)
}

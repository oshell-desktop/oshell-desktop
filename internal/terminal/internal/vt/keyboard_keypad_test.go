package vt

import (
	"fmt"
	"strconv"
	"testing"
)

// Source oracle: OShell 3f4efd9 KittyKeyboardEncoder's keypad functional
// branches. Functional keypad keys never have alternate character identities.
func TestOShellKeypadModes(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	enc, err := NewKeyEncoder()
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Free()
	type sample struct {
		key          Key
		code         int
		text, normal string
		app          byte
	}
	cases := []sample{
		{KeyNumpadDecimal, 57409, ",", "", 'n'}, {KeyNumpadDivide, 57410, "/", "", 'o'},
		{KeyNumpadMultiply, 57411, "*", "", 'j'}, {KeyNumpadSubtract, 57412, "-", "", 'm'},
		{KeyNumpadAdd, 57413, "+", "", 'k'}, {KeyNumpadEnter, 57414, "", "\r", 'M'},
		{KeyNumpadEqual, 57415, "=", "", 'X'}, {KeyNumpadSeparator, 57416, ",", "", 'l'},
		{KeyNumpadLeft, 57417, "", "D", 't'}, {KeyNumpadRight, 57418, "", "C", 'v'},
		{KeyNumpadUp, 57419, "", "A", 'x'}, {KeyNumpadDown, 57420, "", "B", 'r'},
		{KeyNumpadPageUp, 57421, "", "5~", 'y'}, {KeyNumpadPageDown, 57422, "", "6~", 's'},
		{KeyNumpadHome, 57423, "", "H", 'w'}, {KeyNumpadEnd, 57424, "", "F", 'q'},
		{KeyNumpadInsert, 57425, "", "2~", 'p'}, {KeyNumpadDelete, 57426, "", "3~", 'n'},
		{KeyNumpadBegin, 57427, "", "", 'u'},
	}
	// PageUp is keypad 9, and therefore SS3 y; keypad 3/PageDown is s.
	for i := 0; i < 10; i++ {
		cases = append(cases, sample{KeyNumpad0 + Key(i), 57399 + i, strconv.Itoa(i), "", byte('p' + i)})
	}
	failures, checked := 0, 0
	for flags := 0; flags < 32; flags++ {
		for _, app := range []bool{false, true} {
			mode := "\x1b>"
			if app {
				mode = "\x1b="
			}
			term.Write([]byte(fmt.Sprintf("\x1b[=%du\x1b[?1h%s", flags, mode)))
			enc.Sync(term, false)
			for _, tc := range cases {
				for mods := Mods(0); mods < 8; mods++ {
					for _, action := range []KeyAction{KeyPress, KeyRepeat, KeyRelease} {
						want := ""
						m := int(mods&ModShift) + int(mods&ModAlt)/2 + int(mods&ModCtrl)*2
						if flags == 0 && action != KeyRelease {
							switch {
							case app && mods == 0:
								want = "\x1bO" + string(tc.app)
							case tc.key == KeyNumpadEnter:
								want = "\r"
								if mods&ModAlt != 0 {
									want = "\x1b" + want
								}
							case tc.text != "":
								want = tc.text
								if mods&ModAlt != 0 {
									want = "\x1b" + want
								}
							case tc.normal != "":
								if len(tc.normal) == 1 {
									if m == 0 {
										want = "\x1b[" + tc.normal
									} else {
										want = fmt.Sprintf("\x1b[1;%d%s", m+1, tc.normal)
									}
								} else {
									if m == 0 {
										want = "\x1b[" + tc.normal
									} else {
										want = fmt.Sprintf("\x1b[%s;%d~", tc.normal[:1], m+1)
									}
								}
							}
						} else if flags != 0 && (action != KeyRelease || flags&2 != 0) {
							want = fmt.Sprintf("\x1b[%d", tc.code)
							typed := flags&2 != 0 && action != KeyPress
							if m != 0 || typed {
								want += fmt.Sprintf(";%d", m+1)
							}
							if typed {
								if action == KeyRepeat {
									want += ":2"
								} else {
									want += ":3"
								}
							}
							if flags&24 == 24 && action != KeyRelease && mods&(ModCtrl|ModAlt) == 0 && tc.text != "" {
								if m == 0 && !typed {
									want += ";"
								}
								want += ";" + strconv.Itoa(int([]rune(tc.text)[0]))
							}
							want += "u"
						}
						got := string(enc.Encode(KeyEvent{Key: tc.key, Mods: mods, Action: action, Text: tc.text}))
						checked++
						if got != want {
							failures++
							if failures <= 12 {
								t.Errorf("flags=%d app=%t key=%d mods=%d action=%d: %q, want %q", flags, app, tc.key, mods, action, got, want)
							}
						}
					}
				}
			}
		}
	}
	if failures > 0 {
		t.Fatalf("%d/%d keypad cases differ", failures, checked)
	}
	t.Logf("%d keypad cases passed", checked)
}

package vt

import (
	"fmt"
	"strings"
	"testing"
)

func TestOShellKeyboardExplicitAlternates(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	enc, err := NewKeyEncoder()
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Free()
	samples := []struct {
		code, shift, base rune
		text              string
	}{
		{'a', 'A', 0, "A"}, {' ', ' ', 0, " "}, {'ě', '2', '2', "2"}, {'я', 'Я', 'z', "Я"},
		{'界', 0, 0, "界"}, {'a', 0, 0, "A"}, {'a', 'A', 0, ""}, {'я', 'Я', 'z', ""},
	}
	for flags := 8; flags < 32; flags++ {
		if flags&8 == 0 {
			continue
		}
		term.Write([]byte(fmt.Sprintf("\x1b[=%du", flags)))
		enc.Sync(term, false)
		for _, s := range samples {
			for mods := Mods(0); mods < 8; mods++ {
				for _, action := range []KeyAction{KeyPress, KeyRepeat, KeyRelease} {
					want := ""
					if action != KeyRelease || flags&2 != 0 {
						code := fmt.Sprint(s.code)
						if flags&4 != 0 {
							shift := s.shift
							if mods&ModShift == 0 {
								shift = 0
							}
							if shift != 0 || s.base != 0 {
								code += ":"
								if shift != 0 {
									code += fmt.Sprint(shift)
								}
								if s.base != 0 {
									code += ":" + fmt.Sprint(s.base)
								}
							}
						}
						event := ""
						if flags&2 != 0 && action != KeyPress {
							n := 2
							if action == KeyRelease {
								n = 3
							}
							event = fmt.Sprintf(":%d", n)
						}
						// Native modifier bit order is Shift/Ctrl/Alt; CSI order is Shift/Alt/Ctrl.
						m := 1
						if mods&ModShift != 0 {
							m += 1
						}
						if mods&ModAlt != 0 {
							m += 2
						}
						if mods&ModCtrl != 0 {
							m += 4
						}
						if m != 1 || event != "" {
							code += fmt.Sprintf(";%d%s", m, event)
						}
						if flags&16 != 0 && action != KeyRelease && mods&(ModCtrl|ModAlt) == 0 && s.text != "" {
							var cps []string
							for _, r := range s.text {
								cps = append(cps, fmt.Sprint(r))
							}
							if m == 1 && event == "" {
								code += ";"
							}
							code += ";" + strings.Join(cps, ":")
						}
						want = "\x1b[" + code + "u"
					}
					got := string(enc.Encode(KeyEvent{Key: KeyUnidentified, Unshifted: s.code, Alternates: true, Shifted: s.shift, BaseLayout: s.base, Text: s.text, Mods: mods, Action: action}))
					if got != want {
						t.Fatalf("flags=%d code=%U shift=%U base=%U mods=%d action=%d text=%q: %q want %q", flags, s.code, s.shift, s.base, mods, action, s.text, got, want)
					}
				}
			}
		}
	}
	// Reused native event must forget explicit metadata on the following call.
	term.Write([]byte("\x1b[=12u"))
	enc.Sync(term, false)
	got := string(enc.Encode(KeyEvent{Action: KeyPress, Key: KeyA, Unshifted: 'a', Text: "A", Mods: ModShift}))
	if got != "\x1b[97:65;2u" {
		t.Fatalf("stale metadata: %q", got)
	}
}

func TestOShellKeyboardAlternateMetadataBoundaries(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	enc, err := NewKeyEncoder()
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Free()
	term.Write([]byte("\x1b[=14u"))
	enc.Sync(term, false)
	for _, tc := range []struct {
		event KeyEvent
		want  string
	}{
		{KeyEvent{Action: KeyPress, Key: KeyDigit2, Unshifted: 'ě', Text: "2", Mods: ModShift}, "\x1b[283:50:50;2u"},
		{KeyEvent{Action: KeyPress, Key: KeySpace, Unshifted: ' ', Text: " ", Mods: ModShift}, "\x1b[32:32;2u"},
		{KeyEvent{Action: KeyRelease, Key: KeyA, Unshifted: 'a', Mods: ModShift, Alternates: true, Shifted: 'A'}, "\x1b[97:65;2:3u"},
		{KeyEvent{Action: KeyPress, Key: KeyA, Unshifted: 'a', Text: "A", Mods: ModShift, Alternates: true}, "\x1b[97;2u"},
		{KeyEvent{Action: KeyPress, Key: KeyA, Unshifted: 'a', Mods: ModShift, Alternates: true, Shifted: 0xd800, BaseLayout: 0x110000}, "\x1b[97;2u"},
		{KeyEvent{Action: KeyPress, Key: KeyA, Unshifted: 'a', Alternates: true, Shifted: -1, BaseLayout: -1}, "\x1b[97u"},
		{KeyEvent{Action: KeyPress, Key: KeyEnter, Alternates: true, Shifted: 'A', BaseLayout: 'b'}, "\x1b[13u"},
	} {
		if got := string(enc.Encode(tc.event)); got != tc.want {
			t.Fatalf("%+v: %q want %q", tc.event, got, tc.want)
		}
	}
}

func TestOShellKeyboardShiftedLegacyFallback(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	enc, err := NewKeyEncoder()
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Free()
	for _, flags := range []int{0, 2, 4, 6, 16, 18, 20, 22} {
		term.Write([]byte(fmt.Sprintf("\x1b[=%du", flags)))
		enc.Sync(term, false)
		for _, text := range []string{"", "Я"} {
			ev := KeyEvent{Action: KeyPress, Key: KeyUnidentified, Unshifted: 'я', Alternates: true, Shifted: 'Я', Mods: ModAlt | ModShift, Text: text}
			want := "\x1bЯ"
			// Swift's partial Kitty flags send an existing commit directly before
			// legacy fallback; a nil commit uses the explicit shifted key plus Alt.
			if flags != 0 && text != "" {
				want = "Я"
			}
			if got := string(enc.Encode(ev)); got != want {
				t.Fatalf("flags=%d text=%q: %q want %q", flags, text, got, want)
			}
		}
	}
}

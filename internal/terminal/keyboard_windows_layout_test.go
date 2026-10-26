package terminal

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"strings"
	"testing"
)

func TestDeferredLayoutTextAndReleaseMatrix(t *testing.T) {
	term, _, conn := bidiViewFixture(t)
	for flags := 0; flags < 32; flags++ {
		term.Feed([]byte(fmt.Sprintf("\x1b[=%du", flags)))
		for _, s := range []struct {
			key                   ui.Key
			base, shift, physical rune
			text                  string
			mods                  ui.Modifiers
		}{
			{ui.KeyZ, 'я', 'Я', 'z', "Я", ui.Shift},
			{ui.Key1, '&', '1', '1', "1", ui.Shift},
			{ui.KeyE, 'e', 'E', 'd', "E", ui.Shift},
			{ui.KeySpace, ' ', ' ', 0, " ", ui.Shift},
			{ui.KeyA, 'a', 0, 0, "A", 0},
			{ui.KeyUnknown, '界', 0, 0, "界", 0},
		} {
			var want strings.Builder
			for i, kind := range []ui.InputKind{ui.InputKeyDown, ui.InputKeyDown, ui.InputKeyUp} {
				ev := ui.InputEvent{Kind: kind, Key: s.key, Unshifted: s.base, Shifted: s.shift, BaseLayout: s.physical, Mods: s.mods, TextPending: true, Repeat: i == 1}
				term.ReplayUserInput(ev)
				if i < 2 {
					term.ReplayUserInput(ui.InputEvent{Kind: ui.InputText, Text: s.text})
				}
				if i == 2 && (flags&2 == 0 || flags&8 == 0) {
					continue
				}
				if i < 2 && flags&8 == 0 {
					want.WriteString(s.text)
					continue
				}
				code := fmt.Sprint(s.base)
				if flags&4 != 0 {
					shift := s.shift
					if s.mods&ui.Shift == 0 {
						shift = 0
					}
					if shift != 0 || s.physical != 0 {
						code += ":"
						if shift != 0 {
							code += fmt.Sprint(shift)
						}
						if s.physical != 0 {
							code += ":" + fmt.Sprint(s.physical)
						}
					}
				}
				mod := 1
				if s.mods&ui.Shift != 0 {
					mod = 2
				}
				event := ""
				if flags&2 != 0 && i > 0 {
					event = fmt.Sprintf(":%d", i+1)
				}
				if mod != 1 || event != "" {
					code += fmt.Sprintf(";%d%s", mod, event)
				}
				if i < 2 && flags&24 == 24 {
					if mod == 1 && event == "" {
						code += ";"
					}
					code += ";" + fmt.Sprint([]rune(s.text)[0])
				}
				want.WriteString("\x1b[" + code + "u")
			}
			term.Send([]byte("END"))
			want.WriteString("END")
			if got := conn.take(want.Len()); got != want.String() {
				t.Fatalf("flags=%d base=%U: %q want %q", flags, s.base, got, want.String())
			}
		}
	}
}

func TestDeferredLayoutAltGrAndNoCommit(t *testing.T) {
	term, _, conn := bidiViewFixture(t)
	for flags := 0; flags < 32; flags++ {
		term.Feed([]byte(fmt.Sprintf("\x1b[=%du", flags)))
		ev := ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyE, Unshifted: 'e', BaseLayout: 'd', TextPending: true, Mods: ui.Ctrl | ui.Alt}
		term.ReplayUserInput(ev)
		term.ReplayUserInput(ui.InputEvent{Kind: ui.InputText, Text: "€"})
		ev.Kind = ui.InputKeyUp
		ev.Mods = 0
		term.ReplayUserInput(ev)
		term.Send([]byte("END"))
		if got := conn.take(len("€END")); got != "€END" {
			t.Fatalf("flags %d AltGr: %q", flags, got)
		}
	}
	term.Feed([]byte("\x1b[=31u"))
	ev := ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyE, Unshifted: 'e', BaseLayout: 'd', TextPending: true, Mods: ui.Ctrl | ui.Alt}
	term.ReplayUserInput(ev)
	ev.Kind = ui.InputKeyUp
	term.ReplayUserInput(ev)
	term.Send([]byte("END"))
	want := "\x1b[101::100;7u\x1b[101::100;7:3uEND"
	if got := conn.take(len(want)); got != want {
		t.Fatalf("no commit: %q want %q", got, want)
	}
}

func TestDeferredLayoutUnmappedKeysDoNotShareReleaseState(t *testing.T) {
	term, _, conn := bidiViewFixture(t)
	term.Feed([]byte("\x1b[=31u"))
	first := ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyUnknown, Unshifted: 'ç', TextPending: true, Mods: ui.Ctrl | ui.Alt}
	second := first
	second.Unshifted = 'ģ'
	term.ReplayUserInput(first)
	term.ReplayUserInput(ui.InputEvent{Kind: ui.InputText, Text: "€"})
	term.ReplayUserInput(second)
	first.Kind = ui.InputKeyUp
	term.ReplayUserInput(first)
	second.Kind = ui.InputKeyUp
	term.ReplayUserInput(second)
	term.Send([]byte("END"))
	want := "€\x1b[291;7u\x1b[291;7:3uEND"
	if got := conn.take(len(want)); got != want {
		t.Fatalf("colliding layout keys: %q want %q", got, want)
	}
}

package terminal

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"strings"
	"testing"
)

func TestLayoutPrintableIdentityAndReplay(t *testing.T) {
	term, _, conn := bidiViewFixture(t)
	for _, tc := range []struct {
		key  ui.Key
		base rune
		text string
		mods ui.Modifiers
		code string
	}{
		{ui.KeyA, 'a', "a", 0, "97"}, {ui.KeyA, 'a', "A", ui.Shift, "97:65"},
		{ui.Key1, '1', "!", ui.Shift, "49:33"}, {ui.KeyUnknown, 'я', "Я", ui.Shift, "1103:1071"},
		{ui.KeyUnknown, '界', "界", 0, "30028"},
	} {
		for _, flags := range []int{0, 31} {
			term.Feed([]byte(fmt.Sprintf("\x1b[=%du", flags)))
			for i, kind := range []ui.InputKind{ui.InputKeyDown, ui.InputKeyDown, ui.InputKeyUp} {
				ev := ui.InputEvent{Kind: kind, Key: tc.key, Unshifted: tc.base, Mods: tc.mods, Repeat: i == 1}
				if tc.mods&ui.Shift != 0 {
					ev.Shifted = []rune(tc.text)[0]
				}
				if i < 2 {
					ev.Text = tc.text
				}
				term.ReplayUserInput(ev)
			}
			want := tc.text + tc.text + "END"
			if flags != 0 {
				mod := 1
				if tc.mods == ui.Shift {
					mod = 2
				}
				cp := []rune(tc.text)[0]
				// Release retains layout alternates but has no associated text.
				want = fmt.Sprintf("\x1b[%s;%d;%du\x1b[%s;%d:2;%du\x1b[%s;%d:3uEND", tc.code, mod, cp, tc.code, mod, cp, tc.code, mod)
				if mod == 1 {
					want = strings.Replace(want, ";1;", ";;", 1)
				}
			}
			term.Send([]byte("END"))
			if got := conn.take(len(want)); got != want {
				t.Fatalf("base %U flags %d: %q want %q", tc.base, flags, got, want)
			}
		}
	}
}

func TestLayoutLiteralAndAlias(t *testing.T) {
	term, _, conn := bidiViewFixture(t)
	term.Feed([]byte("\x1b[=31u"))
	term.ReplayUserInput(ui.InputEvent{Kind: ui.InputText, Text: "€", Literal: true})
	term.SetKeyAliases([]KeyAlias{{Key: ui.KeyA, Mods: ui.Ctrl, TargetKey: ui.KeyHome}})
	term.ReplayUserInput(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyA, Mods: ui.Ctrl, Unshifted: 'a', Text: "\x01"})
	term.ReplayUserInput(ui.InputEvent{Kind: ui.InputKeyUp, Key: ui.KeyA, Mods: ui.Ctrl, Unshifted: 'a'})
	want := "€\x1b[H\x1b[1;1:3HEND"
	term.Send([]byte("END"))
	if got := conn.take(len(want)); got != want {
		t.Fatalf("literal/alias: %q want %q", got, want)
	}
}

func TestLayoutMetaWithoutCommit(t *testing.T) {
	term, _, conn := bidiViewFixture(t)
	for _, flags := range []int{0, 2, 4, 6, 16, 18, 20, 22} {
		term.Feed([]byte(fmt.Sprintf("\x1b[=%du", flags)))
		term.ReplayUserInput(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyUnknown, Unshifted: 'я', Shifted: 'Я', Mods: ui.Alt | ui.Shift})
		want := "\x1bЯEND"
		term.Send([]byte("END"))
		if got := conn.take(len(want)); got != want {
			t.Fatalf("flags %d: %q want %q", flags, got, want)
		}
	}
}

func TestLayoutPhysicalAlternateReplay(t *testing.T) {
	term, _, conn := bidiViewFixture(t)
	term.Feed([]byte("\x1b[=31u"))
	for i, kind := range []ui.InputKind{ui.InputKeyDown, ui.InputKeyDown, ui.InputKeyUp} {
		ev := ui.InputEvent{Kind: kind, Key: ui.KeyUnknown, Unshifted: 'я', Shifted: 'Я', BaseLayout: 'q', Mods: ui.Shift, Repeat: i == 1}
		if i < 2 {
			ev.Text = "Я"
		}
		term.ReplayUserInput(ev)
	}
	want := "\x1b[1103:1071:113;2;1071u\x1b[1103:1071:113;2:2;1071u\x1b[1103:1071:113;2:3uEND"
	term.Send([]byte("END"))
	if got := conn.take(len(want)); got != want {
		t.Fatalf("physical alternate: %q, want %q", got, want)
	}
}

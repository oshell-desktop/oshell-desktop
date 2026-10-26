package terminal

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"testing"
)

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

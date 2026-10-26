package terminal

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestInputCancellationClearsDeferredStateWithoutSending(t *testing.T) {
	term, _, conn := bidiViewFixture(t)
	term.Feed([]byte("\x1b[=31u"))
	forwarded := 0
	term.opts.OnUserInput = func(ui.InputEvent) { forwarded++ }
	v := term.v
	v.input(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyA, Unshifted: 'a', Shifted: 'A', TextPending: true, Mods: ui.Shift})
	if v.pending == nil {
		t.Fatal("missing deferred input")
	}
	v.preedit, v.preeditCaret = "旧", 1
	v.pressedAliases = map[keyIdentity]KeyAlias{{key: ui.KeyHome}: {TargetKey: ui.KeyA}}
	v.composedKeys = map[keyIdentity]bool{{key: ui.KeyA}: true}
	before := forwarded
	v.input(ui.InputEvent{Kind: ui.InputCancel})
	if v.pending != nil || v.preedit != "" || v.preeditCaret != 0 || len(v.pressedAliases) != 0 || len(v.composedKeys) != 0 {
		t.Fatal("cancellation retained pending terminal input")
	}
	if forwarded != before {
		t.Fatal("focus cancellation was synchronized as user input")
	}
	term.Send([]byte("END"))
	if got := conn.take(3); got != "END" {
		t.Fatalf("cancellation sent bytes: %q", got)
	}
	v.input(ui.InputEvent{Kind: ui.InputText, Text: "新"})
	term.Send([]byte("END"))
	want := "\x1b[0;;26032uEND"
	if got := conn.take(len(want)); got != want {
		t.Fatalf("new text inherited old physical key: %q want %q", got, want)
	}
}

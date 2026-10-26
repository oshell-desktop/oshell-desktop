package terminal

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/terminal/internal/vt"
	"testing"
)

func TestExtendedFunctionKeyInputAndReplay(t *testing.T) {
	term, _, conn := bidiViewFixture(t)
	term.Feed([]byte("\x1b[=3u"))
	for n := 13; n <= 35; n++ {
		key := ui.KeyF13 + ui.Key(n-13)
		wantKey := vt.KeyF1 + vt.Key(n-1)
		if n >= 26 {
			wantKey = vt.KeyF26 + vt.Key(n-26)
		}
		if got, text := vtKey(key); got != wantKey || text != 0 || typesText(key) {
			t.Fatalf("F%d identity/text: %d/%d", n, got, text)
		}
		for _, replay := range []bool{false, true} {
			for i, kind := range []ui.InputKind{ui.InputKeyDown, ui.InputKeyDown, ui.InputKeyUp} {
				ev := ui.InputEvent{Kind: kind, Key: key, Repeat: i == 1}
				if replay {
					term.ReplayUserInput(ev)
				} else {
					term.v.input(ev)
				}
			}
			code := 57363 + n
			want := fmt.Sprintf("\x1b[%du\x1b[%d;1:2u\x1b[%d;1:3uEND", code, code, code)
			term.Send([]byte("END"))
			if got := conn.take(len(want)); got != want {
				t.Fatalf("F%d replay=%v: %q, want %q", n, replay, got, want)
			}
		}
	}
}

func TestNativeFunctionBridgePreservesBroadcastIdentity(t *testing.T) {
	source, _, left := bidiViewFixture(t)
	receiver, _, right := bidiViewFixture(t)
	source.v.focused = true
	source.Feed([]byte("\x1b[=3u"))
	var forwarded []ui.InputEvent
	source.opts.OnUserInput = func(ev ui.InputEvent) {
		forwarded = append(forwarded, ev)
		receiver.ReplayUserInput(ev)
	}
	for i := range 3 {
		if !source.SendFunctionKey(35, 0, i == 2, i == 1) {
			t.Fatalf("event %d rejected", i)
		}
	}
	source.Send([]byte("END"))
	receiver.Send([]byte("END"))
	want := "\x1b[57398u\x1b[57398;1:2u\x1b[57398;1:3uEND"
	if got := left.take(len(want)); got != want {
		t.Fatalf("source: %q", got)
	}
	want = "\x1b[23;5~\x1b[23;5~END"
	if got := right.take(len(want)); got != want {
		t.Fatalf("receiver protocol/identity: %q", got)
	}
	if len(forwarded) != 3 {
		t.Fatalf("forwarded: %+v", forwarded)
	}
	for i, ev := range forwarded {
		if ev.Key != ui.KeyF35 || ev.Repeat != (i == 1) {
			t.Fatalf("event %d lost metadata: %+v", i, ev)
		}
	}
	for _, n := range []int{12, 36} {
		if source.SendFunctionKey(n, 0, false, false) {
			t.Fatalf("out of range F%d", n)
		}
	}
}

func TestFunctionKeyBroadcastAcrossDifferentProtocols(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, sourceKitty := range []bool{false, true} {
			source, input, left := bidiViewFixture(t)
			receiver, _, right := bidiViewFixture(t)
			source.v.focused = true
			if sourceKitty {
				source.Feed([]byte("\x1b[=3u"))
			} else {
				receiver.Feed([]byte("\x1b[=3u"))
			}
			source.opts.OnUserInput = receiver.ReplayUserInput
			if native {
				source.SendFunctionKey(35, 0, false, false)
				source.SendFunctionKey(35, 0, true, false)
			} else {
				input.Key(0, ui.KeyF35)
			}
			source.Send([]byte("END"))
			receiver.Send([]byte("END"))
			legacy, kitty := "\x1b[23;5~END", "\x1b[57398u\x1b[57398;1:3uEND"
			wantLeft, wantRight := legacy, kitty
			if sourceKitty {
				wantLeft, wantRight = kitty, legacy
			}
			if got := left.take(len(wantLeft)); got != wantLeft {
				t.Fatalf("native=%v sourceKitty=%v source=%q", native, sourceKitty, got)
			}
			if got := right.take(len(wantRight)); got != wantRight {
				t.Fatalf("native=%v sourceKitty=%v receiver=%q", native, sourceKitty, got)
			}
		}
	}
}

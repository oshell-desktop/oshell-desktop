package gdkinput

import (
	"github.com/egoist/mygo/internal/platform"
	"reflect"
	"testing"
)

type functionalHarness struct {
	keys   FunctionalKeys
	events []platform.SurfaceEvent
	reject bool
}

func (h *functionalHarness) send(ev platform.SurfaceEvent) bool {
	if h.keys.Capture(ev) {
		return true
	}
	h.events = append(h.events, ev)
	return !h.reject
}

func (h *functionalHarness) handle(kind platform.SurfaceEventKind, symbol uint32, filter func() bool) bool {
	return h.keys.Handle(kind, symbol, 38, platform.ModCtrl, h.send, filter)
}

func TestFunctionalKeysPressRepeatRelease(t *testing.T) {
	cases := map[uint32]platform.Key{
		0xff0d: platform.KeyEnter, 0xff1b: platform.KeyEscape, 0xff08: platform.KeyBackspace,
		0xff09: platform.KeyTab, 0xfe20: platform.KeyTab, 0xffff: platform.KeyDelete,
		0xff63: platform.KeyInsert, 0xff50: platform.KeyHome, 0xff57: platform.KeyEnd,
		0xff55: platform.KeyPageUp, 0xff56: platform.KeyPageDown, 0xff51: platform.KeyLeft,
		0xff52: platform.KeyUp, 0xff53: platform.KeyRight, 0xff54: platform.KeyDown, 0xff67: platform.KeyContextMenu,
	}
	for n := 1; n <= 35; n++ {
		key := platform.KeyF1 + platform.Key(n-1)
		if n > 12 {
			key = platform.KeyF13 + platform.Key(n-13)
		}
		cases[0xffbd+uint32(n)] = key
	}
	for symbol, want := range cases {
		h := &functionalHarness{}
		filters := 0
		filter := func() bool { filters++; return false }
		if !h.handle(platform.KeyPressed, symbol, filter) || !h.handle(platform.KeyPressed, symbol, filter) || !h.handle(platform.KeyReleased, symbol, filter) {
			t.Fatalf("symbol %x not handled", symbol)
		}
		if filters != 3 || len(h.events) != 3 {
			t.Fatalf("symbol %x: filters %d events %+v", symbol, filters, h.events)
		}
		for i, ev := range h.events {
			kind := platform.KeyPressed
			if i == 2 {
				kind = platform.KeyReleased
			}
			if ev.Kind != kind || ev.Key != want || ev.Repeat != (i == 1) || ev.Mods != platform.ModCtrl || ev.Keypad {
				t.Fatalf("symbol %x event %d: %+v", symbol, i, ev)
			}
		}
	}
	if len(cases) != 51 {
		t.Fatalf("coverage %d", len(cases))
	}
	var h functionalHarness
	for _, symbol := range []uint32{'a', ' ', 0xffb1, 0xffe1} {
		if h.handle(platform.KeyPressed, symbol, func() bool { t.Fatal("unrelated key entered filter"); return false }) {
			t.Fatalf("unrelated symbol %x", symbol)
		}
	}
}

func TestFunctionalControlCommitsAreNotDuplicated(t *testing.T) {
	for _, tc := range []struct {
		symbol uint32
		text   string
	}{
		{0xff0d, "\r"}, {0xff0d, "\n"}, {0xff0d, "\r\n"}, {0xff09, "\t"}, {0xfe20, "\t"},
		{0xff1b, "\x1b"}, {0xff08, "\b"}, {0xff08, "\x7f"},
	} {
		var h functionalHarness
		for i := 0; i < 2; i++ {
			h.handle(platform.KeyPressed, tc.symbol, func() bool { h.send(platform.SurfaceEvent{Kind: platform.TextInput, Text: tc.text}); return true })
		}
		h.handle(platform.KeyReleased, tc.symbol, func() bool { return false })
		if len(h.events) != 3 || h.events[0].Kind != platform.KeyPressed || !h.events[1].Repeat || h.events[2].Kind != platform.KeyReleased {
			t.Fatalf("control %q: %+v", tc.text, h.events)
		}
	}
}

func TestFunctionalIMEOwnsCommitAndHeldKey(t *testing.T) {
	for _, mode := range []string{"already-composing", "start-and-end", "async", "text-without-preedit"} {
		for _, text := range []string{"中文", "\r"} {
			var h functionalHarness
			if mode == "already-composing" {
				h.keys.Preedit(true)
			}
			filters := 0
			filter := func() bool {
				filters++
				if filters == 1 {
					if mode == "start-and-end" {
						h.keys.Preedit(true)
					}
					if mode == "already-composing" || mode == "start-and-end" {
						h.keys.Preedit(false)
					}
					if mode != "async" {
						h.send(platform.SurfaceEvent{Kind: platform.TextInput, Text: text})
					}
					return true
				}
				return false
			}
			// F1 has no legitimate text commit, even without preedit signals.
			h.handle(platform.KeyPressed, 0xffbe, filter)
			if mode == "async" {
				h.send(platform.SurfaceEvent{Kind: platform.TextInput, Text: text})
			}
			h.handle(platform.KeyPressed, 0xffbe, filter)
			h.handle(platform.KeyReleased, 0xffbe, filter)
			want := []platform.SurfaceEvent{{Kind: platform.TextInput, Text: text}}
			if filters != 3 || !reflect.DeepEqual(h.events, want) || len(h.keys.held) != 0 {
				t.Fatalf("%s %q: %+v / %d", mode, text, h.events, filters)
			}
		}
	}
	// A composition Enter can commit exactly the same control byte as a
	// plain Enter; the preedit transition, not the byte alone, determines it.
	var h functionalHarness
	h.handle(platform.KeyPressed, 0xff0d, func() bool {
		h.keys.Preedit(true)
		h.keys.Preedit(false)
		h.send(platform.SurfaceEvent{Kind: platform.TextInput, Text: "\r"})
		return true
	})
	h.handle(platform.KeyReleased, 0xff0d, func() bool { return false })
	if len(h.events) != 1 || h.events[0].Kind != platform.TextInput {
		t.Fatalf("composition Enter became raw: %+v", h.events)
	}
}

func TestFunctionalFocusShortcutAndHeldIdentity(t *testing.T) {
	var h functionalHarness
	filter := func() bool { return false }
	h.handle(platform.KeyPressed, 0xffbe, filter)
	h.handle(platform.KeyPressed, 0xffbf, filter) // same hardware, changed symbol
	h.handle(platform.KeyReleased, 'x', filter)
	if len(h.events) != 3 || h.events[1].Key != platform.KeyF1 || h.events[2].Key != platform.KeyF1 {
		t.Fatalf("held identity changed: %+v", h.events)
	}
	h = functionalHarness{reject: true}
	h.handle(platform.KeyPressed, 0xff0d, func() bool { h.send(platform.SurfaceEvent{Kind: platform.TextInput, Text: "\r"}); return true })
	h.handle(platform.KeyReleased, 0xff0d, filter)
	if len(h.events) != 1 || h.events[0].Kind != platform.KeyPressed {
		t.Fatalf("shortcut leaked: %+v", h.events)
	}
	h = functionalHarness{}
	h.handle(platform.KeyPressed, 0xff0d, func() bool {
		h.keys.Reset()
		h.send(platform.SurfaceEvent{Kind: platform.TextInput, Text: "old focus"})
		return true
	})
	h.handle(platform.KeyReleased, 0xff0d, filter)
	if len(h.events) != 0 || len(h.keys.held) != 0 {
		t.Fatalf("reset leaked input: %+v", h.events)
	}
	h.handle(platform.KeyPressed, 0xffbe, filter)
	h.keys.Preedit(true)
	h.handle(platform.KeyPressed, 0xffbe, filter)
	h.keys.Preedit(false)
	h.handle(platform.KeyReleased, 0xffbe, filter)
	if len(h.events) != 2 || h.events[0].Kind != platform.KeyPressed || h.events[1].Kind != platform.KeyReleased {
		t.Fatalf("composition interrupted accepted pair: %+v", h.events)
	}
}

func TestFunctionalNestedReleaseDoesNotRevivePress(t *testing.T) {
	var h functionalHarness
	h.handle(platform.KeyPressed, 0xffbe, func() bool { h.handle(platform.KeyReleased, 0xffbe, func() bool { return false }); return false })
	if len(h.events) != 0 || len(h.keys.held) != 0 {
		t.Fatalf("revived key after nested release: %+v", h.events)
	}
}

func TestFunctionalFocusChangeDuringDelivery(t *testing.T) {
	for _, composition := range []bool{false, true} {
		var h functionalHarness
		send := func(ev platform.SurfaceEvent) bool {
			if h.keys.Capture(ev) {
				return true
			}
			h.events = append(h.events, ev)
			h.keys.Reset()
			return true
		}
		h.keys.Handle(platform.KeyPressed, 0xffbe, 38, 0, send, func() bool {
			if composition {
				send(platform.SurfaceEvent{Kind: platform.TextInput, Text: "first"})
				send(platform.SurfaceEvent{Kind: platform.TextInput, Text: "second"})
			}
			return composition
		})
		h.keys.Handle(platform.KeyReleased, 0xffbe, 38, 0, send, func() bool { return false })
		if len(h.events) != 1 || len(h.keys.held) != 0 {
			t.Fatalf("composition=%v old focus events: %+v", composition, h.events)
		}
	}
}

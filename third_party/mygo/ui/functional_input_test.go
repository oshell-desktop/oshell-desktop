package ui

import (
	"github.com/egoist/mygo/internal/gdkinput"
	"github.com/egoist/mygo/internal/platform"
	"testing"
)

func TestGDKFunctionalInputAndShortcutRouting(t *testing.T) {
	for _, shortcut := range []bool{false, true} {
		var events []InputEvent
		called := 0
		tt := NewTester(func(c *Context) {
			if shortcut && c.Shortcut(Ctrl, KeyEnter) {
				called++
			}
			Box(c).Fill().Focusable().AutoFocus().TextCaret(Rect{W: 1, H: 20}).FunctionalInput(true).HandleInput(func(ev InputEvent) bool { events = append(events, ev); return true })
		}, 200, 100)
		defer tt.rt.close()
		var keys gdkinput.FunctionalKeys
		send := func(ev platform.SurfaceEvent) bool {
			if keys.Capture(ev) {
				return true
			}
			accepted := tt.rt.event(ev)
			tt.settle()
			return accepted
		}
		keys.Handle(platform.KeyPressed, 0xff0d, 36, platform.ModCtrl, send, func() bool { send(platform.SurfaceEvent{Kind: platform.TextInput, Text: "\r"}); return true })
		keys.Handle(platform.KeyReleased, 0xff0d, 36, platform.ModCtrl, send, func() bool { return false })
		if shortcut {
			if called != 1 || len(events) != 0 {
				t.Fatalf("shortcut %d leaked events %+v", called, events)
			}
		} else if len(events) != 2 || events[0].Key != KeyEnter || events[0].Kind != InputKeyDown || events[1].Key != KeyEnter || events[1].Kind != InputKeyUp {
			t.Fatalf("control commit lost key identity or duplicated text: %+v", events)
		}
	}
}

func TestGDKFunctionalCompositionOrdering(t *testing.T) {
	var events []InputEvent
	tt := NewTester(func(c *Context) {
		Box(c).Fill().Focusable().AutoFocus().TextCaret(Rect{W: 1, H: 20}).FunctionalInput(true).HandleInput(func(ev InputEvent) bool { events = append(events, ev); return true })
	}, 200, 100)
	defer tt.rt.close()
	var keys gdkinput.FunctionalKeys
	send := func(ev platform.SurfaceEvent) bool {
		if keys.Capture(ev) {
			return true
		}
		accepted := tt.rt.event(ev)
		tt.settle()
		return accepted
	}
	keys.Handle(platform.KeyPressed, 0xff0d, 36, 0, send, func() bool {
		keys.Preedit(true)
		send(platform.SurfaceEvent{Kind: platform.TextComposition, Text: "中", Caret: 1})
		keys.Preedit(false)
		send(platform.SurfaceEvent{Kind: platform.TextComposition})
		send(platform.SurfaceEvent{Kind: platform.TextInput, Text: "中文"})
		return true
	})
	keys.Handle(platform.KeyReleased, 0xff0d, 36, 0, send, func() bool { return false })
	if len(events) != 3 || events[0].Kind != InputCompose || events[0].Text != "中" || events[1].Kind != InputCompose || events[1].Text != "" || events[2].Kind != InputText || events[2].Text != "中文" {
		t.Fatalf("IME sequence changed: %+v", events)
	}
}

func TestGDKPrintableInputAndShortcutRouting(t *testing.T) {
	for _, shortcut := range []bool{false, true} {
		var events []InputEvent
		calls := 0
		tt := NewTester(func(c *Context) {
			if shortcut && c.Shortcut(Ctrl, KeyA) {
				calls++
			}
			Box(c).Fill().Focusable().AutoFocus().TextCaret(Rect{W: 1, H: 20}).FunctionalInput(true).HandleInput(func(ev InputEvent) bool { events = append(events, ev); return true })
		}, 200, 100)
		var keys gdkinput.FunctionalKeys
		send := func(ev platform.SurfaceEvent) bool {
			if keys.Capture(ev) {
				return true
			}
			taken := tt.rt.event(ev)
			tt.settle()
			return taken
		}
		for _, kind := range []platform.SurfaceEventKind{platform.KeyPressed, platform.KeyReleased} {
			keys.HandlePrintable(kind, 'a', 38, platform.ModCtrl, 'a', 'a', false, send, func() bool {
				if kind == platform.KeyReleased {
					return false
				}
				send(platform.SurfaceEvent{Kind: platform.TextInput, Text: "\x01"})
				return true
			})
		}
		if shortcut {
			if calls != 1 || len(events) != 0 {
				t.Fatalf("shortcut: %d %+v", calls, events)
			}
		} else {
			if len(events) != 2 || events[0].Unshifted != 'a' || events[0].Text != "\x01" || events[1].Unshifted != 'a' {
				t.Fatalf("metadata: %+v", events)
			}
		}
		events = nil
		keys.HandlePrintable(platform.KeyPressed, '€', 26, 0, 'e', '€', true, send, func() bool { send(platform.SurfaceEvent{Kind: platform.TextInput, Text: "€"}); return true })
		keys.HandlePrintable(platform.KeyReleased, '€', 26, 0, 'e', '€', true, send, func() bool { return false })
		if len(events) != 1 || events[0].Kind != InputText || !events[0].Literal || events[0].Text != "€" {
			t.Fatalf("literal: %+v", events)
		}
		tt.rt.close()
	}
}

func TestLayoutAlternatesReachUIOnPressAndRelease(t *testing.T) {
	var events []InputEvent
	tt := NewTester(func(c *Context) {
		Box(c).Fill().Focusable().AutoFocus().HandleInput(func(ev InputEvent) bool { events = append(events, ev); return true })
	}, 200, 100)
	defer tt.rt.close()
	for _, kind := range []platform.SurfaceEventKind{platform.KeyPressed, platform.KeyReleased} {
		tt.rt.event(platform.SurfaceEvent{Kind: kind, Key: platform.KeyUnknown, Mods: platform.ModShift, Unshifted: 'я', Shifted: 'Я', BaseLayout: 'z', TextPending: true})
		tt.settle()
	}
	if len(events) != 2 {
		t.Fatalf("events: %+v", events)
	}
	for _, ev := range events {
		if ev.Unshifted != 'я' || ev.Shifted != 'Я' || ev.BaseLayout != 'z' || !ev.TextPending {
			t.Fatalf("lost layout metadata: %+v", ev)
		}
	}
}

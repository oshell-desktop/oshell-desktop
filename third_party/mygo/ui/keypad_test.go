package ui

import (
	"testing"

	"github.com/egoist/mygo/internal/gdkinput"
	"github.com/egoist/mygo/internal/platform"
)

func TestKeypadMetadataReachesFocusedHandler(t *testing.T) {
	var events []InputEvent
	tt := NewTester(func(c *Context) {
		Box(c).Fill().Focusable().AutoFocus().HandleInput(func(ev InputEvent) bool { events = append(events, ev); return true })
	}, 200, 100)
	defer tt.rt.close()
	for _, pad := range []bool{false, true} {
		for _, kind := range []platform.SurfaceEventKind{platform.KeyPressed, platform.KeyReleased} {
			tt.send(platform.SurfaceEvent{Kind: kind, Key: platform.KeyEnter, Keypad: pad, Mods: platform.ModCtrl, Repeat: kind == platform.KeyPressed})
		}
	}
	if len(events) != 4 {
		t.Fatalf("events: %+v", events)
	}
	for i, ev := range events {
		if ev.Key != KeyEnter || ev.Mods != Ctrl || ev.Keypad != (i >= 2) || ev.Repeat != (i%2 == 0) {
			t.Fatalf("event %d: %+v", i, ev)
		}
	}
}

func TestFunctionalTargetAndGDKEventRouting(t *testing.T) {
	var events []InputEvent
	tt := NewTester(func(c *Context) {
		Box(c).Fill().Focusable().AutoFocus().TextCaret(Rect{W: 1, H: 20}).FunctionalInput(true).HandleInput(func(ev InputEvent) bool { events = append(events, ev); return true })
	}, 200, 100)
	defer tt.rt.close()
	target := tt.h.ime.FunctionalTarget
	if target == 0 || !tt.h.ime.Active {
		t.Fatal("custom text handler did not request keypad identity")
	}
	var kp gdkinput.Keypad
	send := func(ev platform.SurfaceEvent) bool { taken := tt.rt.event(ev); tt.settle(); return taken }
	filter := func() { t.Fatal("plain keypad must not disappear into IME text-only route") }
	kp.Handle(platform.KeyPressed, 0xffb1, 87, 0, send, filter)
	kp.Handle(platform.KeyReleased, 0xffb1, 87, 0, send, filter)
	if len(events) != 3 || events[0].Key != Key1 || !events[0].Keypad || events[1].Text != "1" || events[2].Kind != InputKeyUp || !events[2].Keypad {
		t.Fatalf("GDK/UI bridge: %+v", events)
	}
	tt.SetFocused(false)
	if tt.h.ime.FunctionalTarget != 0 {
		t.Fatal("blur retained keypad target")
	}
	tt.SetFocused(true)
	if tt.h.ime.FunctionalTarget != target {
		t.Fatal("focus restore changed handler identity")
	}
}

func TestTextClientKeepsNormalKeypadIMEPolicy(t *testing.T) {
	c := &primitiveClient{}
	tt := primitiveTester(c)
	defer tt.rt.close()
	if !tt.h.ime.Active || tt.h.ime.Client == nil || tt.h.ime.FunctionalTarget != 0 {
		t.Fatalf("document editor requested raw keypad events: %+v", tt.h.ime)
	}
}

func TestGDKKeypadWindowShortcutDoesNotType(t *testing.T) {
	called := 0
	var events []InputEvent
	tt := NewTester(func(c *Context) {
		if c.Shortcut(Ctrl, Key1) {
			called++
		}
		Box(c).Fill().Focusable().AutoFocus().TextCaret(Rect{W: 1, H: 20}).FunctionalInput(true).HandleInput(func(ev InputEvent) bool { events = append(events, ev); return true })
	}, 200, 100)
	defer tt.rt.close()
	var kp gdkinput.Keypad
	send := func(ev platform.SurfaceEvent) bool { taken := tt.rt.event(ev); tt.settle(); return taken }
	filter := func() { t.Fatal("shortcut entered IME") }
	kp.Handle(platform.KeyPressed, 0xffb1, 87, platform.ModCtrl, send, filter)
	kp.Handle(platform.KeyReleased, 0xffb1, 87, platform.ModCtrl, send, filter)
	if called != 1 || len(events) != 0 {
		t.Fatalf("shortcut=%d, unexpected input %+v", called, events)
	}
}

func TestFunctionalTargetChangesWithCustomHandler(t *testing.T) {
	second := false
	tt := NewTester(func(c *Context) {
		id := "first"
		if second {
			id = "second"
		}
		Box(c.Key(id)).Fill().Focusable().AutoFocus().TextCaret(Rect{W: 1, H: 20}).FunctionalInput(true).HandleInput(func(InputEvent) bool { return true })
	}, 200, 100)
	defer tt.rt.close()
	first := tt.h.ime.FunctionalTarget
	second = true
	tt.Frame()
	if first == 0 || tt.h.ime.FunctionalTarget == 0 || first == tt.h.ime.FunctionalTarget {
		t.Fatalf("focus handoff at same caret retained target: %d -> %d", first, tt.h.ime.FunctionalTarget)
	}
}

func TestTextOnlyCustomHandlerKeepsNormalKeypadIMEPolicy(t *testing.T) {
	tt := NewTester(func(c *Context) {
		Box(c).Fill().Focusable().AutoFocus().TextCaret(Rect{W: 1, H: 20}).HandleInput(func(ev InputEvent) bool { return ev.Kind == InputText })
	}, 200, 100)
	defer tt.rt.close()
	if !tt.h.ime.Active || tt.h.ime.FunctionalTarget != 0 {
		t.Fatalf("text-only handler requested raw keys: %+v", tt.h.ime)
	}
}

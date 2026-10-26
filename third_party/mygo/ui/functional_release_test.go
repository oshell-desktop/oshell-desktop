package ui

import (
	"github.com/egoist/mygo/internal/platform"
	"testing"
)

func TestFunctionalReleaseRequiresAcceptedPress(t *testing.T) {
	for _, mode := range []string{"accept", "reject", "shortcut", "orphan", "repeat-rejected"} {
		t.Run(mode, func(t *testing.T) {
			var events []InputEvent
			pressed := 0
			shortcuts := 0
			tt := NewTester(func(c *Context) {
				if mode == "shortcut" && c.Shortcut(Ctrl, KeyA) {
					shortcuts++
				}
				Box(c).Fill().Focusable().AutoFocus().TextCaret(Rect{W: 1, H: 20}).FunctionalInput(true).HandleInput(func(ev InputEvent) bool {
					events = append(events, ev)
					if ev.Kind == InputKeyDown {
						pressed++
						return mode != "reject" && (mode != "repeat-rejected" || pressed == 1)
					}
					return true
				})
			}, 200, 100)
			defer tt.rt.close()
			if mode != "orphan" {
				tt.rt.event(platform.SurfaceEvent{Kind: platform.KeyPressed, Key: platform.KeyA, Mods: platform.ModCtrl})
				tt.settle()
			}
			if mode == "repeat-rejected" {
				tt.rt.event(platform.SurfaceEvent{Kind: platform.KeyPressed, Key: platform.KeyA, Mods: platform.ModCtrl, Repeat: true})
				tt.settle()
			}
			tt.rt.event(platform.SurfaceEvent{Kind: platform.KeyReleased, Key: platform.KeyA})
			tt.settle()
			releases := 0
			for _, ev := range events {
				if ev.Kind == InputKeyUp {
					releases++
				}
			}
			want := 0
			if mode == "accept" || mode == "repeat-rejected" {
				want = 1
			}
			if releases != want {
				t.Fatalf("releases=%d want=%d events=%+v", releases, want, events)
			}
			if mode == "shortcut" && shortcuts != 1 {
				t.Fatalf("shortcut calls %d", shortcuts)
			}
		})
	}
}

func TestFunctionalReleaseDoesNotCrossFocus(t *testing.T) {
	for _, mode := range []string{"switch", "switch-back", "blur", "reentrant", "reentrant-release"} {
		t.Run(mode, func(t *testing.T) {
			second := false
			var events []InputEvent
			var tt *Tester
			tt = NewTester(func(c *Context) {
				name := "first"
				if second {
					name = "second"
				}
				Box(c.Key(name)).Fill().Focusable().AutoFocus().TextCaret(Rect{W: 1, H: 20}).FunctionalInput(true).HandleInput(func(ev InputEvent) bool {
					events = append(events, ev)
					if mode == "reentrant-release" && ev.Kind == InputKeyDown {
						tt.rt.event(platform.SurfaceEvent{Kind: platform.KeyReleased, Key: platform.KeyF1})
					}
					if mode == "reentrant" && ev.Kind == InputKeyDown {
						tt.rt.event(platform.SurfaceEvent{Kind: platform.SurfaceBlur})
						tt.rt.event(platform.SurfaceEvent{Kind: platform.SurfaceFocus})
					}
					return true
				})
			}, 200, 100)
			defer tt.rt.close()
			tt.rt.event(platform.SurfaceEvent{Kind: platform.KeyPressed, Key: platform.KeyF1})
			tt.settle()
			switch mode {
			case "switch", "switch-back":
				second = true
				tt.Frame()
				if mode == "switch-back" {
					second = false
					tt.Frame()
				}
			case "blur":
				tt.SetFocused(false)
				tt.SetFocused(true)
			}
			events = nil
			tt.rt.event(platform.SurfaceEvent{Kind: platform.KeyReleased, Key: platform.KeyF1})
			tt.settle()
			if len(events) != 0 {
				t.Fatalf("old release reached focus: %+v", events)
			}
		})
	}
}

func TestFunctionalReleaseIdentityAndDefaultHandlers(t *testing.T) {
	for _, functional := range []bool{false, true} {
		var events []InputEvent
		tt := NewTester(func(c *Context) {
			Box(c).Fill().Focusable().AutoFocus().TextCaret(Rect{W: 1, H: 20}).FunctionalInput(functional).HandleInput(func(ev InputEvent) bool { events = append(events, ev); return true })
		}, 200, 100)
		down := platform.SurfaceEvent{Kind: platform.KeyPressed, Key: platform.Key1, Mods: platform.ModCtrl}
		tt.rt.event(down)
		tt.settle()
		up := platform.SurfaceEvent{Kind: platform.KeyReleased, Key: platform.Key1, Keypad: true}
		tt.rt.event(up)
		tt.settle()
		up.Keypad = false
		tt.rt.event(up)
		tt.settle()
		want := 3
		if functional {
			want = 2
		}
		if len(events) != want || events[len(events)-1].Kind != InputKeyUp || events[len(events)-1].Keypad {
			t.Fatalf("functional=%v keypad identity: %+v", functional, events)
		}
		events = nil
		down.Key, down.Unshifted = platform.KeyUnknown, 'я'
		tt.rt.event(down)
		tt.settle()
		up.Key, up.Unshifted = platform.KeyUnknown, '界'
		tt.rt.event(up)
		tt.settle()
		up.Unshifted = 'я'
		tt.rt.event(up)
		tt.settle()
		if len(events) != want || events[len(events)-1].Unshifted != 'я' {
			t.Fatalf("functional=%v Unicode identity: %+v", functional, events)
		}
		tt.rt.close()
	}
}

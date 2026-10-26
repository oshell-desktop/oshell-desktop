package gdkinput

import (
	"github.com/egoist/mygo/internal/platform"
	"testing"
)

func TestPrintableIdentityRepeatReleaseAndLayout(t *testing.T) {
	for _, tc := range []struct {
		base, actual rune
		text         string
		mods         platform.Modifiers
	}{
		{'a', 'a', "a", 0}, {'a', 'A', "A", platform.ModShift}, {'1', '!', "!", platform.ModShift},
		{'я', 'Я', "Я", platform.ModShift}, {'界', '界', "界", 0}, {'é', 'é', "é", 0}, {' ', ' ', " ", 0},
		{'c', 'c', "\x03", platform.ModCtrl}, {'a', 'a', "", platform.ModAlt},
	} {
		var h functionalHarness
		for i, kind := range []platform.SurfaceEventKind{platform.KeyPressed, platform.KeyPressed, platform.KeyReleased} {
			if h.keys.Handle(kind, uint32(tc.actual), 38, tc.mods, h.send, func() bool { t.Fatal("printable routed as functional"); return false }) {
				t.Fatal("wrong handler")
			}
			base := tc.base
			if i == 2 {
				base = 'z'
			} // group changes cannot change the held identity
			if !h.keys.HandlePrintable(kind, uint32(tc.actual), 38, tc.mods, base, tc.actual, false, h.send, func() bool {
				if kind != platform.KeyReleased && tc.text != "" {
					h.send(platform.SurfaceEvent{Kind: platform.TextInput, Text: tc.text})
					return true
				}
				return false
			}) {
				t.Fatal("not handled")
			}
		}
		if len(h.events) != 3 {
			t.Fatalf("%q: %+v", tc.text, h.events)
		}
		for i, ev := range h.events {
			if ev.Unshifted != tc.base || ev.Repeat != (i == 1) || ev.Key != platform.KeyForRune(tc.base) {
				t.Fatalf("identity %q: %+v", tc.text, ev)
			}
			shifted := rune(0)
			if tc.mods&platform.ModShift != 0 {
				shifted = tc.actual
			}
			if ev.Shifted != shifted {
				t.Fatalf("shifted metadata: %+v", ev)
			}
			if i < 2 && ev.Text != tc.text {
				t.Fatalf("commit changed: %+v", ev)
			}
		}
	}
}

func TestPrintableCompositionAndLiteralLevels(t *testing.T) {
	for _, mode := range []string{"preedit", "mismatch", "multi", "async", "literal", "reset", "reject"} {
		t.Run(mode, func(t *testing.T) {
			var h functionalHarness
			h.reject = mode == "reject"
			for i, kind := range []platform.SurfaceEventKind{platform.KeyPressed, platform.KeyPressed, platform.KeyReleased} {
				h.keys.HandlePrintable(kind, 'e', 38, 0, 'e', '€', mode == "literal", h.send, func() bool {
					if kind == platform.KeyReleased {
						return false
					}
					if mode == "preedit" {
						h.keys.Preedit(true)
						h.send(platform.SurfaceEvent{Kind: platform.TextComposition, Text: "候选"})
						h.keys.Preedit(false)
					}
					if mode == "reset" {
						h.keys.Reset()
						h.send(platform.SurfaceEvent{Kind: platform.TextInput, Text: "old"})
						return true
					}
					if mode == "async" {
						return true
					}
					text := "€"
					if mode == "mismatch" {
						text = "中"
					}
					if mode == "multi" {
						text = "e\u0301"
					}
					h.send(platform.SurfaceEvent{Kind: platform.TextInput, Text: text})
					return true
				})
				_ = i
			}
			switch mode {
			case "literal":
				if len(h.events) != 2 {
					t.Fatalf("literal count: %+v", h.events)
				}
				for _, ev := range h.events {
					if ev.Kind != platform.TextInput || ev.Text != "€" || !ev.Literal {
						t.Fatalf("literal: %+v", ev)
					}
				}
			case "reset", "async":
				if len(h.events) != 0 {
					t.Fatalf("leaked: %+v", h.events)
				}
			case "reject":
				if len(h.events) != 2 || h.events[0].Kind != platform.KeyPressed || h.events[1].Kind != platform.KeyPressed {
					t.Fatalf("rejected: %+v", h.events)
				}
			default:
				for _, ev := range h.events {
					if ev.Kind == platform.KeyPressed || ev.Kind == platform.KeyReleased || ev.Literal {
						t.Fatalf("IME leaked key: %+v", ev)
					}
				}
			}
		})
	}
}

func TestPrintableReleaseAfterLayoutDisappears(t *testing.T) {
	var h functionalHarness
	h.keys.HandlePrintable(platform.KeyPressed, 'я', 38, 0, 'я', 'я', false, h.send, func() bool { h.send(platform.SurfaceEvent{Kind: platform.TextInput, Text: "я"}); return true })
	if h.keys.Handle(platform.KeyReleased, 0, 38, 0, h.send, func() bool { t.Fatal("wrong route"); return false }) {
		t.Fatal("wrong handler")
	}
	if !h.keys.HandlePrintable(platform.KeyReleased, 0, 38, 0, 0, 0, false, h.send, func() bool { return false }) {
		t.Fatal("lost release")
	}
	if len(h.events) != 2 || h.events[1].Unshifted != 'я' || h.events[1].Kind != platform.KeyReleased {
		t.Fatalf("release: %+v", h.events)
	}
}

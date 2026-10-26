package gdkinput

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

func TestKeypadSymbols(t *testing.T) {
	for _, tc := range []struct {
		symbol uint32
		key    platform.Key
		text   string
	}{
		{0xff80, platform.KeySpace, " "}, {0xff89, platform.KeyTab, ""}, {0xff8d, platform.KeyEnter, ""},
		{0xff91, platform.KeyF1, ""}, {0xff92, platform.KeyF2, ""}, {0xff93, platform.KeyF3, ""}, {0xff94, platform.KeyF4, ""},
		{0xff95, platform.KeyHome, ""}, {0xff96, platform.KeyLeft, ""}, {0xff97, platform.KeyUp, ""},
		{0xff98, platform.KeyRight, ""}, {0xff99, platform.KeyDown, ""}, {0xff9a, platform.KeyPageUp, ""},
		{0xff9b, platform.KeyPageDown, ""}, {0xff9c, platform.KeyEnd, ""}, {0xff9d, platform.KeyClear, ""},
		{0xff9e, platform.KeyInsert, ""}, {0xff9f, platform.KeyDelete, ""},
		{0xffaa, platform.KeyMultiply, "*"}, {0xffab, platform.KeyAdd, "+"}, {0xffac, platform.KeyComma, ","},
		{0xffad, platform.KeyMinus, "-"}, {0xffae, platform.KeyPeriod, "."}, {0xffaf, platform.KeySlash, "/"},
		{0xffbd, platform.KeyEqual, "="},
		{0xffb0, platform.Key0, "0"}, {0xffb1, platform.Key1, "1"}, {0xffb2, platform.Key2, "2"},
		{0xffb3, platform.Key3, "3"}, {0xffb4, platform.Key4, "4"}, {0xffb5, platform.Key5, "5"},
		{0xffb6, platform.Key6, "6"}, {0xffb7, platform.Key7, "7"}, {0xffb8, platform.Key8, "8"}, {0xffb9, platform.Key9, "9"},
		{'1', platform.KeyUnknown, ""}, {0xff0d, platform.KeyUnknown, ""}, {0xff7f, platform.KeyUnknown, ""}, {0xffffff, platform.KeyUnknown, ""},
	} {
		t.Run(fmt.Sprintf("%x", tc.symbol), func(t *testing.T) {
			var k Keypad
			var events []platform.SurfaceEvent
			send := func(ev platform.SurfaceEvent) bool { events = append(events, ev); return true }
			filter := func() { t.Fatal("ordinary keypad key entered IME") }
			handled := k.Handle(platform.KeyPressed, tc.symbol, 10, platform.ModShift, send, filter)
			if handled != (tc.key != platform.KeyUnknown) {
				t.Fatalf("handled=%t", handled)
			}
			if !handled {
				return
			}
			k.Handle(platform.KeyReleased, tc.symbol, 10, 0, send, filter)
			want := []platform.SurfaceEvent{{Kind: platform.KeyPressed, Key: tc.key, Keypad: true, Mods: platform.ModShift}}
			if tc.text != "" {
				want = append(want, platform.SurfaceEvent{Kind: platform.TextInput, Text: tc.text})
			}
			want = append(want, platform.SurfaceEvent{Kind: platform.KeyReleased, Key: tc.key, Keypad: true})
			if !reflect.DeepEqual(events, want) {
				t.Fatalf("events %+v, want %+v", events, want)
			}
		})
	}
}

func TestKeypadHeldNumLockIdentity(t *testing.T) {
	var k Keypad
	var got []platform.SurfaceEvent
	send := func(ev platform.SurfaceEvent) bool { got = append(got, ev); return true }
	filter := func() { t.Fatal("unexpected IME") }
	k.Handle(platform.KeyPressed, 0xffb1, 87, 0, send, filter)
	k.Handle(platform.KeyPressed, 0xff9c, 87, platform.ModCtrl, send, filter)
	k.Handle(platform.KeyReleased, 0xff9c, 87, 0, send, filter)
	if len(got) != 5 || got[2].Key != platform.Key1 || !got[2].Repeat || got[3].Text != "1" || got[4].Key != platform.Key1 || len(k.held) != 0 {
		t.Fatalf("Num Lock changed a held key: %+v", got)
	}
}

func TestKeypadCompositionOwnsPressAndRelease(t *testing.T) {
	for _, symbol := range []uint32{0xffb1, 0xff8d, 0xff96} {
		var k Keypad
		k.Composing = true
		filters := 0
		send := func(platform.SurfaceEvent) bool { t.Fatal("preedit leaked a keypad event"); return false }
		filter := func() { filters++; k.Composing = false } // IME commits and ends preedit
		k.Handle(platform.KeyPressed, symbol, 87, 0, send, filter)
		k.Handle(platform.KeyPressed, symbol, 87, 0, send, filter)
		k.Handle(platform.KeyReleased, symbol, 87, 0, send, filter)
		if filters != 3 || len(k.held) != 0 {
			t.Fatalf("composition routing: %d filters, %+v", filters, k.held)
		}
	}
}

func TestKeypadCompositionDuringHeldPress(t *testing.T) {
	var k Keypad
	var got []platform.SurfaceEvent
	send := func(ev platform.SurfaceEvent) bool { got = append(got, ev); return true }
	filters := 0
	filter := func() { filters++ }
	k.Handle(platform.KeyPressed, 0xffb1, 87, 0, send, filter)
	k.Composing = true
	k.Handle(platform.KeyPressed, 0xffb1, 87, 0, send, filter)
	k.Handle(platform.KeyReleased, 0xffb1, 87, 0, send, filter)
	if filters != 1 || len(got) != 3 || got[2].Kind != platform.KeyReleased {
		t.Fatalf("composition must suppress repeats and pair the original release: %+v / %d", got, filters)
	}
}

func TestKeypadShortcutAndFocusIsolation(t *testing.T) {
	for _, resetInCallback := range []bool{false, true} {
		var k Keypad
		var got []platform.SurfaceEvent
		send := func(ev platform.SurfaceEvent) bool {
			got = append(got, ev)
			if resetInCallback {
				k.Reset()
			}
			return resetInCallback
		}
		filter := func() { t.Fatal("shortcut/focus change entered IME") }
		k.Handle(platform.KeyPressed, 0xffb1, 87, platform.ModCtrl, send, filter)
		k.Handle(platform.KeyReleased, 0xffb1, 87, 0, send, filter)
		if len(got) != 1 || got[0].Kind != platform.KeyPressed || len(k.held) != 0 {
			t.Fatalf("rejected/reset press emitted text or release: %+v", got)
		}
	}
	var k Keypad
	send := func(platform.SurfaceEvent) bool { return true }
	k.Handle(platform.KeyPressed, 0xffb1, 87, 0, send, func() {})
	k.Composing = true
	k.Reset()
	if k.Composing || len(k.held) != 0 {
		t.Fatal("reset retained a composition/held key")
	}
	k.Handle(platform.KeyReleased, 0xff9c, 87, 0, func(platform.SurfaceEvent) bool { t.Fatal("orphan release"); return false }, func() { t.Fatal("orphan entered IME") })
}

//go:build windows && (amd64 || arm64)

package windows

import (
	"runtime"
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

type keypadRecorder struct {
	platform.WindowHandler
	events []platform.SurfaceEvent
}

func TestKeypadHeldIdentitySurvivesNumLockAndFocus(t *testing.T) {
	h := &keypadRecorder{}
	s := &surface{w: &window{h: h}}
	const scan = 0x4f << 16
	s.message(0, wmKeyDown, 0x61, scan)       // keypad 1
	s.message(0, wmKeyDown, 0x23, scan|1<<30) // same physical key after Num Lock
	s.message(0, wmKeyUp, 0x23, scan|1<<30)
	for _, ev := range h.events {
		if ev.Key != platform.Key1 || !ev.Keypad {
			t.Fatalf("changed held key: %+v", ev)
		}
	}
	if len(s.keypadKeys) != 0 {
		t.Fatal("key-up retained held keypad")
	}
	s.message(0, wmKeyDown, 0x61, scan)
	s.message(0, wmKillFocus, 0, 0)
	if len(s.keypadKeys) != 0 {
		t.Fatal("focus loss retained held keypad")
	}
	h.events = nil
	s.message(0, wmKeyDown, 0x23, scan)
	s.message(0, wmKeyUp, 0x23, scan|1<<30)
	for _, ev := range h.events {
		if ev.Key != platform.KeyEnd || !ev.Keypad {
			t.Fatalf("new key inherited old press: %+v", ev)
		}
	}
}

func (h *keypadRecorder) SurfaceEvent(ev platform.SurfaceEvent) bool {
	h.events = append(h.events, ev)
	return true
}

func TestKeypadWindowsMessageIdentity(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	const extended = 1 << 24
	for _, tc := range []struct {
		vk, flags uintptr
		key       platform.Key
		keypad    bool
	}{
		{0x0d, 0, platform.KeyEnter, false}, {0x0d, extended, platform.KeyEnter, true},
		{0x31, 0, platform.Key1, false}, {0x61, 0, platform.Key1, true},
		{0x60, 0, platform.Key0, true}, {0x69, 0, platform.Key9, true},
		{0x6a, 0, platform.KeyMultiply, true}, {0x6b, 0, platform.KeyAdd, true},
		{0x6c, 0, platform.KeyComma, true}, {0x6d, 0, platform.KeyMinus, true},
		{0x6e, 0, platform.KeyPeriod, true}, {0x6f, extended, platform.KeySlash, true},
		{0x0c, 0, platform.KeyClear, true},
		{0x24, 0, platform.KeyHome, true}, {0x24, extended, platform.KeyHome, false},
		{0x23, 0, platform.KeyEnd, true}, {0x23, extended, platform.KeyEnd, false},
		{0x21, 0, platform.KeyPageUp, true}, {0x21, extended, platform.KeyPageUp, false},
		{0x22, 0, platform.KeyPageDown, true}, {0x22, extended, platform.KeyPageDown, false},
		{0x25, 0, platform.KeyLeft, true}, {0x25, extended, platform.KeyLeft, false},
		{0x26, 0, platform.KeyUp, true}, {0x26, extended, platform.KeyUp, false},
		{0x27, 0, platform.KeyRight, true}, {0x27, extended, platform.KeyRight, false},
		{0x28, 0, platform.KeyDown, true}, {0x28, extended, platform.KeyDown, false},
		{0x2d, 0, platform.KeyInsert, true}, {0x2d, extended, platform.KeyInsert, false},
		{0x2e, 0, platform.KeyDelete, true}, {0x2e, extended, platform.KeyDelete, false},
	} {
		h := &keypadRecorder{}
		s := &surface{w: &window{h: h}}
		for _, msg := range []uint32{wmKeyDown, wmKeyDown, wmKeyUp} {
			flags := tc.flags
			if len(h.events) > 0 {
				flags |= 1 << 30
			}
			s.message(0, msg, tc.vk, flags)
		}
		if len(h.events) != 3 {
			t.Fatalf("VK %x: lost key event: %+v", tc.vk, h.events)
		}
		for i, ev := range h.events {
			if ev.Key != tc.key || ev.Keypad != tc.keypad || ev.Repeat != (i == 1) {
				t.Fatalf("VK %x flags %x event %d: %+v", tc.vk, tc.flags, i, ev)
			}
		}
	}
}

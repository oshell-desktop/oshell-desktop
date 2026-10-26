//go:build windows && (amd64 || arm64)

package windows

import (
	"github.com/egoist/mygo/internal/platform"
	"runtime"
	"testing"
	"unsafe"
)

func TestWindowsLayoutPhysicalKeys(t *testing.T) {
	// Independent letter order, including keys whose alphabetical VK order
	// differs completely from their physical scan-code order.
	scans := []byte{0x1e, 0x30, 0x2e, 0x20, 0x12, 0x21, 0x22, 0x23, 0x17, 0x24, 0x25, 0x26, 0x32, 0x31, 0x18, 0x19, 0x10, 0x13, 0x1f, 0x14, 0x16, 0x2f, 0x11, 0x2d, 0x15, 0x2c}
	for i, scan := range scans {
		if got := physicalLayoutRune(uintptr(scan) << 16); got != rune('a'+i) {
			t.Fatalf("scan %x: %q", scan, got)
		}
	}
	for _, tc := range []struct {
		scan uintptr
		r    rune
	}{{2, '1'}, {11, '0'}, {12, '-'}, {13, '='}, {26, '['}, {27, ']'}, {39, ';'}, {40, '\''}, {41, '`'}, {43, '\\'}, {51, ','}, {52, '.'}, {53, '/'}, {57, ' '}} {
		if got := physicalLayoutRune(tc.scan << 16); got != tc.r {
			t.Fatalf("scan %x: %q want %q", tc.scan, got, tc.r)
		}
	}
	for _, flags := range []uintptr{0, 0x1e<<16 | 1<<24, 0x56 << 16, 0x70 << 16} {
		if got := physicalLayoutRune(flags); got != 0 {
			t.Fatalf("invented physical identity %x: %q", flags, got)
		}
	}
}

func TestWindowsLayoutUnicodeAndHeldIdentity(t *testing.T) {
	s := &surface{}
	state := [256]byte{}
	state[0x11], state[0x12], state[0xa2], state[0xa5] = 0x80, 0x80, 0x80, 0x80
	state[0x14] = 1
	calls := 0
	translate := func(vk, scan, layout uintptr, keys *[256]byte) rune {
		calls++
		if vk != 'Z' || scan != 0x2c || layout != 123 {
			t.Fatalf("identity changed %x/%x/%d", vk, scan, layout)
		}
		if keys[0x10] != 0 {
			if keys[0x11] != 0 || keys[0x12] != 0 || keys[0xa2] != 0 || keys[0xa5] != 0 || keys[0x14] != 1 {
				t.Fatalf("wrong state: %+v", keys)
			}
			return 'Я'
		}
		if *keys != [256]byte{} {
			t.Fatal("base inherited modifiers")
		}
		return 'я'
	}
	for i, kind := range []platform.SurfaceEventKind{platform.KeyPressed, platform.KeyPressed, platform.KeyReleased} {
		vk, layout := uintptr('Z'), uintptr(123)
		key := platform.KeyZ
		if i > 0 {
			vk, layout, key = 'Q', 999, platform.KeyQ
		}
		ev := s.layoutEventWith(platform.SurfaceEvent{Kind: kind, Key: key, Mods: platform.ModShift | platform.ModCtrl | platform.ModAlt, Repeat: i == 1}, vk, 0x2c<<16, layout, &state, translate)
		if ev.Key != platform.KeyZ || ev.Unshifted != 'я' || ev.Shifted != 'Я' || ev.BaseLayout != 'z' || !ev.TextPending || ev.Repeat != (i == 1) {
			t.Fatalf("event: %+v", ev)
		}
	}
	if calls != 4 || len(s.layoutKeys) != 0 {
		t.Fatalf("queries %d/held %+v", calls, s.layoutKeys)
	}
	if state[0x11] != 0x80 || state[0x14] != 1 {
		t.Fatal("mutated keyboard snapshot")
	}
}

func TestWindowsLayoutUnknownDeadAndScope(t *testing.T) {
	s := &surface{}
	state := [256]byte{}
	ev := platform.SurfaceEvent{Kind: platform.KeyPressed, Key: platform.KeyUnknown}
	got := s.layoutEventWith(ev, 0xe2, 0x56<<16, 123, &state, func(uintptr, uintptr, uintptr, *[256]byte) rune { return '<' })
	if got.Unshifted != '<' || got.BaseLayout != 0 || !got.TextPending {
		t.Fatalf("ISO key: %+v", got)
	}
	got = s.layoutEventWith(ev, 0xde, 0x28<<16, 123, &state, func(uintptr, uintptr, uintptr, *[256]byte) rune { return 0 })
	if got.Unshifted != 0 || got.TextPending {
		t.Fatalf("dead key invented metadata: %+v", got)
	}
	s.layoutKeys[0x1e] = heldLayout{base: 'a'}
	if got = s.layoutEvent(platform.SurfaceEvent{Kind: platform.KeyReleased, Key: platform.KeyA}, 'A', 0x1e<<16); got.Unshifted != 0 || len(s.layoutKeys) != 1 {
		t.Fatalf("non-terminal release cleanup: %+v %+v", got, s.layoutKeys)
	}
	s.layoutKeys[0x1e] = heldLayout{base: 'a'}
	s.input.FunctionalTarget = 1
	s.layoutDead = true
	s.layoutEvent(platform.SurfaceEvent{Kind: platform.KeyReleased, Key: platform.KeyA}, 'A', 0x1e<<16)
	if _, ok := s.layoutKeys[0x1e]; ok {
		t.Fatal("composition retained old key")
	}
}

func TestWindowsLayoutReadOnlySystemTranslation(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if windowsBuild() < 14393 {
		t.Skip("read-only translation requires Windows 10 1607")
	}
	var before, after [256]byte
	if ok, _, _ := procGetKeyboardState.Call(uintptr(unsafe.Pointer(&before))); ok == 0 {
		t.Fatal("GetKeyboardState")
	}
	layout, _, _ := procGetKeyboardLayout.Call(0)
	var empty [256]byte
	first := translatedLayoutRune('A', 0x1e, layout, &empty)
	for range 20 {
		if r := translatedLayoutRune('A', 0x1e, layout, &empty); r != first {
			t.Fatalf("unstable read-only query: %U/%U", first, r)
		}
	}
	if first == 0 {
		t.Fatal("current layout produced no A identity")
	}
	if ok, _, _ := procGetKeyboardState.Call(uintptr(unsafe.Pointer(&after))); ok == 0 {
		t.Fatal("GetKeyboardState")
	}
	if before != after {
		t.Fatal("layout queries changed thread keyboard state")
	}
	t.Logf("current-layout base for VK_A: %U", first)
}

func TestWindowsLayoutMessagesAndFocus(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if windowsBuild() < 14393 {
		t.Skip("read-only layout unavailable")
	}
	h := &keypadRecorder{}
	s := &surface{w: &window{h: h}, input: platform.TextInputState{Active: true, FunctionalTarget: 1}}
	s.message(0, wmKeyDown, 'A', 0x1e<<16)
	s.message(0, wmChar, 'a', 0x1e<<16)
	s.message(0, wmKeyUp, 'A', 0x1e<<16|1<<30)
	if len(h.events) != 3 || h.events[0].Unshifted == 0 || !h.events[0].TextPending || h.events[1].Kind != platform.TextInput || h.events[2].Unshifted != h.events[0].Unshifted {
		t.Fatalf("message chain: %+v", h.events)
	}
	s.message(0, wmKeyDown, 'A', 0x1e<<16)
	s.message(0, wmKillFocus, 0, 0)
	if len(s.layoutKeys) != 0 {
		t.Fatal("focus retained layout identities")
	}
	s.layoutKeys = map[uint16]heldLayout{1: {base: 'a'}}
	s.SetTextInput(platform.TextInputState{Active: true, FunctionalTarget: 2})
	if len(s.layoutKeys) != 0 {
		t.Fatal("target change retained layout identities")
	}
	s.message(0, wmDeadChar, '^', 0)
	h.events = nil
	s.message(0, wmKeyDown, 'A', 0x1e<<16)
	if len(h.events) != 1 || h.events[0].Unshifted != 0 {
		t.Fatalf("dead-key sequence obtained metadata: %+v", h.events)
	}
	s.message(0, wmChar, 'â', 0)
	if s.layoutDead {
		t.Fatal("commit retained dead-key marker")
	}

}

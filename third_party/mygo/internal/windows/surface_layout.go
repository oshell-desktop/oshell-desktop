//go:build windows && (amd64 || arm64)

package windows

import (
	"github.com/egoist/mygo/internal/platform"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"
)

var (
	procGetKeyboardState  = user32.NewProc("GetKeyboardState")
	procGetKeyboardLayout = user32.NewProc("GetKeyboardLayout")
	procToUnicodeEx       = user32.NewProc("ToUnicodeEx")
)

type heldLayout struct {
	key            platform.Key
	vk, layout     uintptr
	base, physical rune
}

type layoutTranslator func(vk, scan, layout uintptr, state *[256]byte) rune

// Querying with bit 2 is non-mutating on Windows 10 1607 and later. Never
// call the stateful fallback: it would consume dead keys before WM_CHAR.
func translatedLayoutRune(vk, scan, layout uintptr, state *[256]byte) rune {
	var buf [8]uint16
	n, _, _ := procToUnicodeEx.Call(vk, scan, uintptr(unsafe.Pointer(state)), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 4, layout)
	count := int32(n)
	if count <= 0 || count > int32(len(buf)) {
		return 0
	}
	r := rune(buf[0])
	if utf16.IsSurrogate(r) {
		if count < 2 {
			return 0
		}
		r = utf16.DecodeRune(r, rune(buf[1]))
	}
	if r < 0x20 || r == 0x7f || r >= 0x80 && r <= 0x9f || r == utf8.RuneError || !utf8.ValidRune(r) {
		return 0
	}
	return r
}

// physicalLayoutRune maps the standardized PC set-1 writing keys to the
// same US base-layout identities used by OShell's Mac scan-code map. It
// does not guess from the virtual key (which changes with keyboard layout).
// Extended, ISO/JIS extra and synthetic scan-less keys have no guessed base.
func physicalLayoutRune(flags uintptr) rune {
	if flags&(1<<24) != 0 {
		return 0
	}
	scan := uint8(flags >> 16)
	for _, row := range []struct {
		first uint8
		text  string
	}{
		{0x02, "1234567890-="}, {0x10, "qwertyuiop[]"}, {0x1e, "asdfghjkl;'"}, {0x2c, "zxcvbnm,./"},
	} {
		if int(scan) >= int(row.first) && int(scan) < int(row.first)+len(row.text) {
			return rune(row.text[int(scan-row.first)])
		}
	}
	switch scan {
	case 0x29:
		return '`'
	case 0x2b:
		return '\\'
	case 0x39:
		return ' '
	}
	return 0
}

func layoutWritingKey(vk uintptr) bool {
	return vk >= '0' && vk <= '9' || vk >= 'A' && vk <= 'Z' || vk == 0x20 || vk >= 0xba && vk <= 0xc0 || vk >= 0xdb && vk <= 0xdf || vk == 0xe2
}

func (s *surface) layoutEvent(ev platform.SurfaceEvent, vk, flags uintptr) platform.SurfaceEvent {
	if ev.Kind == platform.KeyReleased {
		defer delete(s.layoutKeys, layoutKeyID(vk, flags))
	}
	if s.input.FunctionalTarget == 0 || ev.Keypad || s.layoutComposing || s.layoutDead || windowsBuild() < 14393 {
		return ev
	}
	var state [256]byte
	if ok, _, _ := procGetKeyboardState.Call(uintptr(unsafe.Pointer(&state))); ok == 0 {
		return ev
	}
	layout, _, _ := procGetKeyboardLayout.Call(0)
	return s.layoutEventWith(ev, vk, flags, layout, &state, translatedLayoutRune)
}

func (s *surface) layoutEventWith(ev platform.SurfaceEvent, vk, flags, layout uintptr, state *[256]byte, translate layoutTranslator) platform.SurfaceEvent {
	id := layoutKeyID(vk, flags)
	h, held := s.layoutKeys[id]
	if ev.Kind == platform.KeyReleased {
		delete(s.layoutKeys, id)
	}
	if !held {
		if !layoutWritingKey(vk) || layout == 0 {
			return ev
		}
		var empty [256]byte
		base := translate(vk, flags>>16&0xff, layout, &empty)
		if base == 0 {
			return ev
		}
		h = heldLayout{key: ev.Key, vk: vk, layout: layout, base: unicode.ToLower(base), physical: physicalLayoutRune(flags)}
	}
	if ev.Kind == platform.KeyPressed {
		if s.layoutKeys == nil {
			s.layoutKeys = make(map[uint16]heldLayout)
		}
		s.layoutKeys[id] = h
	}
	ev.Key, ev.Unshifted, ev.BaseLayout, ev.TextPending = h.key, h.base, h.physical, true
	if ev.BaseLayout == ev.Unshifted {
		ev.BaseLayout = 0
	}
	if ev.Mods&platform.ModShift != 0 {
		shifted := *state
		// Shift/Caps affect the shifted identity; Ctrl/Alt do not turn it into
		// a control byte or AltGr character. Actual text still comes from WM_CHAR.
		for _, key := range []int{0x11, 0x12, 0xa2, 0xa3, 0xa4, 0xa5} {
			shifted[key] = 0
		}
		shifted[0x10], shifted[0xa0] = 0x80, 0x80
		ev.Shifted = translate(h.vk, flags>>16&0xff, h.layout, &shifted)
	}
	return ev
}

func layoutKeyID(vk, flags uintptr) uint16 {
	id := uint16(flags >> 16 & 0x1ff)
	if id&0xff == 0 {
		id = uint16(vk) | 0x200 | id&0x100
	}
	return id
}

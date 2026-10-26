package gdkinput

import "unsafe"

// PhysicalXKBName identifies writing positions, independent of the keymap's
// symbols. ISO/JIS extra keys and unknown names have no guessed US identity.
func PhysicalXKBName(name [4]byte) rune {
	for _, row := range []struct{ prefix, text string }{
		{"AE", "1234567890-="}, {"AD", "qwertyuiop[]"},
		{"AC", "asdfghjkl;'"}, {"AB", "zxcvbnm,./"},
	} {
		if name[0] != row.prefix[0] || name[1] != row.prefix[1] || name[2] < '0' || name[2] > '9' || name[3] < '0' || name[3] > '9' {
			continue
		}
		n := int(name[2]-'0')*10 + int(name[3]-'0')
		if n > 0 && n <= len(row.text) {
			return rune(row.text[n-1])
		}
	}
	switch string(name[:]) {
	case "TLDE":
		return '`'
	case "BKSL":
		return '\\'
	case "SPCE":
		return ' '
	}
	return 0
}

// PhysicalWaylandKey uses GDK Wayland's documented evdev + 8 event keycode.
// Do not use this for X11: its server keycodes can be assigned differently.
func PhysicalWaylandKey(code uint16) rune {
	if code < 8 {
		return 0
	}
	code -= 8
	for _, row := range []struct {
		first uint16
		text  string
	}{
		{2, "1234567890-="}, {16, "qwertyuiop[]"}, {30, "asdfghjkl;'"}, {44, "zxcvbnm,./"},
	} {
		if code >= row.first && int(code-row.first) < len(row.text) {
			return rune(row.text[code-row.first])
		}
	}
	switch code {
	case 41:
		return '`'
	case 43:
		return '\\'
	case 57:
		return ' '
	}
	return 0
}

// XKBDescription and XKBNames follow X11's LP64 ABI (Linux amd64/arm64).
// Only named-key metadata is requested; all returned memory stays Xlib-owned.
type XKBDescription struct {
	Display                           uintptr
	Flags, Device                     uint16
	Min, Max                          uint8
	_                                 [2]byte
	Controls, Server, Map, Indicators uintptr
	Names                             *XKBNames
	Compat, Geometry                  uintptr
}
type XKBNames struct {
	Keycodes, Geometry, Symbols, Types, Compat uintptr
	VMods                                      [16]uintptr
	Indicators                                 [32]uintptr
	Groups                                     [4]uintptr
	Keys                                       *[4]byte
	Aliases, RadioGroups, PhysicalSymbols      uintptr
	NumKeys, NumAliases                        uint8
	NumRadioGroups                             uint16
}

// CopyXKBPhysicalKeys copies names before XkbFreeKeyboard releases them.
func CopyXKBPhysicalKeys(desc *XKBDescription) (result [256]rune) {
	if desc == nil || desc.Names == nil || desc.Names.Keys == nil || desc.Min > desc.Max {
		return
	}
	names := unsafe.Slice(desc.Names.Keys, int(desc.Max)+1)
	for code := int(desc.Min); code <= int(desc.Max); code++ {
		result[code] = PhysicalXKBName(names[code])
	}
	return
}

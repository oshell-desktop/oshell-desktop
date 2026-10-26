// Package gdkinput translates GDK input without calling GTK. Keeping the
// translation separate lets its event ordering run in tests on every host.
package gdkinput

import "github.com/egoist/mygo/internal/platform"

// KeypadKey maps GDK_KEY_KP_* symbols from gdk/gdkkeysyms.h. Text is the
// literal keypad symbol, not an approximation of ordinary layout/IME input.
func KeypadKey(symbol uint32) (platform.Key, string) {
	if symbol >= 0xffb0 && symbol <= 0xffb9 {
		return platform.Key0 + platform.Key(symbol-0xffb0), string(rune('0' + symbol - 0xffb0))
	}
	if symbol >= 0xff91 && symbol <= 0xff94 {
		return platform.KeyF1 + platform.Key(symbol-0xff91), ""
	}
	switch symbol {
	case 0xff80:
		return platform.KeySpace, " "
	case 0xff89:
		return platform.KeyTab, ""
	case 0xff8d:
		return platform.KeyEnter, ""
	case 0xff95:
		return platform.KeyHome, ""
	case 0xff96:
		return platform.KeyLeft, ""
	case 0xff97:
		return platform.KeyUp, ""
	case 0xff98:
		return platform.KeyRight, ""
	case 0xff99:
		return platform.KeyDown, ""
	case 0xff9a:
		return platform.KeyPageUp, ""
	case 0xff9b:
		return platform.KeyPageDown, ""
	case 0xff9c:
		return platform.KeyEnd, ""
	case 0xff9d:
		return platform.KeyClear, ""
	case 0xff9e:
		return platform.KeyInsert, ""
	case 0xff9f:
		return platform.KeyDelete, ""
	case 0xffaa:
		return platform.KeyMultiply, "*"
	case 0xffab:
		return platform.KeyAdd, "+"
	case 0xffac:
		return platform.KeyComma, ","
	case 0xffad:
		return platform.KeyMinus, "-"
	case 0xffae:
		return platform.KeyPeriod, "."
	case 0xffaf:
		return platform.KeySlash, "/"
	case 0xffbd:
		return platform.KeyEqual, "="
	}
	return platform.KeyUnknown, ""
}

type heldKey struct {
	key           platform.Key
	text          string
	ime, accepted bool
}

// Keypad preserves functional keypad events for a custom keyboard handler.
// Ordinary text clients must continue to send all events through GTK's IME.
// All methods run on the GUI thread, including send and filter callbacks.
type Keypad struct {
	Composing  bool
	held       map[uint32]heldKey
	generation uint64
}

// Reset drops keys belonging to the previous focus/handler and its preedit.
func (k *Keypad) Reset() {
	k.Composing = false
	k.held = nil
	k.generation++
}

// Handle returns false for non-keypad input; the backend then follows its
// normal IME path. Hardware identifies the physical key across Num Lock
// changes. Only an accepted press may emit text and a matching release.
func (k *Keypad) Handle(kind platform.SurfaceEventKind, symbol uint32, hardware uint16, mods platform.Modifiers, send func(platform.SurfaceEvent) bool, filter func()) bool {
	if kind != platform.KeyPressed && kind != platform.KeyReleased {
		return false
	}
	id := uint32(hardware)
	if id == 0 {
		id = 0x10000 | symbol
	}
	h, held := k.held[id]
	if !held {
		key, text := KeypadKey(symbol)
		if key == platform.KeyUnknown {
			return false
		}
		h = heldKey{key: key, text: text, ime: k.Composing}
	}
	if kind == platform.KeyReleased {
		delete(k.held, id)
		if held {
			if h.ime {
				filter()
			} else if h.accepted {
				send(platform.SurfaceEvent{Kind: kind, Key: h.key, Keypad: true, Mods: mods})
			}
		}
		return true // do not send an orphan release after focus was reset
	}
	if k.held == nil {
		k.held = make(map[uint32]heldKey)
	}
	k.held[id] = h
	if h.ime || k.Composing {
		// A key that started in preedit stays with the IME until released,
		// including repeats after a commit ended the preedit sequence.
		filter()
		return true
	}
	generation := k.generation
	accepted := send(platform.SurfaceEvent{Kind: kind, Key: h.key, Keypad: true, Mods: mods, Repeat: held})
	// A handler can change focus (or open a nested native event loop).
	if generation != k.generation {
		return true
	}
	if _, stillHeld := k.held[id]; !stillHeld {
		return true
	}
	h.accepted = h.accepted || accepted
	k.held[id] = h
	if accepted && h.text != "" {
		send(platform.SurfaceEvent{Kind: platform.TextInput, Text: h.text})
	}
	return true
}

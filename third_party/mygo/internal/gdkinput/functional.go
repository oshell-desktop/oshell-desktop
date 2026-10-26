package gdkinput

import (
	"github.com/egoist/mygo/internal/platform"
	"unicode"
	"unicode/utf8"
)

// FunctionalKey identifies non-printing GDK keys. Printable keys use
// HandlePrintable with GTK layout data; keypad symbols have their own translator.
func FunctionalKey(symbol uint32) platform.Key {
	if k := FunctionKey(symbol); k != platform.KeyUnknown {
		return k
	}
	switch symbol {
	case 0xff0d:
		return platform.KeyEnter
	case 0xff1b:
		return platform.KeyEscape
	case 0xff08:
		return platform.KeyBackspace
	case 0xff09, 0xfe20:
		return platform.KeyTab
	case 0xffff:
		return platform.KeyDelete
	case 0xff63:
		return platform.KeyInsert
	case 0xff50:
		return platform.KeyHome
	case 0xff57:
		return platform.KeyEnd
	case 0xff55:
		return platform.KeyPageUp
	case 0xff56:
		return platform.KeyPageDown
	case 0xff51:
		return platform.KeyLeft
	case 0xff52:
		return platform.KeyUp
	case 0xff53:
		return platform.KeyRight
	case 0xff54:
		return platform.KeyDown
	case 0xff67:
		return platform.KeyContextMenu
	}
	return platform.KeyUnknown
}

type functionalHeld struct {
	key           platform.Key
	unshifted     rune
	physical      rune
	serial        uint64
	ime, accepted bool
	literal       bool
}

type inputCapture struct {
	events  []platform.SurfaceEvent
	preedit bool
}

// FunctionalKeys adapts GTK's keyboard events for an opted-in terminal.
// GTK receives every event first. A plain control-character commit can then
// be represented by its functional key; composition commits remain text.
// Only the GUI thread may call these methods.
type FunctionalKeys struct {
	composing          bool
	held               map[uint32]functionalHeld
	generation, serial uint64
	capture            *inputCapture
}

func (k *FunctionalKeys) Reset() {
	k.composing = false
	k.held = nil
	k.generation++
	// An in-flight filter retains its capture so old-focus commits can be
	// discarded when it returns, rather than escaping into the new target.
}

func (k *FunctionalKeys) Preedit(active bool) {
	k.composing = active
	if k.capture != nil {
		k.capture.preedit = true
	}
}

// Capture defers synchronous IME signals until Handle knows whether they came
// from a simple control character or composition. Asynchronous commits and
// signals outside a handled key event retain the normal GTK delivery path.
func (k *FunctionalKeys) Capture(ev platform.SurfaceEvent) bool {
	if k.capture == nil || ev.Kind != platform.TextInput && ev.Kind != platform.TextComposition {
		return false
	}
	k.capture.events = append(k.capture.events, ev)
	return true
}

func (k *FunctionalKeys) Handle(kind platform.SurfaceEventKind, symbol uint32, hardware uint16, mods platform.Modifiers, send func(platform.SurfaceEvent) bool, filter func() bool) bool {
	id := uint32(hardware)
	if id == 0 {
		id = 0x10000 | symbol
	}
	if h, ok := k.held[id]; ok && h.unshifted != 0 {
		return false
	}
	return k.handle(kind, symbol, hardware, mods, 0, 0, 0, false, send, filter)
}

// HandlePrintable uses the layout's unmodified and translated scalars, never
// a character guessed from a US keyboard. Text still comes from GTK's IME.
// levelText marks a consumed layout modifier other than Shift/Caps Lock.
func (k *FunctionalKeys) HandlePrintable(kind platform.SurfaceEventKind, symbol uint32, hardware uint16, mods platform.Modifiers, base, translated rune, levelText bool, send func(platform.SurfaceEvent) bool, filter func() bool) bool {
	return k.HandlePrintableLayout(kind, symbol, hardware, mods, base, translated, 0, levelText, send, filter)
}

// HandlePrintableLayout adds an independently resolved physical US position.
// Hold its identity through layout changes until the matching release.
func (k *FunctionalKeys) HandlePrintableLayout(kind platform.SurfaceEventKind, symbol uint32, hardware uint16, mods platform.Modifiers, base, translated, physical rune, levelText bool, send func(platform.SurfaceEvent) bool, filter func() bool) bool {
	return k.handle(kind, symbol, hardware, mods, unicode.ToLower(base), translated, physical, levelText, send, filter)
}

func (k *FunctionalKeys) handle(kind platform.SurfaceEventKind, symbol uint32, hardware uint16, mods platform.Modifiers, base, translated, physical rune, levelText bool, send func(platform.SurfaceEvent) bool, filter func() bool) bool {
	if kind != platform.KeyPressed && kind != platform.KeyReleased {
		return false
	}
	id := uint32(hardware)
	if id == 0 {
		id = 0x10000 | symbol
	}
	h, held := k.held[id]
	if !held {
		h.key = FunctionalKey(symbol)
		if printableScalar(base) {
			h.key, h.unshifted = platform.KeyForRune(base), base
			if printableScalar(physical) && physical != base {
				h.physical = physical
			}
		}
		if h.key == platform.KeyUnknown && h.unshifted == 0 {
			return false
		}
	}
	shifted := rune(0)
	if h.unshifted != 0 && mods&platform.ModShift != 0 && printableScalar(translated) {
		shifted = translated
	}
	generation := k.generation
	wasComposing := k.composing
	if kind == platform.KeyReleased {
		delete(k.held, id)
	} else {
		if k.held == nil {
			k.held = make(map[uint32]functionalHeld)
		}
		k.serial++
		h.serial = k.serial
		k.held[id] = h
	}
	frame, parent := &inputCapture{}, k.capture
	k.capture = frame
	consumed := filter()
	k.capture = parent
	if generation != k.generation {
		return true
	}
	flush := func() {
		for _, ev := range frame.events {
			if generation != k.generation {
				break
			}
			send(ev)
		}
	}
	if kind == platform.KeyReleased {
		flush()
		if held && h.accepted && generation == k.generation {
			send(platform.SurfaceEvent{Kind: kind, Key: h.key, Mods: mods, Unshifted: h.unshifted, Shifted: shifted, BaseLayout: h.physical})
		}
		return true
	}
	if current, ok := k.held[id]; !ok || current.serial != h.serial {
		return true
	}
	// Even if preedit ends inside filter(), that event still belongs to the
	// IME. A consumed event with no synchronous commit may be asynchronous.
	plain := plainControlCommit(h.key, frame.events)
	text := ""
	if h.unshifted != 0 && len(frame.events) == 1 && frame.events[0].Kind == platform.TextInput {
		text = frame.events[0].Text
		// A mismatching or multi-scalar commit is composition, even if an IME
		// did not emit preedit. Control commits belong to the shortcut encoder.
		plain = text == string(translated) && printableScalar(translated) || utf8.RuneCountInString(text) == 1 && len(text) == 1 && (text[0] < 0x20 || text[0] == 0x7f)
	}
	ime := h.ime || wasComposing || frame.preedit || (consumed || len(frame.events) > 0) && !plain
	if !ime && h.unshifted != 0 && levelText && text == string(translated) && printableScalar(translated) {
		// AltGr-like levels are literal text, as OShell's Option compose path.
		h.ime, h.literal = true, true
		k.held[id] = h
		send(platform.SurfaceEvent{Kind: platform.TextInput, Text: text, Literal: true})
		return true
	}
	h.ime = ime
	k.held[id] = h
	if ime {
		if h.literal && text == string(translated) && printableScalar(translated) && !wasComposing && !frame.preedit {
			for i := range frame.events {
				frame.events[i].Literal = true
			}
		}
		flush()
		return true
	}
	accepted := send(platform.SurfaceEvent{Kind: kind, Key: h.key, Mods: mods, Repeat: held, Unshifted: h.unshifted, Shifted: shifted, BaseLayout: h.physical, Text: text})
	if generation != k.generation {
		return true
	}
	if current, ok := k.held[id]; !ok || current.serial != h.serial {
		return true
	}
	h.accepted = h.accepted || accepted
	k.held[id] = h
	return true
}

func plainControlCommit(key platform.Key, events []platform.SurfaceEvent) bool {
	if len(events) != 1 || events[0].Kind != platform.TextInput {
		return false
	}
	text := events[0].Text
	switch key {
	case platform.KeyEnter:
		return text == "\r" || text == "\n" || text == "\r\n"
	case platform.KeyTab:
		return text == "\t"
	case platform.KeyEscape:
		return text == "\x1b"
	case platform.KeyBackspace:
		return text == "\b" || text == "\x7f"
	}
	return false
}

func printableScalar(r rune) bool {
	return r >= 0x20 && r != 0x7f && !(r >= 0x80 && r <= 0x9f) && utf8.ValidRune(r)
}

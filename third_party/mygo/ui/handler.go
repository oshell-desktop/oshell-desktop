package ui

import "time"

// InputKind is the kind of an InputEvent.
type InputKind uint8

// Kinds of input.
const (
	// InputKeyDown and InputKeyUp report a key with the modifiers held;
	// Repeat marks the auto-repeat of a key held down. A key that types
	// text comes before the text, an InputText event, on macOS and
	// Windows; on Linux, only the text comes while the element takes text
	// (TextCaret), except keys requested with FunctionalInput outside
	// a composition. Layout-aware printable keys carry Unshifted and their
	// synchronous Text commit together unless TextPending requests a separate
	// InputText (as on Windows).
	InputKeyDown InputKind = iota + 1
	InputKeyUp
	// InputText is text typed, or committed by an input method.
	InputText
	// InputCompose is the composition of an input method, with its caret
	// at rune Caret; an empty Text ends it.
	InputCompose
	// InputCommand is an edit command of the menus, such as Copy of the
	// Edit menu: "copy", "cut", "paste", "selectAll", "undo", "redo" or
	// "delete".
	InputCommand
	// InputPointerDown and InputPointerUp report Button pressed or
	// released at X, Y, with Clicks the count of quick successive presses

	// (2 for a double click). Counts belong to the window sequence, so replacing
	// an element under the pointer does not restart them. Once the element takes an InputPointerDown,
	// the pointer's moves and its release come to it wherever they are,
	// and InputPointerCancel comes if capture or the window focus is lost.
	InputPointerDown
	InputPointerUp
	// InputPointerMove reports the pointer at X, Y, with Button held, or
	// -1 when none is.
	InputPointerMove
	// InputScroll scrolls by DX, DY DIPs at X, Y; Precise marks
	// touchpads, which scroll by pixels rather than by lines.
	InputScroll
	// InputCancel immediately clears pending keyboard/composition state when a
	// FunctionalInput handler loses focus. It is not a key release or text commit,
	// and is delivered even if focus returns before the next frame is rendered.
	InputCancel
	// InputPointerCancel ends capture without copying, activating, or dropping.
	InputPointerCancel
)

// InputEvent is input an element takes as it comes (HandleInput).
type InputEvent struct {
	// Unshifted identifies a layout-aware printable key. On key-down Text is
	// its synchronous IME commit (possibly empty for Ctrl/Alt shortcuts).
	// A zero value retains the platform's ordinary key-then-text path.
	Unshifted rune
	// TextPending preserves the platform's separate text/IME commit after a
	// layout-aware key-down. False means Text is already the complete commit.
	TextPending bool
	// Shifted and BaseLayout are optional Unicode layout identities, independent
	// of committed Text. They also accompany key-up when available.
	Shifted, BaseLayout rune
	// Literal marks a layout-composed InputText that must remain literal text.
	Literal bool
	Kind    InputKind
	Key     Key
	Mods    Modifiers
	Repeat  bool
	// Keypad distinguishes numeric keypad events without changing their
	// logical Key used by ordinary controls and shortcuts.
	Keypad bool
	// Text of InputText, InputCompose, InputCommand or layout-aware InputKeyDown, and Caret the
	// rune of an InputCompose's caret.
	Text  string
	Caret int
	// X and Y locate the pointer relative to the element's box.
	X, Y float32
	// Button is 0 for the primary button, 1 the secondary and 2 the
	// middle.
	Button int
	Clicks int
	// PointerDown may carry the source event clock, independent of dispatch delay.
	// Compare timestamps only within one surface. Zero is valid when present.
	HasTimestamp bool
	Timestamp    time.Duration
	// Host double-click settings at this press; the flag permits a valid zero interval.
	// Distances are maximum axis deltas in DIPs.
	HasDoubleClickSettings                     bool
	DoubleClickInterval                        time.Duration
	DoubleClickDistanceX, DoubleClickDistanceY float32
	// DX and DY are what InputScroll scrolls by; positive DY moves the view
	// down the content.
	DX, DY  float32
	Precise bool
}

// HandleInput has fn take the element's input as it comes, on the main
// thread, before the next frame is built: keys, text and edit commands
// while the element has the keyboard focus, the pointer pressed on it,
// moving over it or scrolling over it. fn reports whether it took the
// event; one it leaves goes on as it would without fn, to shortcuts, Tab,
// context menus and scroll containers. Keys that the window or an element
// around the focus handles as a Shortcut go to the shortcut first. A frame
// follows the events fn takes.
//
// It is for widgets that need every key as it is pressed, such as a
// terminal; most widgets ask about their input as they are built instead
// (Clicked, Shortcut, Dragged).
func (e *node) HandleInput(fn func(ev InputEvent) bool) *node {
	e.inputFn = fn
	return e
}

// TextCaret has the element take text from the system's input methods
// while it has the keyboard focus, with their composition at r, the caret,
// relative to the element's box: candidate windows show there. Its
// InputText and Compose events (HandleInput) bring the text.
func (e *node) TextCaret(r Rect) *node {
	e.caret, e.takesText = r, true
	return e
}

// FunctionalInput preserves layout, non-printing and keypad events for a custom
// HandleInput element using TextCaret, such as a terminal. On Linux, plain
// control-character IME commits retain their functional key identity; real
// composition remains text. The handler must accept a press (return true)
// to receive its paired release. Ordinary editors should leave this disabled.
func (e *node) FunctionalInput(enabled bool) *node {
	e.functionalInput = enabled
	return e
}

// handler returns the state of the innermost element of chain that
// handles its input.
func (rt *engine) handler(chain []uint64) *state {
	for _, id := range chain {
		if s := rt.states[id]; s != nil && s.input != nil && s.flags&flagDisabled == 0 {
			return s
		}
	}
	return nil
}

// deliver gives ev to the element of s, with the pointer's position
// relative to its box, and reports whether it took it.
func (rt *engine) deliver(s *state, ev InputEvent) bool {
	if s == nil || s.input == nil {
		return false
	}
	ev.X, ev.Y = rt.pointerX-s.x, rt.pointerY-s.y
	if !s.input(ev) {
		return false
	}
	rt.requestFrame()
	return true
}

// focusHandler returns the state of the focused element when it handles
// its input.
func (rt *engine) focusHandler() *state {
	if s := rt.states[rt.focused]; s != nil && s.input != nil && rt.windowFocused {
		return s
	}
	return nil
}

package terminal

import (
	"github.com/egoist/mygo/ui"
)

// SendFunctionKey bridges native extended-key monitors to the normal input path.
// Call on the UI thread; the focused view and Ghostty protocol state own encoding.
func (t *Terminal) SendFunctionKey(number int, mods ui.Modifiers, release, repeat bool) bool {
	if number < 13 || number > 35 || t.v == nil || !t.v.focused || !t.ManagedInputAvailable() {
		return false
	}
	kind := ui.InputKeyDown
	if release {
		kind = ui.InputKeyUp
	}
	ev := ui.InputEvent{Kind: kind, Key: ui.KeyF13 + ui.Key(number-13), Mods: mods, Repeat: repeat && !release}
	handled := t.v.input(ev)
	if handled {
		if t.opts.OnUserInput != nil {
			t.opts.OnUserInput(ev)
		}
		t.redraw()
	}
	// Legacy protocols omit releases; still consume the matching native event.
	return handled || release
}

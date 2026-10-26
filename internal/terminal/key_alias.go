package terminal

import "github.com/egoist/mygo/ui"

// KeyAlias maps a press, repeat and release to the same target combination.
// Both combinations pass through the terminal's active keyboard protocol.
type KeyAlias struct {
	Key, TargetKey   ui.Key
	Mods, TargetMods ui.Modifiers
}

type keyIdentity struct {
	key       ui.Key
	keypad    bool
	unshifted rune
}

// SetKeyAliases updates alternatives on the UI thread, like SetInputPolicy.
func (t *Terminal) SetKeyAliases(aliases []KeyAlias) {
	t.opts.KeyAliases = append([]KeyAlias(nil), aliases...)
}

func (v *view) aliasKeyEvent(ev ui.InputEvent) ui.InputEvent {
	id := keyIdentity{ev.Key, ev.Keypad, ev.Unshifted}
	alias, matched := v.pressedAliases[id]
	if ev.Kind == ui.InputKeyUp {
		delete(v.pressedAliases, id)
	} else if !matched {
		for _, candidate := range v.t.opts.KeyAliases {
			if ev.Key == candidate.Key && ev.Mods == candidate.Mods {
				alias, matched = candidate, true
				if v.pressedAliases == nil {
					v.pressedAliases = make(map[keyIdentity]KeyAlias)
				}
				v.pressedAliases[id] = alias
				break
			}
		}
	}
	if matched {
		ev.Key, ev.Mods = alias.TargetKey, alias.TargetMods
		ev.Unshifted, ev.Shifted, ev.BaseLayout, ev.Text = 0, 0, 0, ""
		ev.TextPending = false
		ev.Keypad = false // aliases name logical target keys, not keypad keys
	}
	return ev
}

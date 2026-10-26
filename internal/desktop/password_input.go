package desktop

import (
	"github.com/egoist/mygo/ui"
	"runtime"
	"unicode/utf8"
)

func passwordInput(c *ui.Context, value *string) ui.Element {
	return passwordSelection(ui.TextInput(c, value), value)
}
func passwordSelection(input ui.Element, value *string) ui.Element {
	input.Password()
	return passwordSelectAll(input, value)
}
func passwordSelectAll(input ui.Element, value *string) ui.Element {
	if runtime.GOOS == "darwin" {
		pending := ui.Local(input, "password-select-all", func() bool { return false })
		if *pending {
			input.SetTextSelection(0, utf8.RuneCountInString(*value))
			*pending = false
		}
		input.HandleInput(func(ev ui.InputEvent) bool {
			if ev.Kind != ui.InputKeyDown || ev.Key != ui.KeyA || ev.Mods != ui.Ctrl {
				return false
			}
			// The UI editor takes rune offsets, not UTF-8 bytes or AppKit UTF-16 units.
			// Selection never reads or writes the clipboard.
			*pending = true
			return true
		})
	}
	return input
}

// Uses the same editor in both modes so its selection and Unicode text survive.
func revealPasswordInput(c *ui.Context, value *string, revealed *bool) ui.Element {
	var input ui.Element
	ui.Row(c).FillWidth().Gap(8).Children(func() {
		input = passwordSelectAll(ui.TextInput(c, value).NoSpellcheck(), value).Grow(1)
		if !*revealed {
			input.Password()
		}
		last := ui.Local(input, "last-password-visibility", func() bool { return *revealed })
		if *last != *revealed {
			input.Focus()
			*last = *revealed
		}
		ui.Checkbox(c, revealed, "显示密码").Tooltip("仅显示本次输入；已保存的密码不会回显")
	})
	return input
}

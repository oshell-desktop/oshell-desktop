package gdkinput

import "github.com/egoist/mygo/internal/platform"

// InputPurpose returns GTK 3 input-purpose and input-hints values. These
// are advisory; never treat them as validation or a guarantee against learning.
func InputPurpose(state platform.TextInputState) (purpose, hints int32) {
	if !state.Active {
		return 0, 0
	}
	switch {
	case state.Password:
		purpose = 8 // GTK_INPUT_PURPOSE_PASSWORD
	case state.FunctionalTarget != 0:
		purpose = 10 // GTK_INPUT_PURPOSE_TERMINAL
	}
	if state.Password || state.NoSpellcheck || state.FunctionalTarget != 0 {
		hints = 2 // GTK_INPUT_HINT_NO_SPELLCHECK
	}
	return
}

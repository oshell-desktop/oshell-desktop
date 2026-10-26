package gdkinput

import (
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

func TestNativeInputPurposes(t *testing.T) {
	for _, tc := range []struct {
		name           string
		state          platform.TextInputState
		purpose, hints int32
	}{
		{"plain", platform.TextInputState{Active: true}, 0, 0},
		{"password", platform.TextInputState{Active: true, Password: true}, 8, 2},
		{"revealed", platform.TextInputState{Active: true, NoSpellcheck: true}, 0, 2},
		{"terminal", platform.TextInputState{Active: true, FunctionalTarget: 1}, 10, 2},
		{"password-priority", platform.TextInputState{Active: true, Password: true, FunctionalTarget: 1}, 8, 2},
		{"inactive", platform.TextInputState{Password: true, NoSpellcheck: true, FunctionalTarget: 1}, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			purpose, hints := InputPurpose(tc.state)
			if purpose != tc.purpose || hints != tc.hints {
				t.Fatalf("native properties = (%d, %d), want (%d, %d)", purpose, hints, tc.purpose, tc.hints)
			}
		})
	}
}

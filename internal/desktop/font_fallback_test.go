package desktop

import (
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestMissingTerminalFontUsesMonospace(t *testing.T) {
	font := ui.Font{Family: terminalFamily("OShell Missing Font Fixture"), Size: 18}
	want := ui.Font{Family: terminalFamily(""), Size: 18}
	for _, text := range []string{"MWil09", "\ue0a0"} {
		if !reflect.DeepEqual(ui.Shape(text, font), ui.Shape(text, want)) {
			t.Fatalf("missing font fallback differs for %q", text)
		}
	}
}

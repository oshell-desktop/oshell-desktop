package desktop

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"os"
	"reflect"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("OSHELL_P2_NATIVE_LIFECYCLE") == "1" {
		os.Exit(runP2NativeLifecycle())
	}
	os.Setenv("OSHELL_TEST_NO_AGENT", "1")
	os.Setenv("OSHELL_TEST_IGNORE_SSH_CONFIG", "1")
	os.Setenv("OSHELL_TEST_EXPLICIT_KEYS_ONLY", "1")
	if e := registerFonts(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func TestBundledSymbolFont(t *testing.T) {
	direct := ui.Shape("\ue0a0", ui.Font{Family: "Symbols Nerd Font Mono", Size: 18})
	for _, family := range []string{terminalFamily(""), terminalFamily("DejaVu Sans Mono")} {
		glyphs := ui.Shape("\ue0a0", ui.Font{Family: family, Size: 18})
		if !reflect.DeepEqual(direct, glyphs) {
			t.Fatalf("symbol fallback differs for %s", family)
		}
	}

	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Padding(20).Gap(12).Children(func() {
			for _, family := range []string{"Symbols Nerd Font Mono", terminalFamily(""), terminalFamily("DejaVu Sans Mono")} {
				ui.Text(c, "\ue0a0").Font(family).FontSize(28)
			}
		})
	}, 350, 200)
	snapshot(t, tt, "font-symbol.png")
}

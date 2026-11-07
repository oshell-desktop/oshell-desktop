package terminal

import (
	"context"
	"fmt"
	"math"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

func TestCascadiaLightGridAndSelection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	fonts, err := core.FontCatalog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	installed := false
	for _, font := range fonts {
		installed = installed || font.Name == "Cascadia Code Light"
	}
	if !installed {
		t.Skip("installed Cascadia Code Light required for Windows font acceptance")
	}
	// The app registers its own normal/bold Cascadia faces at startup. They
	// must not shadow the installed Light face while resolving its GDI name.
	for _, style := range []string{"Regular", "Bold", "Italic", "BoldItalic"} {
		data, err := os.ReadFile("../desktop/assets/fonts/CascadiaCode-" + style + ".ttf")
		if err != nil {
			t.Fatal(err)
		}
		if err := ui.RegisterFont(data, ""); err != nil {
			t.Fatal(err)
		}
	}
	loadLib(t)
	for _, scale := range []float32{1, 1.5, 2} {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			font := Font{Family: "Cascadia Code Light, monospace", Size: 18}
			term, err := New(Options{Conn: newPipe(), NoBlink: true, Font: font})
			if err != nil {
				t.Fatal(err)
			}
			defer term.Close()
			tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill().AutoFocus() }, 1000, 170)
			tt.SetScale(scale)
			text := "Web console: https://example.invalid:9090/"
			term.Feed([]byte(text + "\r\nLast login: fixture from 192.0.2.20\r\n\x1b[32muser@host\x1b[0m:~$ \x1b[1mbold\x1b[0m \x1b[3mitalic\x1b[0m 中文"))
			tt.Frame()
			// Cascadia's installed Light and normal faces have the same mono
			// advance. The old GDI-name miss produced a proportional UI face.
			reference := ui.Shape("M", ui.Font{Family: "Cascadia Code", Size: 18})
			if reflect.DeepEqual(ui.Shape("M", ui.Font{Family: font.Family, Size: 18}), reference) {
				t.Fatal("installed Light was replaced by bundled regular")
			}
			want := max(int(math.Ceil(float64(reference[0].Advance*scale-0.05))), 1)
			if term.v.cellW != want {
				t.Fatalf("cell width %d, want %d at scale %v", term.v.cellW, want, scale)
			}
			for _, g := range ui.Shape("MWi09", ui.Font{Family: font.Family, Size: 18}) {
				if math.Abs(float64(g.Advance-reference[0].Advance)) > 0.05 {
					t.Fatal("ASCII fell back to a proportional font")
				}
			}
			// Pointer selection must follow the corrected grid, not only look
			// narrower. Copy a known substring using actual pointer events.
			v := term.v
			x := (float32(v.ox) + 0.2*float32(v.cellW)) / scale
			y := (float32(v.oy) + 0.5*float32(v.cellH)) / scale
			beginCharacterDrag(tt, x, y)
			tt.Move(x+11*float32(v.cellW)/scale, y)
			tt.Release(x+11*float32(v.cellW)/scale, y)
			tt.Command("copy")
			if got := tt.Clipboard(); got != text[:11] {
				t.Fatalf("pointer selection %q, want %q", got, text[:11])
			}
			save(t, tt, fmt.Sprintf("cascadia-light-%g", scale))
			term.SetFont(Font{Family: "Consolas", Size: 18})
			tt.Frame()
			term.SetFont(font)
			tt.Frame()
			if term.v.cellW != want {
				t.Fatal("font switching left stale grid metrics")
			}
		})
	}
}

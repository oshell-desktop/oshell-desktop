package desktop

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

func TestApplicationBrand(t *testing.T) {
	root := filepath.Join("..", "..")
	manifest, err := os.ReadFile(filepath.Join(root, "mygo.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Icon    string
		Windows struct{ Icon string }
	}
	if err := json.Unmarshal(manifest, &config); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{config.Icon, config.Windows.Icon} {
		if path == "" {
			continue
		}
		source, err := os.ReadFile(filepath.Join(root, path))
		if err != nil || !bytes.Equal(source, brandPNG) {
			t.Fatalf("packaging artwork %q differs from the embedded application icon: %v", path, err)
		}
	}
	if config.Icon == "" {
		t.Fatal("missing shared packaging icon")
	}
	for _, theme := range []string{"light", "dark"} {
		cfg := core.DefaultConfig()
		cfg.Settings.InterfaceTheme = theme
		a := newApp(&core.Store{Dir: t.TempDir()}, cfg)
		for _, page := range []struct {
			name string
			view func(*ui.Context)
			size int
		}{{"welcome", a.view, 48}, {"about", a.settingsView, 28}} {
			t.Run(theme+"/"+page.name, func(t *testing.T) {
				if page.name == "about" {
					a.prepareSettings()
					defer a.closeSettings()
				}
				tt := ui.NewTester(page.view, 900, 560)
				if page.name == "about" {
					if err := tt.Click("关于"); err != nil {
						t.Fatal(err)
					}
				}
				r, ok := tt.Find("OShell Desktop 图标")
				snapshot(t, tt, "brand-"+page.name+"-"+theme+".png")
				if !ok || int(r.W) != page.size || int(r.H) != page.size {
					t.Fatalf("missing or clipped application icon: %v", r)
				}
				// The old graphite tile also covered the full box. The approved
				// artwork's orange frame reaches the top and bottom of it.
				for _, y := range []int{int(r.Y) + 1, int(r.Y+r.H) - 1} {
					p := tt.Image().RGBAAt(int(r.X+r.W/2), y)
					if int(p.R)-int(p.G) < 36 || int(p.G)-int(p.B) < 18 {
						t.Fatalf("missing orange frame at y=%d of %v: %v", y, r, p)
					}
				}
			})
		}
	}
}

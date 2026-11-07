package desktop

import (
	"runtime"
	"testing"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"oshell-desktop/internal/terminal"
)

func TestColorSettingsPickerPreviewAndDraft(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	a.prepareSettings()
	original := a.config.Settings.Scheme().Foreground
	tt := ui.NewTester(a.colorSettings, 800, 1000)
	if !tt.HasText("选中文字") || !tt.HasText("亮红 Aa") {
		t.Fatal("color examples missing", tt.Texts())
	}
	if err := tt.Click("文字颜色选择器"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Red"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.features.colorDraft.Foreground != "#EF4444" {
		t.Fatal("picker failed to write color draft", a.features.colorDraft.Foreground)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	snapshot(t, tt, "color-settings-preview.png")
	if a.config.Settings.Scheme().Foreground != original {
		t.Fatal("picker changed saved scheme before Apply")
	}
	a.features.colorDraft.Background = "invalid"
	tt.Frame()
	if !tt.HasText("请输入 #RRGGBB") {
		t.Fatal("invalid color feedback missing")
	}
	a.closeSettings()
	a.prepareSettings()
	if a.features.colorDraft.Foreground != original {
		t.Fatal("Cancel retained draft color")
	}
}
func TestColorInputRejectsInvalidHexWithoutPanic(t *testing.T) {
	for _, value := range []string{"", "#", "#12345Z", "#12345678", "ffffff"} {
		if _, ok := parseSettingColor(value); ok {
			t.Fatal(value)
		}
	}
	color, ok := parseSettingColor("#12aBcD")
	if !ok || color.R != 0x12 || color.G != 0xab || color.B != 0xcd {
		t.Fatal("valid hex rejected")
	}
}

func TestCursorColorApplyAndCancel(t *testing.T) {
	store := &core.Store{Dir: t.TempDir()}
	a := newApp(store, core.DefaultConfig())
	legacy := a.config.Settings.Scheme().Copy()
	legacy.AutoCursor = true
	a.config.Settings.CustomSchemes = append(a.config.Settings.CustomSchemes, legacy)
	a.config.Settings.ColorSchemeID = legacy.ID
	a.prepareSettings()
	tt := ui.NewTester(a.colorSettings, 900, 1100)
	if err := tt.Click("光标颜色选择器"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Red"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.features.colorDraft.Cursor != "#EF4444" {
		t.Fatal("custom cursor color did not update")
	}
	a.applySettings()
	saved, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	scheme := saved.Settings.Scheme()
	if !scheme.AutoCursor || scheme.Cursor != "#EF4444" || terminalTheme(scheme).Cursor != ui.Hex("#EF4444") {
		t.Fatal("custom mode was not persisted")
	}
	a.prepareSettings()
	a.features.colorDraft.Cursor = "#000000"
	a.closeSettings()
	a.prepareSettings()
	if a.features.colorDraft.Cursor != "#EF4444" {
		t.Fatal("cancel persisted draft cursor color")
	}
}

func TestWindowsPresetCursorOnHighlightedBrace(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows preset behavior")
	}
	for _, scheme := range core.ColorPresets() {
		if scheme.AutoCursor {
			t.Fatalf("%s overrides its preset cursor", scheme.ID)
		}
	}
	a := parityApp(t)
	p, _ := operatorPane(t, a, "cursor")
	th := terminalTheme(a.config.Settings.Scheme())
	p.term.SetTheme(th, nil)
	// Two matched braces, black on white, with the caret on the first.
	p.term.Feed([]byte("\x1b[30;48;2;255;255;255m{}\x1b[0m\r"))
	tt := ui.NewTester(func(c *ui.Context) { terminal.View(c, p.term).Fill().AutoFocus() }, 300, 100)
	before := tt.Image()
	for y := 0; y < before.Rect.Max.Y; y++ {
		for x := 0; x < before.Rect.Max.X; x++ {
			pixel := before.RGBAAt(x, y)
			if pixel.R != 0x55 || pixel.G != 0xe5 || pixel.B != 0xcc {
				continue
			}
			p.term.Feed([]byte("\x1b[?25l"))
			tt.Frame()
			under := tt.Image().RGBAAt(x, y)
			if under.R != 255 || under.G != 255 || under.B != 255 {
				t.Fatalf("lost underlying match background: %v", under)
			}
			return
		}
	}
	t.Fatal("macOS OShell dark preset cursor not drawn over highlighted brace")
}

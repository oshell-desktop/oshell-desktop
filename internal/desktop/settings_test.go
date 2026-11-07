package desktop

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

func TestSettingsSidebarNavigationAndDraftActions(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	a.prepareSettings()
	tt := ui.NewTester(a.settingsView, 700, 560)
	before := a.config.Settings.FontSize
	footer, _ := tt.Find("设置操作区")
	heading, _ := tt.Find("设置页标题")
	tt.Scroll(600, 350, 0, 2000)
	bottom, ok := tt.Find("减少动态效果")
	if !ok || bottom.H == 0 || bottom.Y >= footer.Y {
		t.Fatal("lower general settings are not reachable by scrolling", bottom)
	}
	if got, _ := tt.Find("设置操作区"); got != footer {
		t.Fatal("footer moved with content")
	}
	if got, _ := tt.Find("设置页标题"); got != heading {
		t.Fatal("heading moved with content")
	}
	tt.Scroll(600, 350, 0, -2000)
	if err := tt.Click("增大终端字号"); err != nil {
		t.Fatal(err)
	}
	for i, page := range settingsPages {
		if err := tt.Click(page.title); err != nil {
			t.Fatal(err)
		}
		if a.features.settingsTab != i || !tt.HasText(page.detail) {
			t.Fatal("sidebar opened wrong page", page.title)
		}
	}
	if err := tt.Click("常规"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyDown)
	if a.features.settingsTab != 1 {
		t.Fatal("sidebar arrow navigation failed")
	}
	if a.features.settingsDraft.FontSize != before+1 || a.config.Settings.FontSize != before {
		t.Fatal("navigation lost or saved draft")
	}
	if err := tt.Click("取消"); err != nil {
		t.Fatal(err)
	}
	if a.settingsOpen || a.config.Settings.FontSize != before {
		t.Fatal("cancel saved draft")
	}
	a.prepareSettings()
	a.features.settingsTab = 0
	tt.Frame()
	if err := tt.Click("增大终端字号"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("应用"); err != nil {
		t.Fatal(err)
	}
	if a.settingsOpen || a.config.Settings.FontSize != before+1 {
		t.Fatal("apply did not save draft", a.features.settingsError)
	}
}

func TestSettingsDraftCancelAndApply(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	a.prepareSettings()
	a.features.settingsDraft.FontSize = 18
	a.closeSettings()
	if a.config.Settings.FontSize == 18 {
		t.Fatal("cancel changed settings")
	}
	a.prepareSettings()
	a.features.settingsDraft.FontSize = 18
	a.features.settingsLines = "6000"
	tt := ui.NewTester(a.settingsView, 800, 650)
	snapshot(t, tt, "settings-general.png")
	a.features.settingsTab = 1
	tt.Frame()
	snapshot(t, tt, "settings-colors.png")
	a.features.colorDraft.Background = "#123456"
	a.applySettings()
	if a.config.Settings.FontSize != 18 || a.config.Settings.ScrollbackLines != 6000 || a.config.Settings.Scheme().Background != "#123456" {
		t.Fatal(a.config.Settings)
	}
	if len(a.config.Settings.CustomSchemes) != 1 {
		t.Fatal("editing a preset did not create a custom copy")
	}
}

func TestSettingsThemePreviewDoesNotMutateSavedAppearance(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	a.config.Settings.InterfaceTheme = "light"
	a.prepareSettings()
	a.features.settingsDraft.InterfaceTheme = "dark"
	tt := ui.NewTester(a.settingsView, 800, 650)
	// The left edge belongs to the navigation panel. Sample the exposed
	// window background above the content at the right edge instead.
	pixel := tt.Image().RGBAAt(799, 1)
	darkBackground := professionalTheme(true).Background
	if pixel.R != darkBackground.R || pixel.G != darkBackground.G || pixel.B != darkBackground.B {
		t.Fatalf("whole settings background did not preview dark: %+v", pixel)
	}
	snapshot(t, tt, "settings-preview-dark.png")
	if a.config.Settings.InterfaceTheme != "light" {
		t.Fatal("preview mutated saved appearance")
	}
	a.closeSettings()
	if a.config.Settings.InterfaceTheme != "light" {
		t.Fatal("cancel persisted theme")
	}
	a.prepareSettings()
	a.features.settingsDraft.InterfaceTheme = "dark"
	a.applySettings()
	if a.config.Settings.InterfaceTheme != "dark" {
		t.Fatal("apply did not persist theme")
	}
	a.prepareSettings()
	a.features.settingsDraft.InterfaceTheme = "light"
	tt.Frame()
	pixel = tt.Image().RGBAAt(799, 1)
	lightBackground := professionalTheme(false).Background
	if pixel.R != lightBackground.R || pixel.G != lightBackground.G || pixel.B != lightBackground.B {
		t.Fatalf("light preview inherited saved dark: %+v", pixel)
	}
	snapshot(t, tt, "settings-preview-light.png")
	a.closeSettings()
	if a.config.Settings.InterfaceTheme != "dark" {
		t.Fatal("cancel of light preview changed dark preference")
	}
}

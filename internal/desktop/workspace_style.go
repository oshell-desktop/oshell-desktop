package desktop

import "github.com/egoist/mygo/ui"

const (
	detailRowHeight = 24
	detailBarHeight = 32
)

func windowHeading(c *ui.Context, title string) ui.Element {
	return ui.Text(c, title).FontSize(14).FontWeight(600)
}

// One palette for application windows, dialogs and auxiliary panels. Terminal
// colors remain controlled by the saved terminal scheme.
func professionalTheme(dark bool) *ui.Theme {
	t := ui.LightTheme()
	t.Background = ui.Hex("#f5f6f8")
	t.Panel = ui.Hex("#ffffff")
	t.Surface = ui.Hex("#eceff4")
	t.SurfaceHover = ui.Hex("#e1e7f0")
	t.SurfacePressed = ui.Hex("#d5deec")
	t.Border = ui.Hex("#d9e0e9")
	t.Text = ui.Hex("#283344")
	t.TextMuted = ui.Hex("#647184")
	t.Accent = ui.Hex("#426da9")
	t.AccentHover = ui.Hex("#365f99")
	t.AccentPressed = ui.Hex("#2c5084")
	t.Selection = ui.Hex("#dfe9f7")
	if dark {
		t = ui.DarkTheme()
		t.Background = ui.Hex("#20252d")
		t.Panel = ui.Hex("#252c36")
		t.Surface = ui.Hex("#2b3340")
		t.SurfaceHover = ui.Hex("#354153")
		t.SurfacePressed = ui.Hex("#404e63")
		t.Border = ui.Hex("#3b4657")
		t.Text = ui.Hex("#dce3ed")
		t.TextMuted = ui.Hex("#a4b0c1")
		t.Accent = ui.Hex("#8aafe2")
		t.AccentHover = ui.Hex("#a1c1ec")
		t.AccentPressed = ui.Hex("#739ace")
		t.AccentText = ui.Hex("#152338")
		t.Selection = ui.Hex("#31435d")
		t.Inverse, t.InverseText = ui.Hex("#354153"), t.Text
	}
	t.Focus = t.Accent.Alpha(.65)
	t.Radius, t.Spacing, t.FontSize = 3, 3, 12
	t.CompactLists = true
	return t
}

func panelColor(c *ui.Context) ui.Color {
	if fill := c.Theme().Panel; fill != ui.Transparent {
		return fill
	}
	return c.Theme().Background
}

func panelSurface(c *ui.Context) ui.Element {
	return ui.Column(c).Padding(8).Gap(6).
		Background(panelColor(c)).Border(1, c.Theme().Border)
}

func pageHeading(c *ui.Context, title, detail string, icon *ui.SVG) {
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, icon).Size(16, 16).FontSize(16).TextColor(c.Theme().TextMuted)
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			windowHeading(c, title)
			if detail != "" {
				ui.Text(c, detail).FontSize(12).TextColor(c.Theme().TextMuted).Wrap()
			}
		})
	})
}

func sectionTitle(c *ui.Context, title string) ui.Element {
	return ui.Text(c, title).FontSize(13).FontWeight(600).TextColor(c.Theme().Text).Margin(4, 0, 2, 0)
}

func dialogActions(c *ui.Context) ui.Element {
	return ui.Row(c).Justify(ui.End).AlignItems(ui.Center).Gap(6).
		Padding(6, 0, 0, 0).BorderWidth(1, 0, 0, 0).BorderColor(c.Theme().Border)
}

func dialogBody(c *ui.Context, preferredWidth float32) ui.Element {
	w, _ := c.Size()
	return ui.Column(c).Width(min(preferredWidth, max(1, w-64))).MinWidth(0)
}

// Retain MyGo's modal focus, backdrop and Escape handling; only the panel
// presentation is ours. Fixed-size dialog content still respects window bounds.
func desktopModal(c *ui.Context, open *bool, content func()) ui.Element {
	w, h := c.Size()
	return ui.DialogBase(c, open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(10, 16, 26, .48))
		panel.Padding(12).Gap(6).Radius(3).Background(panelColor(c)).
			Border(1, c.Theme().Border).MaxWidth(max(1, w-24)).MaxHeight(max(1, h-24)).
			Shadow(0, 8, 24, 0, ui.RGBA(0, 0, 0, .22))
		ui.Scroll(c).MaxWidth(max(1, w-64)).MaxHeight(max(1, h-64)).Label("对话框内容").Children(content)
	})
}

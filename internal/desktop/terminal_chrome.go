package desktop

import "github.com/egoist/mygo/ui"

var terminalFlame = ui.Hex("#ff8550")

// The divider follows the terminal palette; child UI keeps the window theme.
// Keep the native splitter's larger invisible drag target and keyboard support.
func (a *App) terminalSplit(c *ui.Context, size *float32, vertical bool, first, second func()) {
	original := c.Theme()
	chrome := *original
	scheme := a.config.Settings.Scheme()
	background, foreground := ui.Hex(scheme.Background), ui.Hex(scheme.Foreground)
	chrome.Border = background.Mix(foreground, .12)
	chrome.Accent = chrome.Border
	wrap := func(child func()) func() { return func() { c.SetTheme(original); child(); c.SetTheme(&chrome) } }
	c.SetTheme(&chrome)
	if vertical {
		ui.SplitVertical(c, size, wrap(first), wrap(second)).Fill().Background(background)
	} else {
		ui.Split(c, size, wrap(first), wrap(second)).Fill().Background(background)
	}
	c.SetTheme(original)
}

func (a *App) showPaneFocusStripe(t *tab, p *pane) bool {
	return a.activeTab() == t && t.active == p && len(a.visiblePanes()) > 1
}

func tabTitleButton(c *ui.Context, label, title string, selected, unread bool) ui.Element {
	return ui.Button(c, "").Label(label).Tooltip(label).Height(25).Padding(3, 7).Gap(5).Radius(4).Background(ui.Transparent).BorderWidth(0).Children(func() {
		text := ui.Text(c, title).FontSize(12).SingleLine().TextColor(c.Theme().TextMuted)
		if selected {
			text.TextColor(c.Theme().Text)
		}
		if unread {
			text.TextColor(tabUnreadColor(c))
		}
		if selected || unread {
			text.Bold()
		}
	})
}

func terminalHeaderButton(c *ui.Context, label string, icon *ui.SVG, text string, foreground ui.Color) ui.Element {
	return ui.Button(c, "").Label(label).Tooltip(label).Height(22).Padding(2, 4).Gap(4).Radius(0).Background(ui.Transparent).BorderWidth(0).Children(func() {
		if icon != nil {
			ui.Icon(c, icon).FontSize(13).TextColor(foreground.Alpha(.75))
		}
		if text != "" {
			ui.Text(c, text).FontSize(12).SingleLine().TextColor(foreground.Alpha(.75))
		}
	})
}

func tabUnreadColor(c *ui.Context) ui.Color {
	if c.Theme().Dark {
		return ui.Hex("#FF9F0A")
	}
	return ui.Hex("#B85C00")
}

func tabBarButton(c *ui.Context, label string, icon *ui.SVG) ui.Element {
	button := ui.Button(c, "").Label(label).Tooltip(label).Width(25).Height(28).Padding(4).Radius(4).Background(ui.Transparent).BorderWidth(0)
	if button.Pressed() {
		button.Background(c.Theme().SurfacePressed)
	} else if button.Hovered() {
		button.Background(c.Theme().SurfaceHover)
	}
	return button.Children(func() {
		ui.Icon(c, icon).FontSize(15).TextColor(c.Theme().TextMuted)
	})
}

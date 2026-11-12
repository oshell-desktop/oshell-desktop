// SPDX-License-Identifier: GPL-3.0-only
package desktop

import "github.com/egoist/mygo/ui"

// Small, consistent outline icons; parsed once, shared across all frames.
func glyph(body string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.35" stroke-linecap="round" stroke-linejoin="round">` + body + `</svg>`))
}

var (
	catalogIcon     = glyph(`<rect x="2" y="3" width="16" height="6" rx="1.6"/><rect x="2" y="11" width="16" height="6" rx="1.6"/><path d="M5 6h2m-2 8h2"/>`)
	quickIcon       = glyph(`<path d="m12 2-9 10h6l-1 6 9-11h-6z"/>`)
	newSessionIcon  = glyph(`<rect x="2" y="3" width="16" height="14" rx="2"/><path d="M10 6v8m-4-4h8"/>`)
	groupsIcon      = glyph(`<rect x="2" y="7" width="14" height="10" rx="2"/><path d="M5 4h12q1 0 1 1v8"/>`)
	groupLayoutIcon = glyph(`<rect x="1" y="4" width="5" height="12" rx="1"/><rect x="8" y="4" width="4" height="12" rx="1"/><rect x="14" y="4" width="5" height="12" rx="1"/>`)
	paneLayoutIcon  = glyph(`<rect x="2" y="3" width="16" height="14" rx="2"/><path d="M10 3v14M2 10h16"/>`)
	toolsIcon       = glyph(`<path d="M11 4a5 5 0 0 0-6 6l-3 4a2 2 0 0 0 3 3l5-5a5 5 0 0 0 6-6l-3 3-3-3z"/>`)
	mcpIcon         = glyph(`<path d="M3 5h5a2.5 2.5 0 1 1 4 0h4v4a2.5 2.5 0 1 1 0 4v4H3v-4a2.5 2.5 0 1 0 0-4z"/>`)
	networkIcon     = glyph(`<circle cx="10" cy="10" r="8"/><ellipse cx="10" cy="10" rx="3.6" ry="8"/><path d="M2 10h16"/>`)
	propertiesIcon  = glyph(`<rect x="2" y="3" width="16" height="14" rx="2"/><path d="M2 7h16m-12 4h8m-5 3h5"/>`)
	gearIcon        = glyph(`<path d="m8 2-.6 2.1-1.5.9-2.1-.5-1.6 2.8 1.5 1.6v2.2l-1.5 1.6 1.6 2.8 2.1-.5 1.5.9L8 18h4l.6-2.1 1.5-.9 2.1.5 1.6-2.8-1.5-1.6V8.9l1.5-1.6-1.6-2.8-2.1.5-1.5-.9L12 2z"/><circle cx="10" cy="10" r="3"/>`)
	stopIcon        = glyph(`<circle cx="10" cy="10" r="7.5"/><rect x="7" y="7" width="6" height="6" rx="1"/>`)
	downloadIcon    = glyph(`<circle cx="10" cy="10" r="7.5"/><path d="M10 5.5v8m-3-3 3 3 3-3"/>`)
	moreIcon        = glyph(`<path d="M3 5h14M3 10h14M3 15h14"/>`)

	paletteIcon         = glyph(`<path d="M17.5 10a7.5 7.5 0 1 0-7.5 7.5h1.2a1.8 1.8 0 0 0 1.3-3l-.5-.5a1 1 0 0 1 .7-1.7h1.8a3 3 0 0 0 3-2.3z"/><circle cx="5.5" cy="9" r=".6"/><circle cx="8" cy="5.5" r=".6"/><circle cx="12" cy="5.5" r=".6"/><circle cx="15" cy="8.5" r=".6"/>`)
	keyboardIcon        = glyph(`<rect x="2" y="4" width="16" height="12" rx="2"/><path d="M5 7h1m2 0h1m2 0h1m2 0h1M5 10h1m2 0h1m2 0h1m2 0h1M6 13h8"/>`)
	clockIcon           = glyph(`<circle cx="10" cy="10" r="7.5"/><path d="M10 5v5H6"/>`)
	infoIcon            = glyph(`<circle cx="10" cy="10" r="7.5"/><path d="M10 9v5m0-8v.1"/>`)
	folderIcon          = glyph(`<path d="M2.5 5h5l1.5 2h8.5v9h-15z"/><path d="M2.5 5V3.5h5l1.5 2h7v1.5"/>`)
	sidebarIcon         = glyph(`<rect x="2" y="3" width="16" height="14" rx="2"/><path d="M7 3v14M4 6h1m-1 3h1m-1 3h1"/>`)
	collapseSidebarIcon = glyph(`<rect x="2" y="3" width="16" height="14" rx="2"/><path d="M7 3v14m6-10-3 3 3 3"/>`)
	syncIcon            = glyph(`<path d="M16.5 8a6.8 6.8 0 0 0-11.7-3L2.5 7.5m0-4v4h4M3.5 12a6.8 6.8 0 0 0 11.7 3l2.3-2.5m0 4v-4h-4"/>`)
	terminalIcon        = glyph(`<rect x="2" y="3.5" width="16" height="13" rx="2"/><path d="m5 7 3 3-3 3m5 0h4"/>`)
	plusIcon            = glyph(`<path d="M10 4v12M4 10h12"/>`)
	closeIcon           = glyph(`<path d="m6 6 8 8m0-8-8 8"/>`)
	settingsIcon        = glyph(`<path d="M4 5h12M4 10h12M4 15h12"/><rect x="6" y="3" width="3" height="4" rx="1"/><rect x="12" y="8" width="3" height="4" rx="1"/><rect x="7" y="13" width="3" height="4" rx="1"/>`)
	splitIcon           = glyph(`<rect x="2" y="3" width="16" height="14" rx="2"/><path d="M10 3v14m0-7h8"/>`)
	linkIcon            = glyph(`<path d="m8 12 4-4M7 13l-1 1a3 3 0 0 1-4-4l4-4a3 3 0 0 1 4 0m0 8a3 3 0 0 0 4 0l4-4a3 3 0 0 0-4-4l-1 1"/>`)
	chevronIcon         = glyph(`<path d="m6 8 4 4 4-4"/>`)
	arrowUpIcon         = glyph(`<path d="m5 10 5-5 5 5m-5-5v11"/>`)
	searchIcon          = glyph(`<circle cx="8.5" cy="8.5" r="5"/><path d="m12 12 5 5"/>`)
	recordIcon          = glyph(`<circle cx="10" cy="10" r="7"/><circle cx="10" cy="10" r="2.5"/>`)
	editIcon            = glyph(`<path d="m12.5 3.5 4 4M3.5 16.5l1-4 9-9a2.1 2.1 0 0 1 3 3l-9 9z"/>`)
)

func toolButton(c *ui.Context, label string, icon *ui.SVG, text string) ui.Element {
	return ui.Button(c, "").Label(label).Tooltip(label).Height(26).Padding(2, 6).Gap(4).Radius(3).Children(func() {
		if icon != nil {
			ui.Icon(c, icon).FontSize(16).TextColor(c.Theme().TextMuted)
		}
		if text != "" {
			ui.Text(c, text).FontSize(12).SingleLine()
		}
	})
}
func flatButton(c *ui.Context, label string, icon *ui.SVG, text string) ui.Element {
	b := toolButton(c, label, icon, text).Height(detailRowHeight).BorderWidth(0)
	switch {
	case b.Pressed():
		b.Background(c.Theme().SurfacePressed)
	case b.Hovered() || b.FocusVisible():
		b.Background(c.Theme().SurfaceHover)
	default:
		b.Background(ui.Transparent)
	}
	return b
}

func rowIconButton(c *ui.Context, label string, icon *ui.SVG, foreground ui.Color) ui.Element {
	button := ui.ButtonBase(c).Role(ui.RoleButton).Label(label).Tooltip(label).
		Size(28, 28).Radius(5).Cursor(ui.CursorPointer)
	if button.Pressed() {
		button.Background(foreground.Alpha(.20))
	} else if button.Hovered() || button.FocusVisible() {
		button.Background(foreground.Alpha(.12))
	}
	return button.Children(func() {
		ui.Icon(c, icon).FontSize(16).TextColor(foreground)
	})
}

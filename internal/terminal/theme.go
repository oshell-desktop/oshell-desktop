package terminal

import (
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/terminal/internal/vt"
)

// Theme is the colors of a terminal, which programs may change (OSC 4,
// 10, 11, 12) until the terminal resets. GhosttyTheme returns Ghostty's
// themes.
type Theme struct {
	Foreground, Background ui.Color
	// Cursor colors the cursor: the foreground when zero. CursorText
	// colors the text under a block cursor: the background when zero.
	Cursor, CursorText ui.Color
	// Selection highlights the selected text: the foreground, a third
	// opaque, when zero. SelectionText colors the selected text: its own
	// colors when zero.
	Selection, SelectionText ui.Color
	// Palette holds the 16 colors of programs: black, red, green, yellow,
	// blue, magenta, cyan and white, then their bright kinds.
	Palette [16]ui.Color
}

// DarkTheme returns the colors of terminals in dark windows: the window's
// background and text, with Visual Studio Code's palette.
func DarkTheme() *Theme {
	ut := ui.DarkTheme()
	return &Theme{
		Foreground: ut.Text, Background: ut.Background,
		Palette: [16]ui.Color{
			ui.Hex("#000000"), ui.Hex("#cd3131"), ui.Hex("#0dbc79"), ui.Hex("#e5e510"),
			ui.Hex("#2472c8"), ui.Hex("#bc3fbc"), ui.Hex("#11a8cd"), ui.Hex("#e5e5e5"),
			ui.Hex("#666666"), ui.Hex("#f14c4c"), ui.Hex("#23d18b"), ui.Hex("#f5f543"),
			ui.Hex("#3b8eea"), ui.Hex("#d670d6"), ui.Hex("#29b8db"), ui.Hex("#ffffff"),
		},
	}
}

// LightTheme returns the colors of terminals in light windows: the
// window's background and text, with Visual Studio Code's palette.
func LightTheme() *Theme {
	ut := ui.LightTheme()
	return &Theme{
		Foreground: ut.Text, Background: ut.Background,
		Palette: [16]ui.Color{
			ui.Hex("#000000"), ui.Hex("#cd3131"), ui.Hex("#00bc00"), ui.Hex("#949800"),
			ui.Hex("#0451a5"), ui.Hex("#bc05bc"), ui.Hex("#0598bc"), ui.Hex("#555555"),
			ui.Hex("#666666"), ui.Hex("#cd3131"), ui.Hex("#14ce14"), ui.Hex("#b5ba00"),
			ui.Hex("#0451a5"), ui.Hex("#bc05bc"), ui.Hex("#0598bc"), ui.Hex("#a5a5a5"),
		},
	}
}

var themes = [2]*Theme{LightTheme(), DarkTheme()}

func defaultTheme(dark bool) *Theme {
	if dark {
		return themes[1]
	}
	return themes[0]
}

// dark reports whether the theme's background is dark.
func (th *Theme) dark() bool {
	b := th.Background
	return 299*int(b.R)+587*int(b.G)+114*int(b.B) < 128*1000
}

// palette derives extended colors from the theme in Lab, matching OShell's
// default SwiftTerm strategy. The first 16 ANSI colors stay unchanged.
func (th *Theme) palette() *[256]vt.RGB {
	var p [256]vt.RGB
	for i, c := range th.Palette {
		p[i] = rgb(c)
	}
	fillLabPalette(&p, rgb(th.Background), rgb(th.Foreground))
	return &p
}

// apply makes the theme the terminal's defaults.
func (th *Theme) apply(t *vt.Terminal) {
	t.SetDerivedPalette(fillLabPalette)
	var cursor *vt.RGB
	if th.Cursor.A > 0 {
		c := rgb(th.Cursor)
		cursor = &c
	}
	t.SetColors(rgb(th.Foreground), rgb(th.Background), cursor, th.palette())
}

func rgb(c ui.Color) vt.RGB { return vt.RGB{R: c.R, G: c.G, B: c.B} }

func color(c vt.RGB) ui.Color { return ui.RGB(c.R, c.G, c.B) }

// SPDX-License-Identifier: GPL-3.0-only
package desktop

import (
	"embed"
	"runtime"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"oshell-desktop/internal/terminal"
)

//go:embed assets/fonts/*.ttf
var builtinFonts embed.FS
var fontRegistration struct {
	sync.Once
	err error
}

func registerFonts() error {
	fontRegistration.Do(func() {
		files, _ := builtinFonts.ReadDir("assets/fonts")
		for _, f := range files {
			data, e := builtinFonts.ReadFile("assets/fonts/" + f.Name())
			if e == nil {
				e = ui.RegisterFont(data, "")
			}
			if e != nil && fontRegistration.err == nil {
				fontRegistration.err = e
			}
		}
	})
	return fontRegistration.err
}
func terminalFamily(name string) string {
	if name == "" {
		name = "monospace"
		if runtime.GOOS == "darwin" {
			name = "Menlo, monospace"
		}
	} else {
		// A missing saved family must fall back to text, before the symbol
		// font becomes the primary face and delegates ASCII to the UI font.
		name += ", monospace"
	}
	if !strings.Contains(name, "Symbols Nerd Font Mono") {
		name += ", Symbols Nerd Font Mono"
	}
	return name
}
func terminalTheme(s core.ColorScheme) *terminal.Theme {
	t := &terminal.Theme{Foreground: ui.Hex(s.Foreground), Background: ui.Hex(s.Background), Cursor: ui.Hex(s.Cursor), CursorText: ui.Hex(s.CursorText), Selection: ui.Hex(s.Selection), SelectionText: ui.Hex(s.SelectionText)}
	for i, c := range s.ANSI {
		t.Palette[i] = ui.Hex(c)
	}
	return t
}

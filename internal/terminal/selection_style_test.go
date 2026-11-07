package terminal

import (
	imagecolor "image/color"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

// SF-I02 / SF-A02: selection must cover the cross-line tail, recolor glyphs,
// and preserve copied text rather than copying the visual highlight padding.
func TestPresetSelectionStyleAndCopy(t *testing.T) {
	loadLib(t)
	for _, scheme := range core.ColorPresets()[:2] {
		t.Run(scheme.ID, func(t *testing.T) {
			th := &Theme{Foreground: ui.Hex(scheme.Foreground), Background: ui.Hex(scheme.Background), Selection: ui.Hex(scheme.Selection), SelectionText: ui.Hex(scheme.SelectionText)}
			term, err := New(Options{Conn: newPipe(), NoBlink: true, Font: Font{Size: 18}, Theme: th})
			if err != nil {
				t.Fatal(err)
			}
			defer term.Close()
			tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill().AutoFocus() }, 1000, 130)
			first := "WARNING! The remote SSH server rejected X11 forwarding request."
			second := "Last login: fixture from 192.0.2.24"
			term.Feed([]byte("\x1b[?25l" + first + "\r\n" + second + "\r\nuser@host ~]$ "))
			term.mu.Lock()
			selected := term.term.SelectSearchRange(0, 20, 1, len(second))
			term.mu.Unlock()
			if !selected {
				t.Fatal("cross-line fixture selection failed")
			}
			tt.Frame()
			selection := th.Selection
			graphicColor(t, term, tt, 70, 0, imagecolor.RGBA{selection.R, selection.G, selection.B, 255})
			bg := th.Background
			graphicColor(t, term, tt, 70, 1, imagecolor.RGBA{bg.R, bg.G, bg.B, 255})
			// The second line is selected: find solid glyph interiors in its
			// expected foreground color, not the unselected ANSI foreground.
			v := term.v
			pixels := tt.Image()
			ink := 0
			for y := v.oy + v.cellH; y < v.oy+2*v.cellH; y++ {
				for x := v.ox; x < v.ox+len(second)*v.cellW; x++ {
					p := pixels.RGBAAt(x, y)
					if abs(int(p.R)-int(th.SelectionText.R)) < 8 && abs(int(p.G)-int(th.SelectionText.G)) < 8 && abs(int(p.B)-int(th.SelectionText.B)) < 8 {
						ink++
					}
				}
			}
			if ink < 20 {
				t.Fatal("selected glyphs did not use contrasting foreground")
			}
			tt.Command("copy")
			if got := tt.Clipboard(); got != first[20:]+"\n"+second || strings.Contains(got, "user@host") {
				t.Fatalf("selection style changed copied text: %q", got)
			}
			save(t, tt, "selection-"+scheme.ID)
		})
	}
}

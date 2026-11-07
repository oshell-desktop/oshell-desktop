package desktop

import (
	"github.com/egoist/mygo/ui"
	"image"
	"oshell-desktop/internal/core"
	"testing"
)

func longestFlameLine(img *image.RGBA) int {
	longest := 0
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		run := 0
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			c := img.RGBAAt(x, y)
			if c.R == 255 && c.G == 133 && c.B == 80 {
				run++
				longest = max(longest, run)
			} else {
				run = 0
			}
		}
	}
	return longest
}
func TestTerminalChromeSelectionZoomAndThemeRestoration(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	first, _ := operatorPane(t, a, "first")
	second, _ := operatorPane(t, a, "second")
	first.term.SetTheme(terminalTheme(a.config.Settings.Scheme()), nil)
	second.term.SetTheme(terminalTheme(a.config.Settings.Scheme()), nil)
	tab := a.tabs[0]
	tab.root = &node{id: core.NewID(), first: tab.root, second: a.tabs[1].root, size: 333}
	a.tabs = a.tabs[:1]
	a.ensureGroups()
	tt := ui.NewTester(a.view, 1000, 700)
	if !a.showPaneFocusStripe(tab, first) || a.showPaneFocusStripe(tab, second) {
		t.Fatal("focus stripe selection incorrect")
	}
	if longestFlameLine(tt.Image()) < 300 {
		t.Fatal("active split stripe not rendered")
	}
	snapshot(t, tt, "terminal-chrome-light.png")
	a.togglePaneZoom()
	tt.Frame()
	if a.showPaneFocusStripe(tab, first) {
		t.Fatal("zoom kept multi-terminal stripe")
	}
	if n := longestFlameLine(tt.Image()); n != 0 {
		t.Fatalf("zoom should hide the orange split focus stripe; tabs use the blue application accent: %d", n)
	}
	a.togglePaneZoom()
	a.cyclePane(1)
	tt.Frame()
	if a.showPaneFocusStripe(tab, first) || !a.showPaneFocusStripe(tab, second) {
		t.Fatal("stripe did not follow focus")
	}
	a.config.Settings.InterfaceTheme = "dark"
	tt.Frame()
	snapshot(t, tt, "terminal-chrome-dark.png")
	if longestFlameLine(tt.Image()) < 300 {
		t.Fatal("dark theme lost stripe")
	}
	if tab.root.size != 333 || tab.active != second {
		t.Fatal("theme changed layout or active pane")
	}
	// Split styling is scoped; it must not leak terminal colors into the window.
	var windowColor ui.Color
	ui.NewTester(func(c *ui.Context) {
		a.theme(c)
		before := *c.Theme()
		a.terminalSplit(c, new(float32), false, func() {}, func() {})
		windowColor = c.Theme().Background
		if c.Theme().Text != before.Text {
			t.Fatal("split leaked text theme")
		}
	}, 400, 200)
	if windowColor != professionalTheme(true).Background {
		t.Fatal("split leaked background theme")
	}
}

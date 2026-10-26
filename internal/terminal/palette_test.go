package terminal

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/terminal/internal/vt"
)

func TestLabColorReferenceValues(t *testing.T) {
	// D65/sRGB reference coordinates, independently of the palette traversal.
	for _, tc := range []struct {
		rgb vt.RGB
		lab labColor
	}{
		{vt.RGB{}, labColor{}},
		{vt.RGB{R: 255, G: 255, B: 255}, labColor{100, 0, 0}},
		{vt.RGB{R: 255}, labColor{53.2408, 80.0925, 67.2032}},
		{vt.RGB{G: 255}, labColor{87.7347, -86.1827, 83.1793}},
		{vt.RGB{B: 255}, labColor{32.2970, 79.1875, -107.8602}},
	} {
		got := rgbToLab(tc.rgb)
		if math.Abs(got.l-tc.lab.l) > 0.001 || math.Abs(got.a-tc.lab.a) > 0.001 || math.Abs(got.b-tc.lab.b) > 0.001 {
			t.Fatalf("%v: Lab %v, want %v", tc.rgb, got, tc.lab)
		}
		if got.rgb() != tc.rgb {
			t.Fatalf("RGB roundtrip: %v -> %v", tc.rgb, got.rgb())
		}
	}
	if got := (labColor{l: 50}).rgb(); got != (vt.RGB{R: 119, G: 119, B: 119}) {
		t.Fatalf("perceptual midpoint = %v, want 119 gray", got)
	}
}

func TestThemeLabPaletteDynamicColorsAndQueries(t *testing.T) {
	loadLib(t)
	for _, fragmented := range []bool{false, true} {
		t.Run(fmt.Sprint("fragmented=", fragmented), func(t *testing.T) {
			var replies bytes.Buffer
			term, err := vt.NewTerminal(80, 24, vt.Effects{WritePTY: func(p []byte) { replies.Write(p) }})
			if err != nil {
				t.Fatal(err)
			}
			defer term.Free()
			if !term.SetDerivedPalette(fillLabPalette) {
				if os.Getenv("OSHELL_REQUIRE_WIDTH_EXTENSION") == "1" {
					t.Fatal("required OShell dynamic palette extension is absent")
				}
				t.Skip("requires OShell native extension ABI 2")
			}
			rs, err := vt.NewRenderState()
			if err != nil {
				t.Fatal(err)
			}
			defer rs.Free()
			write := func(text string) {
				if fragmented {
					for _, b := range []byte(text) {
						term.Write([]byte{b})
					}
				} else {
					term.Write([]byte(text))
				}
			}
			th := *DarkTheme()
			th.apply(term)
			// Changing foreground clears prior application palette overrides,
			// including first-16 entries. The following query is in the SAME
			// input block; repaint-time updates would answer with stale colors.
			write("\x1b]4;1;#010203;231;#040506\x07\x1b]10;#e0c090\x07\x1b]4;231;?\x07")
			th.Foreground = ui.Hex("#e0c090")
			rs.Update(term)
			if rs.Colors().Palette != *th.palette() {
				t.Fatal("dynamic foreground left stale colors or OSC overrides")
			}
			if !strings.Contains(replies.String(), "4;231;rgb:e0e0/c0c0/9090") {
				t.Fatalf("query was not synchronous: %q", replies.String())
			}
			// A light background reverses the luminance anchors immediately.
			write("\x1b]11;#fff5ee\x1b\\")
			th.Background = ui.Hex("#fff5ee")
			rs.Update(term)
			if rs.Colors().Palette != *th.palette() {
				t.Fatal("dynamic background did not recompute light-theme anchors")
			}
			// Queries, invalid colors and cursor-only changes must not rebuild
			// the palette or discard an application's explicit colors.
			write("\x1b]4;127;#010203\x07\x1b]10;?\x07\x1b]11;invalid\x07\x1b]12;#123456\x07")
			rs.Update(term)
			if rs.Colors().Palette[127] != (vt.RGB{R: 1, G: 2, B: 3}) {
				t.Fatal("query/cursor/invalid color unexpectedly reset palette")
			}
			write("\x1b]104;127\x07")
			rs.Update(term)
			if rs.Colors().Palette != *th.palette() {
				t.Fatal("OSC 104 did not restore latest derived palette")
			}
			// Even assigning the same foreground resets OSC overrides in Swift.
			write("\x1b]4;127;#010203\x07\x1b]10;#e0c090\x07")
			rs.Update(term)
			if rs.Colors().Palette != *th.palette() {
				t.Fatal("same-color assignment failed to clear overrides")
			}
			// Applying a user theme clears dynamic foreground/background and
			// palette overrides instead of leaving the old program colors active.
			write("\x1b]4;127;#010203\x07")
			LightTheme().apply(term)
			rs.Update(term)
			colors := rs.Colors()
			if colors.Palette != *LightTheme().palette() || colors.Foreground != rgb(LightTheme().Foreground) || colors.Background != rgb(LightTheme().Background) {
				t.Fatal("theme application retained old program colors")
			}
			if colors.HasCursor {
				t.Fatal("theme application retained old program cursor color")
			}
			// OSC 10 may carry foreground, background and cursor together.
			write("\x1b]10;#eeccaa;#142536;#2468ac\x07")
			combined := *LightTheme()
			combined.Foreground, combined.Background = ui.Hex("#eeccaa"), ui.Hex("#142536")
			rs.Update(term)
			colors = rs.Colors()
			if colors.Palette != *combined.palette() || !colors.HasCursor || colors.Cursor != rgb(ui.Hex("#2468ac")) {
				t.Fatal("multi-color OSC 10 did not apply all targets in order")
			}
			write("\x1b]4;127;#010203\x07\x1b]110\x07\x1b]111\x07")
			rs.Update(term)
			if rs.Colors().Palette != *LightTheme().palette() {
				t.Fatal("existing OSC 110/111 reset support left stale palette")
			}
		})
	}
}

func TestThemeLabPaletteAnchorsAndLightTheme(t *testing.T) {
	th := *DarkTheme()
	th.Background, th.Foreground = ui.Hex("#172334"), ui.Hex("#eadac4")
	p := th.palette()
	for i, c := range th.Palette {
		if p[i] != rgb(c) {
			t.Fatalf("ANSI %d changed", i)
		}
	}
	// SwiftTerm's cube traverses red, green, blue, with base8 anchors in
	// bit order R=1, G=2, B=4 (not the RGB ordering of xterm's fixed cube).
	for _, tc := range []struct{ index, anchor int }{{196, 1}, {46, 2}, {226, 3}, {21, 4}, {201, 5}, {51, 6}} {
		if p[tc.index] != rgb(th.Palette[tc.anchor]) {
			t.Fatalf("corner %d = %v, want ANSI %d", tc.index, p[tc.index], tc.anchor)
		}
	}
	if p[16] != rgb(th.Background) || p[231] != rgb(th.Foreground) {
		t.Fatal("cube ignored foreground/background anchors")
	}
	for i := 233; i < 256; i++ {
		if rgbToLab(p[i]).l <= rgbToLab(p[i-1]).l {
			t.Fatalf("non-monotonic gray ramp at %d", i)
		}
	}
	th.Background, th.Foreground = th.Foreground, th.Background
	if light := th.palette(); *light != *p {
		t.Fatal("light theme did not preserve dark-to-light anchor ordering")
	}
	th.Palette[1] = ui.Hex("#ff4070")
	if next := th.palette(); next[196] == p[196] || next[127] == p[127] {
		t.Fatal("theme change did not recompute extended colors")
	}
}

func TestThemeLabPaletteAppliedToNativeTerminal(t *testing.T) {
	loadLib(t)
	term, err := vt.NewTerminal(80, 24, vt.Effects{})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Free()
	rs, err := vt.NewRenderState()
	if err != nil {
		t.Fatal(err)
	}
	defer rs.Free()
	for _, th := range []*Theme{DarkTheme(), LightTheme()} {
		th.apply(term)
		if err := rs.Update(term); err != nil {
			t.Fatal(err)
		}
		if got := rs.Colors().Palette; got != *th.palette() {
			t.Fatal("native terminal did not receive theme-derived palette")
		}
		term.Write([]byte("\x1b]4;196;#010203\x07"))
		rs.Update(term)
		if rs.Colors().Palette[196] != (vt.RGB{R: 1, G: 2, B: 3}) {
			t.Fatal("application palette override lost")
		}
		term.Write([]byte("\x1b]104;196\x07"))
		rs.Update(term)
		if rs.Colors().Palette[196] != th.palette()[196] {
			t.Fatal("OSC reset did not restore theme-derived color")
		}
	}
}

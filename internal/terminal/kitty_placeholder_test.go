package terminal

import (
	"bytes"
	"fmt"
	"image"
	imagecolor "image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/terminal/internal/vt"
)

func placeholderText(marks ...int) string {
	r := []rune{kittyPlaceholderScalar}
	for _, n := range marks {
		r = append(r, kittyPlaceholderDiacritics[n])
	}
	return string(r)
}

func TestKittyPlaceholderDecoderInheritance(t *testing.T) {
	style := vt.Style{FG: vt.Color{Kind: 2, RGB: vt.RGB{R: 1, G: 2, B: 3}}, UnderlineColor: vt.Color{Kind: 1, Index: 7}}
	first, ok := decodeKittyPlaceholder([]rune("\U0010eeee\u030d\u030e\u0310"), style, 4, nil)
	if !ok || first.imageRow != 1 || first.imageCol != 2 || first.imageID != 0x03010203 || first.placementID != 7 {
		t.Fatalf("literal three-mark identity: %+v", first)
	}
	for _, tc := range []struct {
		name, chars        string
		col                int
		style              vt.Style
		row, imageCol, msb int
	}{
		{"none", placeholderText(), 5, style, 1, 3, 3},
		{"row", placeholderText(1), 5, style, 1, 3, 3},
		{"row changed", placeholderText(2), 5, style, 2, 0, 0},
		{"row col", placeholderText(1, 3), 5, style, 1, 3, 3},
		{"col changed", placeholderText(1, 4), 5, style, 1, 4, 0},
		{"gap", placeholderText(), 6, style, 0, 0, 0},
		{"unknown marks ignored", "\U0010eeee\u0301\u030d\u0301", 5, style, 1, 3, 3},
		{"explicit clamp", placeholderText(2, 4, 296, 1), 5, style, 2, 4, 296},
		{"underline reset", placeholderText(), 5, vt.Style{FG: style.FG}, 0, 0, 0},
		{"foreground reset", placeholderText(), 5, vt.Style{UnderlineColor: style.UnderlineColor}, 0, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := decodeKittyPlaceholder([]rune(tc.chars), tc.style, tc.col, &first)
			if !ok || got.imageRow != tc.row || got.imageCol != tc.imageCol || got.msb != tc.msb || got.imageID>>24 != uint32(min(tc.msb, 255)) {
				t.Fatalf("got %+v, want row=%d col=%d msb=%d", got, tc.row, tc.imageCol, tc.msb)
			}
		})
	}
	if _, ok := decodeKittyPlaceholder([]rune("x\u0305"), style, 0, nil); ok {
		t.Fatal("ordinary text decoded as an image")
	}
	if len(kittyPlaceholderDiacritics) != 297 || len(kittyPlaceholderDiacriticIndex) != 297 {
		t.Fatal("protocol diacritic table lost entries")
	}
}

func TestTerminalKittyPlaceholderRendering(t *testing.T) {
	loadLib(t)
	term, err := New(Options{Conn: newPipe(), NoBlink: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { term.Close() })
	tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill() }, 400, 220)
	tt.Frame()
	term.Feed([]byte("\x1b[?25l"))
	w, h := term.v.cellW*3, term.v.cellH*2
	pixels := make([]byte, w*h*3)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 3
			pixels[i], pixels[i+1] = 255, 0
			if y >= h/2 {
				pixels[i], pixels[i+1] = 0, 255
			}
		}
	}
	// Virtual images ignore raster crop and z; foreground and underline use
	// original numeric colors, even under inverse, bold and invisible SGR.
	feedGraphic(term, fmt.Sprintf("a=T,U=1,f=24,s=%d,v=%d,i=66051,p=7,c=3,r=2,z=-2147483648,x=999", w, h), pixels)
	term.Feed([]byte("\x1b[H\x1b[38;2;1;2;3;58;5;7;1;7;8m" + placeholderText(0, 0) + placeholderText() + placeholderText() + "\x1b[2;1H" + placeholderText(1, 0) + placeholderText() + placeholderText()))
	tt.Frame()
	red, green := imagecolor.RGBA{255, 0, 0, 255}, imagecolor.RGBA{0, 255, 0, 255}
	for col := 0; col < 3; col++ {
		graphicColor(t, term, tt, col, 0, red)
		graphicColor(t, term, tt, col, 1, green)
	}
	if len(term.v.lines[0].placeholders) != 3 || len(term.v.lines[0].runs) != 0 {
		t.Fatal("placeholder text leaked into glyph runs")
	}
	old := term.v.graphics[0].Image
	// A transmission changes the current protocol image without replacing the
	// virtual placement. An ordinary retained placement would keep old pixels.
	for i := range pixels {
		pixels[i] = 0
	}
	for i := 2; i < len(pixels); i += 3 {
		pixels[i] = 255
	}
	term.Feed([]byte("\x1b[?2026h"))
	feedGraphic(term, fmt.Sprintf("a=t,f=24,s=%d,v=%d,i=66051", w, h), pixels)
	tt.Frame()
	graphicColor(t, term, tt, 1, 0, red)
	term.Feed([]byte("\x1b[?2026l"))
	tt.Frame()
	graphicColor(t, term, tt, 1, 0, imagecolor.RGBA{0, 0, 255, 255})
	if term.v.graphics[0].Image == old {
		t.Fatal("placeholder retained obsolete protocol image")
	}
	// Deleting the placement suppresses its image but keeps invisible cells.
	feedGraphic(term, "a=d,d=i,i=66051,p=7", nil)
	tt.Frame()
	if len(term.v.graphics) != 0 || len(term.v.graphicBitmaps) != 0 || len(term.v.lines[0].placeholders) != 3 {
		t.Fatal("delete lost cells or retained image")
	}
	term.Feed([]byte("\x1b[0m\x1b[HABC"))
	tt.Frame()
	if len(term.v.lines[0].placeholders) != 0 {
		t.Fatal("overwriting cells retained placeholder cache")
	}
}

func TestTerminalKittyPlaceholderMissingImage(t *testing.T) {
	loadLib(t)
	term, err := New(Options{Conn: newPipe(), NoBlink: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { term.Close() })
	tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill() }, 320, 180)
	term.Feed([]byte("\x1b[?25l\x1b[38;5;7m" + strings.Repeat(placeholderText(), 3)))
	tt.Frame()
	line := term.v.lines[0]
	if len(line.placeholders) != 3 || len(line.runs) != 0 || len(term.v.graphics) != 0 {
		t.Fatal("missing image did not leave blank text cells")
	}
	for i, p := range line.placeholders {
		if p.imageCol != i || p.imageID != 7 {
			t.Fatal("indexed identity/inheritance", p)
		}
	}
}

func TestKittyPlaceholderClipFitOffsetsAndLayers(t *testing.T) {
	red, green, blue := imagecolor.RGBA{255, 0, 0, 255}, imagecolor.RGBA{0, 255, 0, 255}, imagecolor.RGBA{0, 0, 255, 255}
	for _, scale := range []float32{1, 2} {
		for _, mode := range []vt.LineMode{vt.LineSingle, vt.LineDoubleWidth, vt.LineDoubleTop, vt.LineDoubleBottom} {
			t.Run(fmt.Sprintf("%g/%d", scale, mode), func(t *testing.T) {
				// A 10x20 image fitted inside 30x40 leaves five pixels of
				// horizontal letterboxing. X=2 moves right; Y=3 moves up.
				g := stripeClipFixture()
				g.Image = image.NewNRGBA(image.Rect(0, 0, 10, 20))
				for y := 0; y < 20; y++ {
					for x := 0; x < 10; x++ {
						c := imagecolor.NRGBA{255, 0, 0, 255}
						if y >= 10 {
							c = imagecolor.NRGBA{0, 255, 0, 255}
						}
						g.Image.SetNRGBA(x, y, c)
					}
				}
				g.Source, g.Clipped, g.Virtual = g.Image.Rect, false, true
				g.Columns, g.Rows, g.X, g.Y, g.PlacementID, g.Z = 3, 2, 2, 3, 7, -100
				v := &view{cols: 6, rows: 4, cellW: 10, cellH: 20, scale: scale, graphics: []vt.Graphic{g}, lines: []rowCache{
					{mode: mode, placeholders: []kittyPlaceholder{{col: 1, imageCol: 1, placementID: 7}}},
				}}
				v.prepareGraphics()
				tt := ui.NewTester(func(c *ui.Context) {
					ui.Box(c).Fill().Draw(func(p *ui.Painter, r ui.Rect) {
						p.Fill(r, ui.RGB(0, 0, 255), 0)
						v.drawGraphics(p, 3, 5, 1)
						v.drawGraphics(p, 3, 5, 2)
						v.drawKittyPlaceholders(p, 3, 5)
						v.drawGraphics(p, 3, 5, 3)
					})
				}, 100, 100)
				tt.Frame()
				sx, _, _ := lineTransform(mode, 20)
				at := func(x, y int, want imagecolor.RGBA) {
					checkStripePixel(t, tt, int(float32(x)*sx/scale), int(float32(y)/scale), want)
				}
				at(5, 5, blue)
				at(25, 5, blue)
				at(15, 25, blue)
				at(15, 5, red)
				if mode == vt.LineSingle || mode == vt.LineDoubleWidth {
					at(15, 19, green)
				}
				// The explicit placement ID must match, even when another
				// virtual record has the same image and geometry.
				v.lines[0].placeholders[0].placementID = 8
				tt.Frame()
				at(15, 5, blue)
			})
		}
	}
}

func TestTerminalKittyPlaceholderBidiHistoryAndLateImage(t *testing.T) {
	term, tt, _ := bidiViewFixture(t)
	tt.Frame()
	// Explicit RTL places logical cells from the right edge. The decoder must
	// still inherit coordinates in the visual traversal used by SwiftTerm.
	term.Feed([]byte("\x1b[?25l\x1b[8l\x1b[2 k\x1b[38;5;7m" + strings.Repeat(placeholderText(), 3)))
	tt.Frame()
	if len(term.v.lines[0].placeholders) != 3 {
		t.Fatal("BiDi lost placeholder graphemes")
	}
	for i, p := range term.v.lines[0].placeholders {
		if p.col != term.v.cols-3+i || p.imageCol != i {
			t.Fatalf("visual inheritance %d: %+v", i, p)
		}
	}
	w, h := term.v.cellW*3, term.v.cellH
	im := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			im.SetNRGBA(x, y, imagecolor.NRGBA{255, 0, 0, 255})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, im); err != nil {
		t.Fatal(err)
	}
	feedGraphic(term, "a=T,U=1,f=100,i=7,p=1,c=3,r=1", encoded.Bytes())
	tt.Frame()
	for _, p := range term.v.lines[0].placeholders {
		graphicColor(t, term, tt, p.col, 0, imagecolor.RGBA{255, 0, 0, 255})
	}
	term.Feed([]byte("\x1b[0m" + strings.Repeat("\r\n", term.v.rows+2)))
	tt.Frame()
	for _, line := range term.v.lines {
		if len(line.placeholders) != 0 {
			t.Fatal("offscreen placeholder stayed visible")
		}
	}
	if len(term.v.graphicBitmaps) != 0 {
		t.Fatal("offscreen placeholder retained display bitmap")
	}
	term.mu.Lock()
	term.term.ScrollToTop()
	term.mu.Unlock()
	tt.Frame()
	if len(term.v.lines[0].placeholders) != 3 {
		t.Fatal("history scroll lost placeholder cells")
	}
	for _, p := range term.v.lines[0].placeholders {
		graphicColor(t, term, tt, p.col, 0, imagecolor.RGBA{255, 0, 0, 255})
	}
}

package terminal

import (
	"fmt"
	imagecolor "image/color"
	"testing"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/terminal/internal/vt"
)

// Exercise the real UTF-8/grapheme and row-render paths, not just the decoder's
// table. Several protocol marks live outside the BMP, and the placeholder
// itself is private-use text whose width follows the ambiguous-width setting.
func TestKittyPlaceholderAllMarksThroughTerminal(t *testing.T) {
	loadLib(t)
	for _, wide := range []bool{false, true} {
		for _, split := range []bool{false, true} {
			t.Run(fmt.Sprintf("wide=%v/split=%v", wide, split), func(t *testing.T) {
				term, err := New(Options{Conn: newPipe(), NoBlink: true})
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { term.Close() })
				term.mu.Lock()
				err = term.term.SetAmbiguousWidth(wide)
				term.mu.Unlock()
				if err != nil {
					t.Fatal(err)
				}
				tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill() }, 320, 150)
				tt.Frame()
				for i := range kittyPlaceholderDiacritics {
					data := []byte("\x1b[2J\x1b[H\x1b[8l\x1b[1 k\x1b[?25l\x1b[38;2;1;2;3m" + placeholderText(i, i, i) + placeholderText())
					if split {
						for _, b := range data {
							term.Feed([]byte{b})
						}
					} else {
						term.Feed(data)
					}
					tt.Frame()
					line := term.v.lines[0]
					if len(line.placeholders) != 2 || len(line.runs) != 0 {
						t.Fatalf("mark %d U+%04X: placeholders=%+v runs=%d", i, kittyPlaceholderDiacritics[i], line.placeholders, len(line.runs))
					}
					first, second := line.placeholders[0], line.placeholders[1]
					if first.imageRow != i || first.imageCol != i || first.msb != i || first.imageID != 0x010203|uint32(min(i, 255))<<24 {
						t.Fatalf("mark %d: %+v", i, first)
					}
					if wide {
						if second.col != 2 || second.imageRow != 0 || second.imageCol != 0 || second.msb != 0 {
							t.Fatalf("wide adjacency at %d: %+v", i, second)
						}
					} else if second.col != 1 || second.imageRow != i || second.imageCol != i+1 || second.msb != i {
						t.Fatalf("narrow inheritance at %d: %+v", i, second)
					}
				}
			})
		}
	}
}

func TestKittyPlaceholderLargeGridKeepsCellEdges(t *testing.T) {
	// Swift computes the fit and cell intersection in CGFloat before sending
	// the small visible rectangle to Metal. A float32 full-placement transform
	// loses the one-column difference here and paints over the letterbox.
	for _, tc := range []struct {
		cols, rows int
		horizontal bool
	}{
		{1<<24 + 1, 1 << 24, true}, {1 << 24, 1<<24 + 1, false},
		{1<<32 - 1, 1<<32 - 2, true}, {1<<32 - 2, 1<<32 - 1, false},
	} {
		t.Run(fmt.Sprintf("%d/%d", tc.cols, tc.rows), func(t *testing.T) {
			g := stripeClipFixture()
			g.Virtual, g.Clipped = true, false
			g.Columns, g.Rows = tc.cols, tc.rows
			v := &view{cols: 3, rows: 2, cellW: 10, cellH: 20, scale: 1, graphics: []vt.Graphic{g}, lines: []rowCache{{placeholders: []kittyPlaceholder{{}}}}}
			v.prepareGraphics()
			tt := ui.NewTester(func(c *ui.Context) {
				ui.Box(c).Fill().Draw(func(p *ui.Painter, r ui.Rect) {
					p.Fill(r, ui.RGB(0, 0, 255), 0)
					v.drawKittyPlaceholders(p, 3, 5)
				})
			}, 40, 50)
			tt.Frame()
			blue, red := imagecolor.RGBA{0, 0, 255, 255}, imagecolor.RGBA{255, 0, 0, 255}
			checkStripePixel(t, tt, 2, 5, blue)
			if tc.horizontal {
				checkStripePixel(t, tt, 7, 5, red)
			} else {
				checkStripePixel(t, tt, 5, 15, red)
			}
			checkStripePixel(t, tt, 12, 5, blue)
		})
	}
}

func TestKittyPlaceholderLargeGridProtocolToPixels(t *testing.T) {
	for _, rows := range []uint32{1 << 24, 1<<32 - 2} {
		t.Run(fmt.Sprint(rows), func(t *testing.T) {
			term, tt, _ := bidiViewFixture(t)
			tt.Frame()
			v := term.v
			pixels := make([]byte, v.cellW*v.cellH*3)
			for i := 0; i < len(pixels); i += 3 {
				pixels[i] = 255
			}
			feedGraphic(term, fmt.Sprintf("a=T,U=1,f=24,s=%d,v=%d,i=7,p=1,c=%d,r=%d", v.cellW, v.cellH, rows+1, rows), pixels)
			term.Feed([]byte("\x1b[?25l\x1b[8l\x1b[1 k\x1b[38;5;7m" + placeholderText()))
			tt.Frame()
			if len(v.graphics) != 1 || v.graphics[0].Columns != int(rows+1) || v.graphics[0].Rows != int(rows) {
				t.Fatal("protocol truncated virtual grid dimensions")
			}
			for _, sample := range []struct {
				x    int
				want imagecolor.RGBA
			}{
				{v.cellW / 4, imagecolor.RGBA{0, 0, 0, 255}},
				{v.cellW * 3 / 4, imagecolor.RGBA{255, 0, 0, 255}},
				{v.cellW + v.cellW/2, imagecolor.RGBA{0, 0, 0, 255}},
			} {
				if got := tt.Image().RGBAAt(v.ox+sample.x, v.oy+v.cellH/2); got != sample.want {
					t.Fatalf("pixel %d: got %v, want %v", sample.x, got, sample.want)
				}
			}
		})
	}
}

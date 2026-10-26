package vt

import (
	"fmt"
	"image"
	"strings"
	"testing"
	"unsafe"
)

func TestInlineFractionalCanvasWidth(t *testing.T) {
	r := inlineImageRequest{width: 200, height: 200, cols: 20, rows: 10}
	for _, tc := range []struct{ sourceW, sourceH, height, width int }{
		{3, 4, 1, 0}, {3, 4, 2, 1}, {2, 3, 4, 2}, {3, 2, 3, 4},
	} {
		t.Run(fmt.Sprint(tc), func(t *testing.T) {
			im := decodeInlineImage(inlinePayload(fmt.Sprintf("height=%dpx", tc.height), inlinePNG(t, tc.sourceW, tc.sourceH)), r)
			if tc.width == 0 {
				if im != nil {
					t.Fatal("subpixel width created a raster canvas")
				}
				return
			}
			if im == nil || im.Bounds() != image.Rect(0, 0, tc.width, tc.height) {
				t.Fatalf("fractional width must truncate: %v", im)
			}
		})
	}
}

func TestInlineSubpixelWidthDoesNotConsumeRows(t *testing.T) {
	term, replies := inlineTerminal(t)
	term.Write([]byte("\x1b[3;4H"))
	seq := append([]byte("\x1b]1337;File="), inlinePayload("height=1px", inlinePNG(t, 3, 4))...)
	term.Write(append(seq, []byte("\x07\x1b[6nX")...))
	if replies.String() != "\x1b[3;4R" || len(term.Graphics(10, 20)) != 0 {
		t.Fatalf("failed stripe moved the cursor or rendered: %q", replies.String())
	}
	if text := term.Text(); !strings.Contains(text, "X") {
		t.Fatal("following text lost")
	}
}

func TestRowImageCanvasBudgetIncludesTransparentTail(t *testing.T) {
	for _, tc := range []struct{ width, height, cell, want int }{
		{2, 24, 20, 40}, {2, 20, 20, 20}, {2, 1, 20, 20},
		{0, 1, 20, 0}, {1, 0, 20, 0}, {1, 1, 0, 0}, {1, 1, -1, 0},
		{1, TerminalImageLimit / 4, 1, TerminalImageLimit / 4},
		{1, TerminalImageLimit / 4, 3, 0},
		{2, 1, TerminalImageLimit / 4, 0},
		{1, 1, int(^uint(0) >> 1), 0},
	} {
		if got := rowImageCanvasHeight(tc.width, tc.height, tc.cell); got != tc.want {
			t.Fatalf("%+v: padded canvas height %d", tc, got)
		}
	}
}

func TestRowImageResizedStripeFollowsAttachedRow(t *testing.T) {
	for _, tc := range []struct {
		name, sequence string
		source, row    int
	}{
		{"SU", "\x1b[S", 20, 1}, {"SD", "\x1b[T", 0, 2},
		{"RI", "\x1b[2;1H\x1bM", 0, 2}, {"LF", "\x1b[3;1H\n", 20, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			term, _ := inlineTerminal(t)
			term.Write([]byte("\x1b[2;1H"))
			term.Write(append(append([]byte("\x1b]1337;File="), inlinePayload("", inlinePNG(t, 2, 24))...), '\a'))
			if err := term.Resize(20, 10, 10, 10); err != nil {
				t.Fatal(err)
			}
			before := term.Graphics(10, 10)
			if len(before) != 2 {
				t.Fatal("missing initial stripes")
			}
			id := before[0].ImageID
			term.Write([]byte("\x1b[2;3r" + tc.sequence))
			g := term.Graphics(10, 10)
			if len(g) != 1 || g[0].Source.Min.Y != tc.source || g[0].Y != tc.row*10 || g[0].Height != 20 {
				t.Fatalf("scroll treated attached row as a resizable rectangle: %+v", g)
			}
			term.Write([]byte("\x1b[2S"))
			if len(term.Graphics(10, 10)) != 0 {
				t.Fatal("last row survived scroll-out")
			}
			var storage uintptr
			if err := term.get(30, unsafe.Pointer(&storage)); err != nil {
				t.Fatal(err)
			}
			if call(fnGraphicsImage, storage, uintptr(id)) != 0 {
				t.Fatal("last stripe removed but native pixel allocation remains")
			}
		})
	}
}

func TestRowImageFullStripeCanvas(t *testing.T) {
	for _, protocol := range []string{"inline", "sixel"} {
		for _, fragmented := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/fragmented=%v", protocol, fragmented), func(t *testing.T) {
				term, replies := inlineTerminal(t)
				term.Write([]byte("\x1b[20h\x1b[3;4H"))
				packet := append(append([]byte("\x1b]1337;File="), inlinePayload("", inlinePNG(t, 2, 24))...), '\a')
				if protocol == "sixel" {
					packet = []byte("\x1bPq#1;2;100;0;0!2~-!2~-!2~-!2~\x1b\\")
				}
				packet = append(packet, []byte("\x1b[6n")...)
				if fragmented {
					for _, b := range packet {
						term.Write([]byte{b})
					}
				} else {
					term.Write(packet)
				}
				g := term.Graphics(10, 20)
				if replies.String() != "\x1b[5;4R" || len(g) != 2 {
					t.Fatalf("row count or cursor: %q, %d stripes", replies.String(), len(g))
				}
				for i, stripe := range g {
					if stripe.X != 30 || stripe.Y != 40+20*i || stripe.Width != 2 || stripe.Height != 20 || stripe.Source != image.Rect(0, 20*i, 2, 20*(i+1)) {
						t.Fatalf("stripe %d does not occupy a full cell: %+v", i, stripe)
					}
				}
				if g[0].Image != g[1].Image || g[0].Image.Bounds() != image.Rect(0, 0, 2, 40) {
					t.Fatal("stripes should share a padded canvas")
				}
				for y := 0; y < 40; y++ {
					for x := 0; x < 2; x++ {
						p := g[0].Image.NRGBAAt(x, y)
						if (y < 24 && p.A != 255) || (y >= 24 && (p.R != 0 || p.G != 0 || p.B != 0 || p.A != 0)) {
							t.Fatalf("content or transparent tail at %d,%d: %v", x, y, p)
						}
					}
				}
			})
		}
	}
}

func TestRowImageClippedPixelsReleased(t *testing.T) {
	term, _ := inlineTerminal(t)
	for i := 0; i < 20; i++ {
		term.Write([]byte("\x1b[2;3r\x1b[2;1H"))
		term.Write(append(append([]byte("\x1b]1337;File="), inlinePayload("", inlinePNG(t, 4, 20))...), '\a'))
		g := term.Graphics(10, 20)
		if len(g) != 1 {
			t.Fatal("missing image before margin scroll")
		}
		id := g[0].ImageID
		term.Write([]byte("\x1b[2S"))
		if len(term.Graphics(10, 20)) != 0 {
			t.Fatal("clipped image retained placements")
		}
		var storage uintptr
		if err := term.get(30, unsafe.Pointer(&storage)); err != nil {
			t.Fatal(err)
		}
		if call(fnGraphicsImage, storage, uintptr(id)) != 0 {
			t.Fatal("clipped image retained its native allocation")
		}
	}
}

func TestRowImageLongInsertionInsideMargins(t *testing.T) {
	term, _ := inlineTerminal(t)
	term.Write([]byte("\x1b[2;3r\x1b[3;1H"))
	term.Write(append(append([]byte("\x1b]1337;File="), inlinePayload("", inlinePNG(t, 4, 200))...), '\a'))
	g := term.Graphics(10, 20)
	if len(g) != 1 || g[0].Source != image.Rect(0, 180, 4, 200) || g[0].Y != 20 {
		t.Fatalf("scrolling earlier stripes freed an in-progress image: %+v", g)
	}
	if got := g[0].Image.NRGBAAt(0, 199); got.A != 255 || got.G != 239 {
		t.Fatal("last stripe lost its pixels", got)
	}
}

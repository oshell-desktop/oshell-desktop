package vt

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"os"
	"strings"
	"testing"
)

func TestSixelDecodePixelsAndColors(t *testing.T) {
	r := inlineImageRequest{width: 200, height: 200, cols: 20, rows: 10}
	defaults := [16]uint32{1: 0x123456}
	for _, tc := range []struct {
		name, data string
		w, h       int
		pixels     []color.NRGBA
	}{
		{"last pixel", "#1@", 1, 1, []color.NRGBA{{0x12, 0x34, 0x56, 255}}},
		{"leading separator", "#;1@", 1, 1, []color.NRGBA{{0x12, 0x34, 0x56, 255}}},
		{"invalid selector stops", "#1@#x~", 1, 1, []color.NRGBA{{0x12, 0x34, 0x56, 255}}},
		{"repeat", "#1!3@", 3, 1, []color.NRGBA{{0x12, 0x34, 0x56, 255}, {0x12, 0x34, 0x56, 255}, {0x12, 0x34, 0x56, 255}}},
		{"RGB", "#2;2;100;50;0@", 1, 1, []color.NRGBA{{255, 127, 0, 255}}},
		{"HLS blue", "#2;1;0;50;100@", 1, 1, []color.NRGBA{{0, 0, 255, 255}}},
		{"HLS red", "#2;1;120;50;100@", 1, 1, []color.NRGBA{{255, 0, 0, 255}}},
		{"HLS green", "#2;1;240;50;100@", 1, 1, []color.NRGBA{{0, 255, 0, 255}}},
		{"HLS gray", "#2;1;300;50;0@", 1, 1, []color.NRGBA{{127, 127, 127, 255}}},
		{"HLS endpoint rounding", "#2;1;0;93;100@", 1, 1, []color.NRGBA{{219, 219, 255, 255}}},
		{"RGB clamped", "#2;2;200;100;500@", 1, 1, []color.NRGBA{{255, 255, 255, 255}}},
		{"overlay", "#1@A$#2;2;100;0;0A@", 2, 2, []color.NRGBA{{0x12, 0x34, 0x56, 255}, {255, 0, 0, 255}, {255, 0, 0, 255}, {0x12, 0x34, 0x56, 255}}},
		{"transparent unknown color", "#999@", 1, 1, []color.NRGBA{{}}},
		{"transparent holes", "#1A", 1, 2, []color.NRGBA{{}, {0x12, 0x34, 0x56, 255}}},
		{"raster attributes", "\"1;1;1000;1000#1@", 1, 1, []color.NRGBA{{0x12, 0x34, 0x56, 255}}},
		{"color change", "#1@#1;2;0;100;0@", 2, 1, []color.NRGBA{{0x12, 0x34, 0x56, 255}, {0, 255, 0, 255}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			im := decodeSixel([]byte(tc.data), r, defaults)
			if im == nil || im.Bounds() != image.Rect(0, 0, tc.w, tc.h) {
				t.Fatalf("size = %v", im)
			}
			for i, want := range tc.pixels {
				if got := im.NRGBAAt(i%tc.w, i/tc.w); got != want {
					t.Fatalf("pixel %d = %v, want %v", i, got, want)
				}
			}
		})
	}
	if im := decodeSixel([]byte("#1!400~-#1!400~"), r, defaults); im == nil || im.Bounds() != image.Rect(0, 0, 200, 6) {
		t.Fatal("wide sixel did not fit view width")
	}
}

func TestSixelRejectsOverflowAndUnboundedWork(t *testing.T) {
	r := inlineImageRequest{width: 200, height: 200, cols: 20, rows: 10}
	for _, data := range []string{"", "~~~", "#1?", "#1!0~", "#1!9999999999999999999999~", "#1!104857600~", "#1;2;99999999999999999;0;0@", "#1" + strings.Repeat("!100000~$", 60)} {
		if decodeSixel([]byte(data), r, [16]uint32{}) != nil {
			t.Fatalf("unsafe/empty input accepted: %.40s", data)
		}
	}
	// Repeated blank bands cannot overflow coordinates or allocate huge buffers.
	if decodeSixel([]byte("#1"+strings.Repeat("-", 200000)+"!100@"), r, [16]uint32{}) != nil {
		t.Fatal("oversized height accepted")
	}
}

func TestSixelNativeFragmentsPaletteAndCursor(t *testing.T) {
	for _, fragmented := range []bool{false, true} {
		t.Run(fmt.Sprint(fragmented), func(t *testing.T) {
			term, replies := inlineTerminal(t)
			if oshellABI < 5 || oshellABI > 62 {
				if os.Getenv("OSHELL_REQUIRE_WIDTH_EXTENSION") == "1" {
					t.Fatal("required Sixel extension missing")
				}
				t.Skip("requires ABI 5")
			}
			var palette [256]RGB
			palette[1] = RGB{R: 18, G: 52, B: 86}
			term.SetColors(RGB{}, RGB{}, nil, &palette)
			term.Write([]byte("\x1b]4;1;#ff0000\x07\x1b[20h\x1b[3;4H"))
			seq := []byte("\x1bP0;1q#1!2~-#1!2~-#1!2~-#1!2~\x1b\\\x1b[6nX")
			if fragmented {
				for _, b := range seq {
					term.Write([]byte{b})
				}
			} else {
				term.Write(seq)
			}
			if replies.String() != "\x1b[5;4R" {
				t.Fatalf("cursor/query order = %q", replies.String())
			}
			g := term.Graphics(10, 20)
			if len(g) != 2 || g[0].X != 30 || g[0].Y != 40 || g[1].Y != 60 || g[1].Height != 20 {
				t.Fatalf("stripe placement = %+v", g)
			}
			if got := g[0].Image.NRGBAAt(0, 0); got != (color.NRGBA{18, 52, 86, 255}) {
				t.Fatal("Sixel used OSC palette override instead of theme palette", got)
			}
			term.Write(kittyCommand("a=d,d=A", nil))
			if len(term.Graphics(10, 20)) != 2 {
				t.Fatal("Kitty delete removed Sixel")
			}
		})
	}
}

func TestSixelNativeCancellationOverflowAndRecovery(t *testing.T) {
	term, replies := inlineTerminal(t)
	if oshellABI < 5 || oshellABI > 62 {
		t.Skip("requires ABI 5")
	}
	for _, abort := range []byte{0x18, 0x1a} {
		term.Write(append([]byte("\x1bPq#1;2;100;0;0~~"), abort))
		if len(term.Graphics(10, 20)) != 0 {
			t.Fatal("cancelled Sixel was inserted")
		}
	}
	term.Write([]byte("\x1bPq#1;2;100;0;0~~"))
	term.Write(bytes.Repeat([]byte{'?'}, 8<<20))
	term.Write([]byte("\x1b\\"))
	if len(term.Graphics(10, 20)) != 0 {
		t.Fatal("truncated oversized Sixel displayed")
	}
	term.Write([]byte("\x1bPq#1;2;0;100;0~~\x1b\\\x1bP$qm\x1b\\\x1b[c"))
	if g := term.Graphics(10, 20); len(g) != 1 || g[0].Image.NRGBAAt(0, 0) != (color.NRGBA{0, 255, 0, 255}) {
		t.Fatal("new Sixel after discard failed", len(g))
	}
	if !strings.Contains(replies.String(), "\x1bP1$r") {
		t.Fatal("Sixel intercepted unrelated DECRQSS", replies.String())
	}
	// OShell intentionally does not advertise Sixel, even though it decodes it.
	if strings.Contains(replies.String(), ";4;") {
		t.Fatal("advertised Sixel contrary to OShell options")
	}
}

func TestSixelNativeScrollBuffersResetAndColorLifetime(t *testing.T) {
	term, _ := inlineTerminal(t)
	if oshellABI < 5 || oshellABI > 62 {
		t.Skip("requires ABI 5")
	}
	var palette [256]RGB
	palette[1] = RGB{B: 255}
	term.SetColors(RGB{}, RGB{}, nil, &palette)
	term.Write([]byte("\x1b[10;3H\x1bPq#1;2;100;0;0!2~-!2~-!2~-!2~\x1b\\"))
	g := term.Graphics(10, 20)
	if len(g) != 2 || g[0].X != 20 || g[0].Y != 140 || g[1].Y != 160 {
		t.Fatalf("Sixel bottom scroll = %+v", g)
	}
	term.Write([]byte("\x1b[?1049h\x1bPq#1~~\x1b\\"))
	g = term.Graphics(10, 20)
	if len(g) != 1 || g[0].Image.NRGBAAt(0, 0) != (color.NRGBA{0, 0, 255, 255}) {
		t.Fatal("image-local color leaked into next Sixel or alternate buffer")
	}
	term.Write([]byte("\x1b[!p"))
	if len(term.Graphics(10, 20)) != 1 {
		t.Fatal("soft reset discarded Sixel")
	}
	term.Write([]byte("\x1b[?1049l"))
	if len(term.Graphics(10, 20)) != 2 {
		t.Fatal("primary Sixel lost after alternate buffer")
	}
	term.Write([]byte("\x1bc"))
	if len(term.Graphics(10, 20)) != 0 {
		t.Fatal("RIS retained Sixel")
	}
	term.Write([]byte("\x1bPq#1;2;0;100;0~~\x9c"))
	if len(term.Graphics(10, 20)) != 1 {
		t.Fatal("8-bit ST did not terminate Sixel")
	}
}

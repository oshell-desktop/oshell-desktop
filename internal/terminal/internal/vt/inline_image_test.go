package vt

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"strings"
	"testing"
)

func inlinePNG(t *testing.T, width, height int) []byte {
	t.Helper()
	im := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			im.SetNRGBA(x, y, color.NRGBA{R: uint8(80 + x), G: uint8(40 + y), A: 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, im); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func inlinePayload(params string, data []byte) []byte {
	return []byte("inline=1;" + params + ":" + base64.StdEncoding.EncodeToString(data))
}

func TestInlineImageDimensionsAndDecoders(t *testing.T) {
	r := inlineImageRequest{width: 200, height: 200, cols: 20, rows: 10}
	data := inlinePNG(t, 4, 2)
	for _, tc := range []struct {
		params string
		w, h   int
	}{
		{"", 4, 2}, {"width=10px", 10, 5}, {"height=10px", 20, 10},
		{"width=2", 20, 10}, {"height=2", 80, 40}, {"width=50%", 100, 50},
		{"width=8px;height=8px", 8, 8}, {"width=8px;height=8px;preserveAspectRatio=0", 8, 8},
		{"width=200;height=4096px", 4, 2}, {"width=101%;height=-2", 4, 2},
		{"width=auto;height=0", 4, 2}, {"width=999999999999999999999999", 4, 2},
	} {
		t.Run(tc.params, func(t *testing.T) {
			im := decodeInlineImage(inlinePayload(tc.params, data), r)
			if im == nil || im.Bounds().Dx() != tc.w || im.Bounds().Dy() != tc.h {
				t.Fatalf("wrong dimensions: %v", im)
			}
		})
	}
	for _, format := range []string{"jpeg", "gif"} {
		var b bytes.Buffer
		src := image.NewNRGBA(image.Rect(0, 0, 3, 2))
		var err error
		if format == "jpeg" {
			err = jpeg.Encode(&b, src, nil)
		} else {
			err = gif.Encode(&b, src, nil)
		}
		if err != nil || decodeInlineImage(inlinePayload("", b.Bytes()), r) == nil {
			t.Fatal("missing decoder", format, err)
		}
	}
	for _, p := range [][]byte{
		[]byte("inline=0:" + base64.StdEncoding.EncodeToString(data)),
		[]byte("inline=1:invalid base64"), []byte("inline=1"),
		inlinePayload("", []byte("not an image")),
	} {
		if decodeInlineImage(p, r) != nil {
			t.Fatal("invalid/unsafe image accepted")
		}
	}
	if inlineImageSize(TerminalImageLimit/4+1, 1) || inlineImageSize(1, TerminalImageLimit/4+1) || inlineImageSize(1<<30, 1<<30) {
		t.Fatal("decoded image bounds accepted an oversized allocation")
	}
	// Centered clipping of an extreme aspect ratio must not allocate a
	// multi-million-pixel scale filter for the off-canvas destination.
	if im := decodeInlineImage(inlinePayload("width=8px;height=8px", inlinePNG(t, 32768, 1)), r); im == nil || im.Bounds().Dx() != 8 {
		t.Fatal("bounded clipped aspect ratio failed")
	}
}

func inlineTerminal(t *testing.T) (*Terminal, *strings.Builder) {
	t.Helper()
	term, replies := graphicTerminal(t)
	if oshellABI < 4 || oshellABI > 62 {
		if os.Getenv("OSHELL_REQUIRE_WIDTH_EXTENSION") == "1" {
			t.Fatal("required OShell inline-image extension absent")
		}
		t.Skip("requires OShell ABI 4")
	}
	return term, replies
}

func TestInlineImageNativeStripesCursorAndFragments(t *testing.T) {
	for _, terminator := range []string{"\x07", "\x1b\\"} {
		for _, fragmented := range []bool{false, true} {
			t.Run(fmt.Sprintf("%x-%v", terminator, fragmented), func(t *testing.T) {
				term, replies := inlineTerminal(t)
				term.Write([]byte("\x1b[20h\x1b[3;4H"))
				seq := append([]byte("\x1b]1337;File="), inlinePayload("", inlinePNG(t, 2, 25))...)
				seq = append(seq, []byte(terminator+"\x1b[6nX")...)
				if fragmented {
					for _, b := range seq {
						term.Write([]byte{b})
					}
				} else {
					term.Write(seq)
				}
				if replies.String() != "\x1b[5;4R" {
					t.Fatalf("inline insertion/query order or LNM cursor incorrect: %q", replies.String())
				}
				graphics := term.Graphics(10, 20)
				if len(graphics) != 2 {
					t.Fatalf("missing row stripes: %d", len(graphics))
				}
				for i, g := range graphics {
					if g.X != 30 || g.Y != 40+i*20 || g.Width != 2 || g.Height != 20 || g.Source.Min.Y != i*20 {
						t.Fatalf("stripe %d geometry %+v", i, g)
					}
				}
				if graphics[0].Image != graphics[1].Image || !strings.Contains(term.Text(), "X") {
					t.Fatal("stripes duplicated pixels or swallowed following text")
				}
				term.Write([]byte("\x1b[!p"))
				if len(term.Graphics(10, 20)) != 2 {
					t.Fatal("soft reset cleared inline image")
				}
				term.Write([]byte("\x1bc"))
				if len(term.Graphics(10, 20)) != 0 {
					t.Fatal("full reset retained inline image")
				}
			})
		}
	}
}

func TestInlineImageNativeScrollIsolationAndCancellation(t *testing.T) {
	term, _ := inlineTerminal(t)
	seq := append([]byte("\x1b]1337;File="), inlinePayload("", inlinePNG(t, 2, 45))...)
	term.Write([]byte("\x1b[10;3H"))
	term.Write(append(seq, '\x07'))
	g := term.Graphics(10, 20)
	if len(g) != 3 || g[0].Y != 120 || g[2].Y != 160 {
		t.Fatalf("bottom insertion did not scroll stripes: %+v", g)
	}
	term.Write([]byte("\x1b[?1049h"))
	if len(term.Graphics(10, 20)) != 0 {
		t.Fatal("primary image leaked into alternate screen")
	}
	term.Write(append(seq, '\x18')) // CAN aborts instead of inserting.
	if len(term.Graphics(10, 20)) != 0 {
		t.Fatal("cancelled OSC displayed")
	}
	term.Write([]byte("\x1b[?1049l"))
	if len(term.Graphics(10, 20)) != 3 {
		t.Fatal("primary image lost on return")
	}
	x, y := term.Cursor()
	term.Write([]byte("\x1b]1337;File=inline=0:AAAA\x07"))
	x2, y2 := term.Cursor()
	if x != x2 || y != y2 || len(term.Graphics(10, 20)) != 3 {
		t.Fatal("download command changed screen")
	}
}

func TestInlineImageKittyIsolationAndErase(t *testing.T) {
	term, _ := inlineTerminal(t)
	seq := append([]byte("\x1b]1337;File="), inlinePayload("", inlinePNG(t, 20, 20))...)
	term.Write(append(seq, '\x07'))
	graphics := term.Graphics(10, 20)
	if len(graphics) != 1 {
		t.Fatal("missing image")
	}
	id := graphics[0].ImageID
	for _, command := range []string{
		"a=d,d=A", "a=d,d=C", "a=d,d=P,x=1,y=1", "a=d,d=Q,x=1,y=1,z=0",
		"a=d,d=X,x=1", "a=d,d=Y,y=1", "a=d,d=Z,z=0", "a=d,d=R,x=1,y=4294967295",
		fmt.Sprintf("a=d,d=I,i=%d", id), fmt.Sprintf("a=d,d=F,i=%d", id),
		fmt.Sprintf("a=t,f=24,s=1,v=1,i=%d", id),
	} {
		term.Write([]byte("\x1b[H"))
		term.Write(kittyCommand(command, []byte{255, 0, 0}))
		if g := term.Graphics(10, 20); len(g) != 1 || g[0].Generation != graphics[0].Generation {
			t.Fatal("Kitty command mutated inline image", command)
		}
	}
	term.Write([]byte("\x1b[2J"))
	if len(term.Graphics(10, 20)) != 0 {
		t.Fatal("screen erase retained inline image")
	}
}

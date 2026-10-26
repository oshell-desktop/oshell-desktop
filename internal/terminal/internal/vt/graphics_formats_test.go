package vt

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"strings"
	"testing"

	"golang.org/x/image/bmp"
	"oshell-desktop/internal/tiff"
)

func kittyFormatFixture(t *testing.T, format string) ([]byte, image.Image) {
	t.Helper()
	if format != "bmp" && format != "gif" {
		return inlineOrientedImage(t, format, 1)
	}
	src := image.NewNRGBA(image.Rect(0, 0, 30, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 30; x++ {
			src.SetNRGBA(x, y, color.NRGBA{uint8(x * 7), uint8(y * 23), 80, 255})
		}
	}
	var data bytes.Buffer
	var err error
	if format == "bmp" {
		err = bmp.Encode(&data, src)
	} else {
		err = gif.Encode(&data, src, nil)
	}
	if err != nil {
		t.Fatal(err)
	}
	raw, _, err := image.Decode(bytes.NewReader(data.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	return data.Bytes(), raw
}

func TestKittyEncodedImageFormats(t *testing.T) {
	for _, format := range []string{"png", "jpeg", "gif", "bmp", "tiff", "webp"} {
		for _, compressed := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/zlib%v", format, compressed), func(t *testing.T) {
				term, replies := kittyCacheTerminal(t)
				data, raw := kittyFormatFixture(t, format)
				control := "f=100,i=71,p=1,c=3,r=1,C=1"
				if compressed {
					var b bytes.Buffer
					w := zlib.NewWriter(&b)
					_, _ = w.Write(data)
					if err := w.Close(); err != nil {
						t.Fatal(err)
					}
					data = b.Bytes()
					control += ",o=z"
				}
				term.Write(kittyCommand("a=q,"+control, data))
				if replies.String() != "\x1b_Gi=71,p=1;OK\x1b\\" || len(term.Graphics(10, 20)) != 0 {
					t.Fatal("format query failed", replies.String())
				}
				replies.Reset()
				encoded := base64.StdEncoding.EncodeToString(data)
				cut := len(encoded)/2 + 1
				term.Write(kittyEncodedCommand("a=T,m=1,"+control, encoded[:cut]))
				final := kittyEncodedCommand("m=0", encoded[cut:])
				for _, b := range final {
					term.Write([]byte{b})
				}
				g := term.Graphics(10, 20)
				if replies.String() != "\x1b_Gi=71,p=1;OK\x1b\\" || len(g) != 1 {
					t.Fatal("format display failed", replies.String(), g)
				}
				if g[0].Image.Bounds() != image.Rect(0, 0, 30, 10) || g[0].Width != 30 || g[0].Height != 20 {
					t.Fatal("format geometry changed", g)
				}
				for _, p := range []image.Point{{0, 0}, {29, 0}, {0, 9}, {29, 9}, {13, 5}} {
					if got, want := g[0].Image.NRGBAAt(p.X, p.Y), color.NRGBAModel.Convert(raw.At(p.X, p.Y)); got != want {
						t.Fatal("format pixel changed", p, got, want)
					}
				}
				before := g[0].Image
				term.Write(kittyCommand("a=p,i=71,p=1,x=2,y=1,w=6,h=3,c=3,C=1", nil))
				g = term.Graphics(10, 20)
				if len(g) != 1 || g[0].Image != before || g[0].Source != image.Rect(2, 1, 8, 4) || g[0].Width != 30 || g[0].Height != 15 {
					t.Fatal("encoded format crop/cache differs", g)
				}
				term.Write(kittyCommand("a=d,d=I,i=71", nil))
				if len(term.Graphics(10, 20)) != 0 || len(term.graphicsCache) != 0 {
					t.Fatal("format deletion retained view")
				}
			})
		}
	}
}

func TestKittyEncodedFormatOrientationAndCrop(t *testing.T) {
	corners := [8]image.Point{{0, 0}, {29, 0}, {29, 9}, {0, 9}, {0, 0}, {0, 9}, {29, 9}, {29, 0}}
	for _, format := range []string{"jpeg", "tiff", "webp"} {
		for orientation := byte(1); orientation <= 8; orientation++ {
			t.Run(fmt.Sprintf("%s/%d", format, orientation), func(t *testing.T) {
				term, replies := kittyCacheTerminal(t)
				data, raw := inlineOrientedImage(t, format, orientation)
				term.Write(kittyCommand("a=T,f=100,i=71,p=1", data))
				g := term.Graphics(10, 20)
				if len(g) != 1 {
					t.Fatal("oriented format failed", replies.String())
				}
				w, h := 30, 10
				if orientation >= 5 {
					w, h = h, w
				}
				p := corners[orientation-1]
				if g[0].Image.Bounds() != image.Rect(0, 0, w, h) || g[0].Image.NRGBAAt(0, 0) != color.NRGBAModel.Convert(raw.At(p.X, p.Y)) {
					t.Fatal("display orientation lost", g)
				}
				if x, y := term.Cursor(); x != 3 || y != 1 {
					t.Fatal("protocol cursor used oriented geometry", x, y)
				}
				term.Write(kittyCommand("a=p,i=71,p=2,x=2,y=1,w=6,h=3,C=1", nil))
				g = term.Graphics(10, 20)
				if len(g) != 2 || g[1].Image.Bounds() != image.Rect(0, 0, 30, 10) || g[1].Source != image.Rect(2, 1, 8, 4) || g[1].Image.NRGBAAt(2, 1) != color.NRGBAModel.Convert(raw.At(2, 1)) {
					t.Fatal("crop failed to use raw coordinates", g)
				}
			})
		}
	}
}

func TestKittyGIFCanvasAndFirstFrame(t *testing.T) {
	palette := color.Palette{color.NRGBA{}, color.NRGBA{255, 0, 0, 255}, color.NRGBA{0, 0, 255, 255}}
	first := image.NewPaletted(image.Rect(2, 1, 4, 3), palette)
	for i := range first.Pix {
		first.Pix[i] = 1
	}
	second := image.NewPaletted(image.Rect(0, 0, 8, 4), palette)
	for i := range second.Pix {
		second.Pix[i] = 2
	}
	var data bytes.Buffer
	if err := gif.EncodeAll(&data, &gif.GIF{Image: []*image.Paletted{first, second}, Delay: []int{1, 1}, Config: image.Config{Width: 8, Height: 4, ColorModel: palette}}); err != nil {
		t.Fatal(err)
	}
	term, replies := kittyCacheTerminal(t)
	term.Write(kittyCommand("a=T,f=100,i=71,C=1", data.Bytes()))
	g := term.Graphics(10, 20)
	if len(g) != 1 || !strings.Contains(replies.String(), ";OK") {
		t.Fatal("GIF canvas failed", replies.String())
	}
	if g[0].Image.Bounds() != image.Rect(0, 0, 8, 4) || g[0].Image.NRGBAAt(2, 1) != (color.NRGBA{255, 0, 0, 255}) || g[0].Image.NRGBAAt(0, 0).A != 0 || g[0].Image.NRGBAAt(4, 3).A != 0 {
		t.Fatal("GIF frame offset/transparency/first frame changed", g)
	}
}

func TestKittyTIFFTransparencyAndDepth(t *testing.T) {
	src := image.NewNRGBA64(image.Rect(0, 0, 2, 2))
	src.SetNRGBA64(0, 0, color.NRGBA64{0xffff, 0x8000, 0x4000, 0xffff})
	src.SetNRGBA64(1, 0, color.NRGBA64{0xffff, 0x4000, 0x8000, 0x8000})
	src.SetNRGBA64(0, 1, color.NRGBA64{0, 0, 0, 0})
	src.SetNRGBA64(1, 1, color.NRGBA64{0x2000, 0xffff, 0, 0x4000})
	var data bytes.Buffer
	if err := tiff.Encode(&data, src, &tiff.Options{Compression: tiff.Deflate, Predictor: true}); err != nil {
		t.Fatal(err)
	}
	term, replies := kittyCacheTerminal(t)
	term.Write(kittyCommand("a=T,f=100,i=71,C=1", data.Bytes()))
	g := term.Graphics(10, 20)
	if len(g) != 1 {
		t.Fatal("16-bit TIFF failed", replies.String())
	}
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			got := g[0].Image.NRGBAAt(x, y)
			want := color.NRGBAModel.Convert(src.At(x, y)).(color.NRGBA)
			if got != want {
				t.Fatal("TIFF alpha/depth changed", x, y, got, want)
			}
		}
	}
}

func TestKittyEncodedFormatBoundsAndFailurePreserve(t *testing.T) {
	for _, format := range []string{"png", "jpeg", "gif", "bmp", "tiff", "webp"} {
		t.Run(format, func(t *testing.T) {
			term, replies := kittyCacheTerminal(t)
			data, _ := kittyFormatFixture(t, format)
			term.Write(kittyCommand("a=T,f=100,i=71,p=1,c=3,r=1,C=1", data))
			before := term.Graphics(10, 20)
			for _, invalid := range [][]byte{nil, []byte("unrecognized image"), data[:min(10, len(data))]} {
				replies.Reset()
				term.Write(kittyCommand("a=T,f=100,i=71,p=1,C=1", invalid))
				if replies.String() != "\x1b_Gi=71;EINVAL: bad payload\x1b\\" {
					t.Fatal("bad replacement reply", replies.String())
				}
				after := term.Graphics(10, 20)
				if len(after) != 1 || after[0].Image != before[0].Image {
					t.Fatal("bad format replacement lost valid display")
				}
			}
		})
	}
	// A real, narrow BMP crosses Swift's per-axis bound while staying tiny;
	// reject its header before starting a pixel decode or native allocation.
	for _, width := range []int{10000, 10001} {
		src := image.NewNRGBA(image.Rect(0, 0, width, 1))
		var data bytes.Buffer
		if err := bmp.Encode(&data, src); err != nil {
			t.Fatal(err)
		}
		term, replies := kittyCacheTerminal(t)
		term.Write(kittyCommand("a=q,f=100,i=71", data.Bytes()))
		want := "\x1b_Gi=71;OK\x1b\\"
		if width > 10000 {
			want = "\x1b_Gi=71;EINVAL: bad payload\x1b\\"
		}
		if replies.String() != want {
			t.Fatal("per-axis image bound", width, replies.String())
		}
	}
}

package core

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gen2brain/h265/heic"
)

func normalizedHEIC(t *testing.T, data []byte) image.Image {
	t.Helper()
	encoded, err := NormalizeClipboardImage(data)
	if err != nil {
		t.Fatal(err)
	}
	im, err := png.Decode(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal("not PNG", err)
	}
	return im
}

func TestClipboardHEICFixtures(t *testing.T) {
	for _, name := range []string{"basic.heic", "alpha.heic", "main10.heic"} {
		t.Run(name, func(t *testing.T) {
			data, err := ReadClipboardImageFile(filepath.Join("testdata", "heic", name))
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := heic.DecodeConfig(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			im := normalizedHEIC(t, data)
			if im.Bounds().Dx() != cfg.Width || im.Bounds().Dy() != cfg.Height {
				t.Fatal("dimensions changed", im.Bounds(), cfg)
			}
			if cfg.ColorModel == color.NRGBA64Model && im.ColorModel() != color.NRGBA64Model && im.ColorModel() != color.RGBA64Model {
				t.Fatal("high-bit-depth HEIC reduced to 8-bit PNG")
			}
			if name == "alpha.heic" {
				_, _, _, left := im.At(0, 0).RGBA()
				_, _, _, middle := im.At(cfg.Width/2, 0).RGBA()
				_, _, _, right := im.At(cfg.Width-1, 0).RGBA()
				if left > 8*257 || right < 240*257 || middle <= left || middle >= right {
					t.Fatal("alpha gradient lost in PNG conversion", left, middle, right)
				}
			}
		})
	}
}

func TestClipboardHEICExplicitTenBit(t *testing.T) {
	im := image.NewNRGBA64(image.Rect(0, 0, 32, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 32; x++ {
			im.SetNRGBA64(x, y, color.NRGBA64{uint16(x * 1901), uint16(y * 3701), 12345, 65535})
		}
	}
	var encoded bytes.Buffer
	if err := heic.Encode(&encoded, im, heic.EncodeOptions{BitDepth: 10, Lossless: true, Chroma: heic.Chroma444}); err != nil {
		t.Fatal(err)
	}
	cfg, err := heic.DecodeConfig(bytes.NewReader(encoded.Bytes()))
	if err != nil || cfg.ColorModel != color.NRGBA64Model {
		t.Fatal("fixture is not 10-bit", err)
	}
	decoded := normalizedHEIC(t, encoded.Bytes())
	if decoded.ColorModel() != color.RGBA64Model && decoded.ColorModel() != color.NRGBA64Model {
		t.Fatalf("lost bit depth: %T", decoded)
	}
	// Values between 8-bit steps must survive; an 8-bit roundtrip repeats bytes.
	fine := false
	for y := 0; y < 16; y++ {
		for x := 0; x < 32; x++ {
			r, g, b, _ := decoded.At(x, y).RGBA()
			fine = fine || r%257 != 0 || g%257 != 0 || b%257 != 0
		}
	}
	if !fine {
		t.Fatal("16-bit container contains only quantized 8-bit pixels")
	}
}

func heicFixture(t *testing.T, width, height, tile int) []byte {
	t.Helper()
	im := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			im.SetNRGBA(x, y, color.NRGBA{uint8(x * 7), uint8(y * 11), 30, 255})
		}
	}
	var data bytes.Buffer
	if err := heic.Encode(&data, im, heic.EncodeOptions{Lossless: true, Tile: tile}); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

// Add a property to this test encoder's primary item. Adjust all enclosing box
// sizes and absolute item offsets, so the test exercises a valid HEIF container.
func transformedHEIC(t *testing.T, data []byte, kind string, value byte) []byte {
	t.Helper()
	box := func(kind string, body []byte) []byte {
		out := binary.BigEndian.AppendUint32(nil, uint32(len(body)+8))
		out = append(out, kind...)
		return append(out, body...)
	}
	propertyIndex := byte(0)
	var rewrite func([]byte) []byte
	rewrite = func(src []byte) []byte {
		var out []byte
		for len(src) > 0 {
			if len(src) < 8 {
				t.Fatal("bad fixture box")
			}
			n := int(binary.BigEndian.Uint32(src))
			if n < 8 || n > len(src) {
				t.Fatal("bad fixture size")
			}
			typ, body := string(src[4:8]), bytes.Clone(src[8:n])
			switch typ {
			case "meta":
				body = append(body[:4:4], rewrite(body[4:])...)
			case "iprp":
				body = rewrite(body)
			case "ipco":
				for p := 0; p < len(body); {
					propertyIndex++
					p += int(binary.BigEndian.Uint32(body[p:]))
				}
				propertyIndex++
				body = append(body, box(kind, []byte{value})...)
			case "ipma":
				if propertyIndex == 0 || binary.BigEndian.Uint16(body[8:]) != 1 {
					t.Fatal("unexpected primary item")
				}
				end := 11 + int(body[10])
				body[10]++
				body = append(append(append([]byte{}, body[:end]...), propertyIndex), body[end:]...)
			case "iloc":
				if body[0] != 0 || body[4] != 0x44 || body[5] != 0 {
					t.Fatal("unexpected fixture location format")
				}
				for p, count := 8, int(binary.BigEndian.Uint16(body[6:])); count > 0; count, p = count-1, p+14 {
					if binary.BigEndian.Uint16(body[p+4:]) != 1 {
						t.Fatal("unexpected fixture extent")
					}
					binary.BigEndian.PutUint32(body[p+6:], binary.BigEndian.Uint32(body[p+6:])+10)
				}
			}
			out = append(out, box(typ, body)...)
			src = src[n:]
		}
		return out
	}
	return rewrite(data)
}

func TestClipboardHEICTransformsAndGrid(t *testing.T) {
	data := heicFixture(t, 32, 16, 0)
	original := normalizedHEIC(t, data)
	for _, tc := range []struct {
		kind                string
		value               byte
		width, height, x, y int
	}{
		{"irot", 1, 16, 32, 31, 0}, {"irot", 2, 32, 16, 31, 15}, {"irot", 3, 16, 32, 0, 15},
		{"imir", 0, 32, 16, 0, 15}, {"imir", 1, 32, 16, 31, 0},
	} {
		t.Run(fmt.Sprintf("%s-%d", tc.kind, tc.value), func(t *testing.T) {
			im := normalizedHEIC(t, transformedHEIC(t, data, tc.kind, tc.value))
			if im.Bounds().Dx() != tc.width || im.Bounds().Dy() != tc.height {
				t.Fatal("wrong transform size", im.Bounds())
			}
			if color.NRGBA64Model.Convert(im.At(0, 0)) != color.NRGBA64Model.Convert(original.At(tc.x, tc.y)) {
				t.Fatal("wrong transformed corner")
			}
		})
	}
	for _, tile := range []int{0, 16} {
		im := normalizedHEIC(t, heicFixture(t, 31, 17, tile))
		if im.Bounds().Dx() != 31 || im.Bounds().Dy() != 17 {
			t.Fatal("clean aperture/grid dimensions", tile, im.Bounds())
		}
	}
}

func TestClipboardHEICRejectsMalformedAndOversize(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "heic", "basic.heic"))
	if err != nil {
		t.Fatal(err)
	}
	for _, length := range []int{0, 12, 32, len(data) / 2} {
		if _, err := NormalizeClipboardImage(data[:length]); err == nil {
			t.Fatal("truncated HEIC accepted", length)
		}
	}
	oversize := bytes.Clone(data)
	i := bytes.Index(oversize, []byte("ispe"))
	if i < 0 {
		t.Fatal("fixture has no dimensions")
	}
	binary.BigEndian.PutUint32(oversize[i+8:], 20001)
	if _, err := NormalizeClipboardImage(oversize); err == nil || !strings.Contains(err.Error(), "尺寸") {
		t.Fatal("oversize HEIC not rejected before decode", err)
	}
}

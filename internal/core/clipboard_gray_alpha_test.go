package core

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/png"
	"math/rand"
	"testing"
)

// Build a color-type 4 PNG independently of the production encoder. Go's
// image/png decoder supports gray+alpha, but its encoder emits RGB+alpha.
func clipboardGrayAlphaFixture(t *testing.T, depth byte, opaque ...bool) ([]byte, []color.NRGBA64) {
	t.Helper()
	samples := []color.NRGBA64{{0x1212, 0x1212, 0x1212, 0x4949}, {0xabab, 0xabab, 0xabab, 0}, {0x3434, 0x3434, 0x3434, 65535}, {0xfefe, 0xfefe, 0xfefe, 0x8080}}
	if depth == 16 {
		samples[0] = color.NRGBA64{12345, 12345, 12345, 23456}
		samples[3] = color.NRGBA64{54321, 54321, 54321, 45678}
	}
	if len(opaque) > 0 && opaque[0] {
		for i := range samples {
			samples[i].A = 65535
		}
	}
	var raw []byte
	for y := 0; y < 2; y++ {
		raw = append(raw, 0)
		for _, p := range samples[y*2 : y*2+2] {
			if depth == 8 {
				raw = append(raw, byte(p.R>>8), byte(p.A>>8))
			} else {
				raw = binary.BigEndian.AppendUint16(raw, p.R)
				raw = binary.BigEndian.AppendUint16(raw, p.A)
			}
		}
	}
	var compressed bytes.Buffer
	w := zlib.NewWriter(&compressed)
	if _, err := w.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	header := make([]byte, 13)
	binary.BigEndian.PutUint32(header, 2)
	binary.BigEndian.PutUint32(header[4:], 2)
	header[8], header[9] = depth, 4
	data := appendClipboardPNGChunk([]byte("\x89PNG\r\n\x1a\n"), "IHDR", header)
	data = appendClipboardPNGChunk(data, "IDAT", compressed.Bytes())
	data = appendClipboardPNGChunk(data, "IEND", nil)
	data, err := (clipboardColorMetadata{icc: clipboardTestICC("GRAY")}).insertPNG(data)
	if err != nil {
		t.Fatal(err)
	}
	return data, samples
}

func TestClipboardGrayAlphaICC(t *testing.T) {
	for _, depth := range []byte{8, 16} {
		for _, opaque := range []bool{false, true} {
			input, samples := clipboardGrayAlphaFixture(t, depth, opaque)
			output, err := NormalizeClipboardImage(input)
			if err != nil {
				t.Fatalf("%d-bit gray+alpha: %v", depth, err)
			}
			if output[24] != depth || output[25] != 4 {
				t.Fatalf("wrong gray alpha PNG encoding: %v", output[24:26])
			}
			assertPNGProfile(t, output, clipboardTestICC("GRAY"))
			im, err := png.Decode(bytes.NewReader(output))
			if err != nil {
				t.Fatal(err)
			}
			for i, want := range samples {
				got := im.At(i%2, i/2)
				var sample color.NRGBA64
				switch c := got.(type) {
				case color.NRGBA:
					sample = color.NRGBA64{uint16(c.R) * 257, uint16(c.G) * 257, uint16(c.B) * 257, uint16(c.A) * 257}
				case color.NRGBA64:
					sample = c
				default:
					t.Fatalf("unexpected decoded color %T", got)
				}
				if sample != want {
					t.Fatalf("%d-bit sample %d: got %v, want %v", depth, i, sample, want)
				}
			}
		}
	}
}

func TestClipboardGrayAlphaRejectsRGBProfile(t *testing.T) {
	input, _ := clipboardGrayAlphaFixture(t, 8)
	var profile bytes.Buffer
	w := zlib.NewWriter(&profile)
	w.Write(clipboardTestICC("RGB "))
	w.Close()
	forged := appendClipboardPNGChunk(append([]byte(nil), input[:33]...), "iCCP", append([]byte("ICC\x00\x00"), profile.Bytes()...))
	for at := 33; at < len(input); {
		n := int(binary.BigEndian.Uint32(input[at:])) + 12
		if string(input[at+4:at+8]) != "iCCP" {
			forged = append(forged, input[at:at+n]...)
		}
		at += n
	}
	if _, err := NormalizeClipboardImage(forged); !errors.Is(err, errClipboardColor) {
		t.Fatal("RGB decoder representation hid invalid gray/RGB source profile", err)
	}
}

func grayAlphaSample(c color.Color) color.NRGBA64 {
	switch c := c.(type) {
	case color.NRGBA:
		return color.NRGBA64{uint16(c.R) * 257, uint16(c.G) * 257, uint16(c.B) * 257, uint16(c.A) * 257}
	case color.NRGBA64:
		return c
	default:
		return color.NRGBA64Model.Convert(c).(color.NRGBA64)
	}
}

func TestClipboardGrayAlphaOrientation(t *testing.T) {
	orders := [8][4]int{{0, 1, 2, 3}, {1, 0, 3, 2}, {3, 2, 1, 0}, {2, 3, 0, 1}, {0, 2, 1, 3}, {2, 0, 3, 1}, {3, 1, 2, 0}, {1, 3, 0, 2}}
	for _, depth := range []byte{8, 16} {
		for orientation := 1; orientation <= 8; orientation++ {
			input, samples := clipboardGrayAlphaFixture(t, depth)
			exif := make([]byte, 26)
			copy(exif, "II")
			binary.LittleEndian.PutUint16(exif[2:], 42)
			binary.LittleEndian.PutUint32(exif[4:], 8)
			binary.LittleEndian.PutUint16(exif[8:], 1)
			binary.LittleEndian.PutUint16(exif[10:], 0x112)
			binary.LittleEndian.PutUint16(exif[12:], 3)
			binary.LittleEndian.PutUint32(exif[14:], 1)
			binary.LittleEndian.PutUint16(exif[18:], uint16(orientation))
			withEXIF := appendClipboardPNGChunk(append([]byte(nil), input[:33]...), "eXIf", exif)
			withEXIF = append(withEXIF, input[33:]...)
			output, err := NormalizeClipboardImage(withEXIF)
			if err != nil {
				t.Fatal(depth, orientation, err)
			}
			if testPNGChunk(t, output, "eXIf") != nil {
				t.Fatal("applied orientation was retained")
			}
			assertPNGProfile(t, output, clipboardTestICC("GRAY"))
			im, err := png.Decode(bytes.NewReader(output))
			if err != nil {
				t.Fatal(err)
			}
			for i, source := range orders[orientation-1] {
				if got := grayAlphaSample(im.At(i%2, i/2)); got != samples[source] {
					t.Fatalf("depth %d orientation %d pixel %d: %v want %v", depth, orientation, i, got, samples[source])
				}
			}
		}
	}
}

func TestClipboardGrayTransparencyKey(t *testing.T) {
	for _, depth := range []byte{1, 2, 4, 8, 16} {
		maximum := uint16((uint32(1) << depth) - 1)
		samples := []uint16{0, 1, maximum - 1, maximum}
		row := make([]byte, 1+(4*int(depth)+7)/8)
		for i, v := range samples {
			if depth == 16 {
				binary.BigEndian.PutUint16(row[1+i*2:], v)
			} else if depth == 8 {
				row[1+i] = byte(v)
			} else {
				row[1+i*int(depth)/8] |= byte(v) << uint(8-int(depth)-(i*int(depth))%8)
			}
		}
		var compressed bytes.Buffer
		w := zlib.NewWriter(&compressed)
		w.Write(row)
		w.Close()
		header := make([]byte, 13)
		binary.BigEndian.PutUint32(header, 4)
		binary.BigEndian.PutUint32(header[4:], 1)
		header[8] = depth
		input := appendClipboardPNGChunk([]byte("\x89PNG\r\n\x1a\n"), "IHDR", header)
		input = appendClipboardPNGChunk(input, "tRNS", []byte{0, 1})
		input = appendClipboardPNGChunk(input, "IDAT", compressed.Bytes())
		input = appendClipboardPNGChunk(input, "IEND", nil)
		input, err := (clipboardColorMetadata{icc: clipboardTestICC("GRAY")}).insertPNG(input)
		if err != nil {
			t.Fatal(err)
		}
		output, err := NormalizeClipboardImage(input)
		if err != nil {
			t.Fatal(depth, err)
		}
		assertPNGProfile(t, output, clipboardTestICC("GRAY"))
		if output[24] != max(depth, 8) || output[25] != 4 {
			t.Fatal("wrong transparency output", output[24:26])
		}
		im, err := png.Decode(bytes.NewReader(output))
		if err != nil {
			t.Fatal(err)
		}
		for i, v := range samples {
			gray := uint16(uint32(v) * 65535 / uint32(maximum))
			alpha := uint16(65535)
			if v == 1 {
				alpha = 0
			}
			if got, want := grayAlphaSample(im.At(i, 0)), (color.NRGBA64{gray, gray, gray, alpha}); got != want {
				t.Fatalf("depth %d sample %d: %v want %v", depth, i, got, want)
			}
		}
	}
}

func TestClipboardGrayAlphaFilterRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewSource(71))
	for _, depth := range []byte{8, 16} {
		im := image.NewNRGBA64(image.Rect(3, 5, 70, 44))
		for y := im.Rect.Min.Y; y < im.Rect.Max.Y; y++ {
			for x := im.Rect.Min.X; x < im.Rect.Max.X; x++ {
				gray, alpha := uint16(rng.Uint32()), uint16(rng.Uint32())
				if depth == 8 {
					gray = (gray >> 8) * 257
					alpha = (alpha >> 8) * 257
				}
				im.SetNRGBA64(x, y, color.NRGBA64{gray, gray, gray, alpha})
			}
		}
		encoded, err := encodeClipboardGrayAlpha(im, depth)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := png.Decode(bytes.NewReader(encoded))
		if err != nil {
			t.Fatal(err)
		}
		for y := 0; y < im.Rect.Dy(); y++ {
			for x := 0; x < im.Rect.Dx(); x++ {
				if got, want := grayAlphaSample(decoded.At(x, y)), im.NRGBA64At(x+3, y+5); got != want {
					t.Fatalf("depth %d pixel %d,%d: %v want %v", depth, x, y, got, want)
				}
			}
		}
	}
	var limited clipboardPNGBuffer
	if _, err := limited.Write(make([]byte, ClipboardImageLimit+1)); !errors.Is(err, errClipboardPNGSize) || limited.Len() != 0 {
		t.Fatal("PNG output budget not enforced", err, limited.Len())
	}
	invalid := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	invalid.SetNRGBA(0, 0, color.NRGBA{1, 2, 3, 255})
	if _, err := encodeClipboardGrayAlpha(invalid, 8); !errors.Is(err, errClipboardColor) {
		t.Fatal("colored sample labelled grayscale", err)
	}
}

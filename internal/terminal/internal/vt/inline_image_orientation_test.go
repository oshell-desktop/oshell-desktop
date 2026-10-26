package vt

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"oshell-desktop/internal/tiff"
)

func inlineOrientedImage(t *testing.T, format string, orientation byte) ([]byte, image.Image) {
	t.Helper()
	src := image.NewNRGBA(image.Rect(0, 0, 30, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 30; x++ {
			src.SetNRGBA(x, y, color.NRGBA{uint8(7 * x), uint8(23 * y), 80, 255})
		}
	}
	var encoded bytes.Buffer
	var err error
	switch format {
	case "jpeg":
		err = jpeg.Encode(&encoded, src, &jpeg.Options{Quality: 100})
	case "tiff":
		err = tiff.Encode(&encoded, src, nil)
	case "webp":
		// Lossless synthetic 30x10 RGB gradient, R=7*x, G=23*y, B=80.
		var b []byte
		b, err = base64.StdEncoding.DecodeString("UklGRi4AAABXRUJQVlA4TCIAAAAvHUACALkyRPQ/dhHR/wCRtk1d3L/codM5GKiYAKodaP8C")
		encoded.Write(b)
	default:
		err = png.Encode(&encoded, src)
	}
	if err != nil {
		t.Fatal(err)
	}
	raw, _, err := image.Decode(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	// Little-endian TIFF IFD0, one SHORT orientation entry, no next IFD.
	exif := []byte{'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x12, 1, 3, 0, 1, 0, 0, 0, orientation, 0, 0, 0, 0, 0, 0, 0}
	if format == "tiff" {
		data := append([]byte(nil), encoded.Bytes()...)
		order := binary.LittleEndian
		offset := int(order.Uint32(data[4:]))
		n := int(order.Uint16(data[offset:]))
		entries := data[offset+2 : offset+2+12*n]
		ifd := make([]byte, 2)
		order.PutUint16(ifd, uint16(n+1))
		inserted := false
		for len(entries) > 0 {
			if !inserted && order.Uint16(entries) > 0x112 {
				ifd = append(ifd, exif[10:22]...)
				inserted = true
			}
			ifd = append(ifd, entries[:12]...)
			entries = entries[12:]
		}
		if !inserted {
			ifd = append(ifd, exif[10:22]...)
		}
		ifd = append(ifd, 0, 0, 0, 0)
		order.PutUint32(data[4:], uint32(len(data)))
		return append(data, ifd...), raw
	}
	if format == "webp" {
		// Extended header advertises EXIF and the unrotated canvas size.
		data := append([]byte(nil), encoded.Bytes()[:12]...)
		data = append(data, []byte("VP8X\x0a\x00\x00\x00\x08\x00\x00\x00\x1d\x00\x00\x09\x00\x00")...)
		data = append(data, encoded.Bytes()[12:]...)
		data = append(data, []byte{'E', 'X', 'I', 'F', byte(len(exif)), 0, 0, 0}...)
		data = append(data, exif...)
		binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
		return data, raw
	}
	if format == "jpeg" {
		data := append([]byte{0xff, 0xd8, 0xff, 0xe1, 0, byte(8 + len(exif))}, []byte("Exif\x00\x00")...)
		data = append(data, exif...)
		return append(data, encoded.Bytes()[2:]...), raw
	}
	chunk := make([]byte, 12+len(exif))
	binary.BigEndian.PutUint32(chunk, uint32(len(exif)))
	copy(chunk[4:], "eXIf")
	copy(chunk[8:], exif)
	binary.BigEndian.PutUint32(chunk[len(chunk)-4:], crc32.ChecksumIEEE(chunk[4:len(chunk)-4]))
	data := append([]byte(nil), encoded.Bytes()[:33]...) // after IHDR, before IDAT
	data = append(data, chunk...)
	return append(data, encoded.Bytes()[33:]...), raw
}

func TestInlineImageEXIFOrientation(t *testing.T) {
	r := inlineImageRequest{width: 200, height: 200, cols: 20, rows: 10}
	// Source corners in output TL, TR, BL, BR order. This table is independent
	// of the implementation's coordinate transform.
	corners := [8][4]int{{0, 1, 2, 3}, {1, 0, 3, 2}, {3, 2, 1, 0}, {2, 3, 0, 1}, {0, 2, 1, 3}, {2, 0, 3, 1}, {3, 1, 2, 0}, {1, 3, 0, 2}}
	sourcePoints := [4]image.Point{{0, 0}, {29, 0}, {0, 9}, {29, 9}}
	for _, format := range []string{"jpeg", "png", "tiff", "webp"} {
		for orientation := byte(1); orientation <= 8; orientation++ {
			t.Run(fmt.Sprintf("%s/%d", format, orientation), func(t *testing.T) {
				data, raw := inlineOrientedImage(t, format, orientation)
				im := decodeInlineImage(inlinePayload("", data), r)
				w, h := 30, 10
				if orientation >= 5 {
					w, h = h, w
				}
				if im == nil || im.Bounds() != image.Rect(0, 0, w, h) {
					t.Fatalf("oriented dimensions do not match %dx%d", w, h)
				}
				points := [4]image.Point{{0, 0}, {w - 1, 0}, {0, h - 1}, {w - 1, h - 1}}
				for i, p := range points {
					s := sourcePoints[corners[orientation-1][i]]
					want := color.NRGBAModel.Convert(raw.At(s.X, s.Y))
					if got := im.NRGBAAt(p.X, p.Y); got != want {
						t.Fatalf("corner %d: got %v want %v", i, got, want)
					}
				}
				for _, size := range []struct {
					params string
					w, h   int
				}{
					{"width=6px", 6, 6 * h / w}, {"height=6px", 6 * w / h, 6},
					{"width=6px;height=9px;preserveAspectRatio=0", 6, 9},
				} {
					im = decodeInlineImage(inlinePayload(size.params, data), r)
					if im == nil || im.Bounds() != image.Rect(0, 0, size.w, size.h) {
						t.Fatalf("%s: wrong oriented scale", size.params)
					}
				}
			})
		}
	}
}

func TestInlineImageEXIFNativeStripes(t *testing.T) {
	for _, fragmented := range []bool{false, true} {
		t.Run(fmt.Sprint(fragmented), func(t *testing.T) {
			term, replies := inlineTerminal(t)
			data, raw := inlineOrientedImage(t, "png", 6)
			term.Write([]byte("\x1b[20h\x1b[3;4H"))
			seq := append([]byte("\x1b]1337;File="), inlinePayload("", data)...)
			seq = append(seq, []byte("\x07\x1b[6nX")...)
			if fragmented {
				for _, b := range seq {
					term.Write([]byte{b})
				}
			} else {
				term.Write(seq)
			}
			g := term.Graphics(10, 20)
			if replies.String() != "\x1b[5;4R" || len(g) != 2 {
				t.Fatalf("oriented stripe/cursor: %q, stripes %d", replies.String(), len(g))
			}
			for i, stripe := range g {
				if stripe.X != 30 || stripe.Y != 40+20*i || stripe.Width != 10 || stripe.Height != 20 || stripe.Source.Min.Y != 20*i {
					t.Fatalf("stripe %d: %+v", i, stripe)
				}
				got := color.NRGBAModel.Convert(stripe.Image.At(0, 20*i))
				want := color.NRGBAModel.Convert(raw.At(20*i, 9))
				if got != want {
					t.Fatalf("stripe %d pixels: %v want %v", i, got, want)
				}
			}
		})
	}
}

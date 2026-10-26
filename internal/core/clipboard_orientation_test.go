package core

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"oshell-desktop/internal/imageorient"
	"testing"
)

func TestClipboardImageJPEGOrientation(t *testing.T) {
	im := image.NewNRGBA(image.Rect(0, 0, 30, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 30; x++ {
			im.Set(x, y, color.NRGBA{uint8(x * 7), uint8(y * 11), 0, 255})
		}
	}
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, im, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatal(err)
	}
	original, err := jpeg.Decode(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	for orientation := 1; orientation <= 8; orientation++ {
		tiff := make([]byte, 26)
		copy(tiff, "II")
		binary.LittleEndian.PutUint16(tiff[2:], 42)
		binary.LittleEndian.PutUint32(tiff[4:], 8)
		binary.LittleEndian.PutUint16(tiff[8:], 1)
		binary.LittleEndian.PutUint16(tiff[10:], 0x112)
		binary.LittleEndian.PutUint16(tiff[12:], 3)
		binary.LittleEndian.PutUint32(tiff[14:], 1)
		binary.LittleEndian.PutUint16(tiff[18:], uint16(orientation))
		segment := append([]byte("Exif\x00\x00"), tiff...)
		data := append([]byte{0xff, 0xd8, 0xff, 0xe1, 0, byte(len(segment) + 2)}, segment...)
		data = append(data, encoded.Bytes()[2:]...)
		if got := imageorient.Orientation(data); got != orientation {
			t.Fatal(got, orientation)
		}
		png, err := NormalizeClipboardImage(data)
		if err != nil {
			t.Fatal(err)
		}
		decoded, _, err := image.Decode(bytes.NewReader(png))
		if err != nil {
			t.Fatal(err)
		}
		w, h := 30, 20
		if orientation >= 5 {
			w, h = h, w
		}
		if decoded.Bounds().Dx() != w || decoded.Bounds().Dy() != h {
			t.Fatal(orientation, decoded.Bounds())
		}
		// Check distinct corners against an explicit orientation table.
		corners := [][2]int{{0, 0}, {29, 0}, {29, 19}, {0, 19}, {0, 0}, {0, 19}, {29, 19}, {29, 0}}
		want := corners[orientation-1]
		r, g, _, _ := decoded.At(0, 0).RGBA()
		wr, wg, _, _ := original.At(want[0], want[1]).RGBA()
		if dr := int(r>>8) - int(wr>>8); dr < -1 || dr > 1 {
			t.Fatal(orientation, "red", r>>8, want)
		}
		if dg := int(g>>8) - int(wg>>8); dg < -1 || dg > 1 {
			t.Fatal(orientation, "green", g>>8, want)
		}
	}
	for _, data := range [][]byte{nil, []byte("II"), []byte("MM\x00*\xff\xff\xff\xff"), []byte("\xff\xd8\xff\xe1\xff\xff")} {
		if imageorient.Orientation(data) != 1 {
			t.Fatal("invalid metadata")
		}
	}
}

package imageorient

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"testing"
)

func metadata(order binary.ByteOrder, orientation uint16) []byte {
	b := make([]byte, 26)
	if order == binary.LittleEndian {
		copy(b, "II")
	} else {
		copy(b, "MM")
	}
	order.PutUint16(b[2:], 42)
	order.PutUint32(b[4:], 8)
	order.PutUint16(b[8:], 1)
	order.PutUint16(b[10:], 0x112)
	order.PutUint16(b[12:], 3)
	order.PutUint32(b[14:], 1)
	order.PutUint16(b[18:], orientation)
	return b
}

func containers(b []byte) map[string][]byte {
	jpeg := append([]byte{0xff, 0xd8, 0xff, 0xe1, 0, byte(8 + len(b))}, []byte("Exif\x00\x00")...)
	jpeg = append(jpeg, b...)
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 12+len(b))...)
	binary.BigEndian.PutUint32(png[8:], uint32(len(b)))
	copy(png[12:], "eXIf")
	copy(png[16:], b)
	webp := append([]byte("RIFF\x00\x00\x00\x00WEBPJUNK\x01\x00\x00\x00x\x00EXIF"), make([]byte, 4+len(b)+len(b)%2)...)
	binary.LittleEndian.PutUint32(webp[4:], uint32(len(webp)-8))
	binary.LittleEndian.PutUint32(webp[26:], uint32(len(b)))
	copy(webp[30:], b)
	return map[string][]byte{
		"tiff": b, "jpeg": jpeg, "png": png, "webp": webp,
		"jpeg-fill": append([]byte{0xff, 0xd8, 0xff, 0xff, 0xff, 0x01, 0xff}, jpeg[2:]...),
		"jpeg-app0": append([]byte{0xff, 0xd8, 0xff, 0xe0, 0, 4, 1, 2}, jpeg[2:]...),
	}
}

func TestOrientationContainersAndBounds(t *testing.T) {
	for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		for v := uint16(1); v <= 8; v++ {
			for name, b := range containers(metadata(order, v)) {
				t.Run(fmt.Sprintf("%v/%d/%s", order, v, name), func(t *testing.T) {
					if got := Orientation(b); got != int(v) {
						t.Fatalf("got %d", got)
					}
					for n := 0; n < len(b); n++ {
						if got := Orientation(b[:n]); got < 1 || got > 8 {
							t.Fatalf("truncation %d: %d", n, got)
						}
					}
				})
			}
		}
	}
	for _, v := range []uint16{0, 9, 65535} {
		for name, b := range containers(metadata(binary.LittleEndian, v)) {
			if Orientation(b) != 1 {
				t.Fatal("invalid value", name, v)
			}
		}
	}
	valid := containers(metadata(binary.LittleEndian, 6))
	for name, b := range valid {
		b = append([]byte(nil), b...)
		switch name {
		case "tiff":
			binary.LittleEndian.PutUint32(b[4:], 0xffffffff)
		case "jpeg":
			b[4], b[5] = 0xff, 0xff
		case "png":
			binary.BigEndian.PutUint32(b[8:], 0xffffffff)
		case "webp":
			binary.LittleEndian.PutUint32(b[26:], 0xffffffff)
		default:
			continue
		}
		if Orientation(b) != 1 {
			t.Fatal("invalid offset/length", name)
		}
	}
	// RIFF bytes outside the declared container cannot supply trusted metadata.
	b := append([]byte(nil), valid["webp"]...)
	binary.LittleEndian.PutUint32(b[4:], 4)
	if Orientation(b) != 1 {
		t.Fatal("read outside RIFF")
	}
	b = append([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\x00IEND\x00\x00\x00\x00"), valid["png"][8:]...)
	if Orientation(b) != 1 {
		t.Fatal("read past PNG end")
	}
	for _, marker := range []byte{0xda, 0xd9, 0} {
		b := append([]byte{0xff, 0xd8, 0xff, marker}, valid["jpeg"][2:]...)
		if Orientation(b) != 1 {
			t.Fatal("read past JPEG scan/end", marker)
		}
	}
}

func TestOrientationViewOffsetAndAlpha(t *testing.T) {
	src := image.NewNRGBA64(image.Rect(7, 11, 10, 13))
	for y := 11; y < 13; y++ {
		for x := 7; x < 10; x++ {
			src.SetNRGBA64(x, y, color.NRGBA64{uint16(x * 1000), uint16(y * 1000), 42, 32768})
		}
	}
	// Pixel IDs in row-major order after each transform, on a non-square image.
	want := [][]int{{0, 1, 2, 3, 4, 5}, {2, 1, 0, 5, 4, 3}, {5, 4, 3, 2, 1, 0}, {3, 4, 5, 0, 1, 2}, {0, 3, 1, 4, 2, 5}, {3, 0, 4, 1, 5, 2}, {5, 2, 4, 1, 3, 0}, {2, 5, 1, 4, 0, 3}}
	for v := 1; v <= 8; v++ {
		im := Apply(src, metadata(binary.LittleEndian, uint16(v)))
		b := im.Bounds()
		for i, id := range want[v-1] {
			x, y := b.Min.X+i%b.Dx(), b.Min.Y+i/b.Dx()
			r, g, bl, a := im.At(x, y).RGBA()
			wr, wg, wb, wa := src.At(7+id%3, 11+id/3).RGBA()
			if r != wr || g != wg || bl != wb || a != wa {
				t.Fatalf("orientation %d pixel %d", v, i)
			}
			if got := im.(image.RGBA64Image).RGBA64At(x, y); got != (color.RGBA64{uint16(wr), uint16(wg), uint16(wb), uint16(wa)}) {
				t.Fatal("RGBA64 path changed alpha")
			}
		}
	}
}

func FuzzOrientation(f *testing.F) {
	for _, b := range containers(metadata(binary.LittleEndian, 6)) {
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		if got := Orientation(b); got < 1 || got > 8 {
			t.Fatalf("invalid orientation %d", got)
		}
	})
}

package core

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"testing"

	"github.com/gen2brain/h265/heic"
	"oshell-desktop/internal/tiff"
)

// Synthetic metadata: tests check lossless profile transport, not CMM accuracy.
func clipboardTestICC(space string) []byte {
	p := make([]byte, 160)
	binary.BigEndian.PutUint32(p, uint32(len(p)))
	p[8] = 4
	copy(p[12:], "mntr")
	copy(p[16:], space)
	copy(p[20:], "XYZ ")
	copy(p[36:], "acsp")
	binary.BigEndian.PutUint32(p[128:], 1)
	copy(p[132:], "desc")
	binary.BigEndian.PutUint32(p[136:], 144)
	binary.BigEndian.PutUint32(p[140:], 16)
	copy(p[144:], "text")
	copy(p[152:], "test")
	return p
}

func clipboardColorPixels() image.Image {
	im := image.NewNRGBA64(image.Rect(0, 0, 32, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 32; x++ {
			im.SetNRGBA64(x, y, color.NRGBA64{uint16(x * 1901), uint16(y * 3701), 12345, 65535})
		}
	}
	return im
}

func clipboardTestPNG(t *testing.T, im image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, im); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func testPNGChunk(t *testing.T, png []byte, name string) []byte {
	t.Helper()
	if _, _, err := image.DecodeConfig(bytes.NewReader(png)); err != nil {
		t.Fatal(err)
	}
	for pos := 8; pos+12 <= len(png); {
		n := int(binary.BigEndian.Uint32(png[pos:]))
		if n > len(png)-pos-12 {
			t.Fatal("truncated PNG")
		}
		if string(png[pos+4:pos+8]) == name {
			return png[pos+8 : pos+8+n]
		}
		pos += n + 12
	}
	return nil
}

func assertPNGProfile(t *testing.T, encoded, want []byte) {
	t.Helper()
	chunk := testPNGChunk(t, encoded, "iCCP")
	i := bytes.IndexByte(chunk, 0)
	if i < 0 || i+2 > len(chunk) {
		t.Fatal("profile not embedded")
	}
	r, err := zlib.NewReader(bytes.NewReader(chunk[i+2:]))
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(r)
	r.Close()
	if err != nil || !bytes.Equal(got, want) {
		t.Fatal("profile changed", err)
	}
	if _, err = png.Decode(bytes.NewReader(encoded)); err != nil {
		t.Fatal("invalid PNG chunks", err)
	}
}

func TestClipboardPNGColorRoundTrip(t *testing.T) {
	im := clipboardColorPixels()
	profile := clipboardTestICC("RGB ")
	metadata := clipboardColorMetadata{icc: profile, chunks: map[string][]byte{
		"cICP": {12, 13, 0, 1}, "gAMA": {0, 0, 177, 143}, "cHRM": make([]byte, 32),
	}}
	input, err := metadata.insertPNG(clipboardTestPNG(t, im))
	if err != nil {
		t.Fatal(err)
	}
	output, err := NormalizeClipboardImage(input)
	if err != nil {
		t.Fatal(err)
	}
	assertPNGProfile(t, output, profile)
	for kind, want := range metadata.chunks {
		if !bytes.Equal(testPNGChunk(t, output, kind), want) {
			t.Fatal("color metadata lost", kind)
		}
	}
	decoded, _ := png.Decode(bytes.NewReader(output))
	for y := 0; y < 16; y++ {
		for x := 0; x < 32; x++ {
			if color.NRGBA64Model.Convert(decoded.At(x, y)) != color.NRGBA64Model.Convert(im.At(x, y)) {
				t.Fatal("16-bit sample changed", x, y)
			}
		}
	}
	gray := image.NewGray16(image.Rect(0, 0, 2, 1))
	gray.SetGray16(0, 0, color.Gray16{Y: 12345})
	gp := clipboardTestICC("GRAY")
	input, err = (clipboardColorMetadata{icc: gp}).insertPNG(clipboardTestPNG(t, gray))
	if err != nil {
		t.Fatal(err)
	}
	output, err = NormalizeClipboardImage(input)
	if err != nil {
		t.Fatal(err)
	}
	assertPNGProfile(t, output, gp)
}

func TestClipboardPNGHDREncoding(t *testing.T) {
	metadata := clipboardColorMetadata{chunks: map[string][]byte{
		"cICP": {9, 16, 0, 1}, "mDCV": make([]byte, 24), "cLLI": {0, 0, 39, 16, 0, 0, 7, 208},
	}}
	input, err := metadata.insertPNG(clipboardTestPNG(t, clipboardColorPixels()))
	if err != nil {
		t.Fatal(err)
	}
	output, err := NormalizeClipboardImage(input)
	if err != nil {
		t.Fatal(err)
	}
	for kind, want := range metadata.chunks {
		if !bytes.Equal(testPNGChunk(t, output, kind), want) {
			t.Fatal(kind, "HDR metadata changed")
		}
	}
}

func jpegICCPart(seq, count byte, profile []byte) []byte {
	payload := append([]byte("ICC_PROFILE\x00"), seq, count)
	payload = append(payload, profile...)
	segment := binary.BigEndian.AppendUint16([]byte{0xff, 0xe2}, uint16(len(payload)+2))
	return append(segment, payload...)
}

func TestClipboardJPEGMultipartICC(t *testing.T) {
	var jpegData bytes.Buffer
	if err := jpeg.Encode(&jpegData, clipboardColorPixels(), nil); err != nil {
		t.Fatal(err)
	}
	profile := clipboardTestICC("RGB ")
	first, second := jpegICCPart(1, 2, profile[:70]), jpegICCPart(2, 2, profile[70:])
	wrap := func(parts ...[]byte) []byte {
		data := []byte{0xff, 0xd8}
		for _, part := range parts {
			data = append(data, part...)
		}
		return append(data, jpegData.Bytes()[2:]...)
	}
	output, err := NormalizeClipboardImage(wrap(second, first))
	if err != nil {
		t.Fatal(err)
	}
	assertPNGProfile(t, output, profile)
	for _, input := range [][]byte{wrap(first), wrap(first, first, second), wrap(jpegICCPart(0, 1, profile)), wrap(first, jpegICCPart(2, 3, profile[70:]))} {
		if _, err := NormalizeClipboardImage(input); err == nil {
			t.Fatal("accepted invalid multipart profile")
		}
	}
}

func TestClipboardTIFFICC(t *testing.T) {
	var buf bytes.Buffer
	if err := tiff.Encode(&buf, clipboardColorPixels(), nil); err != nil {
		t.Fatal(err)
	}
	data := bytes.Clone(buf.Bytes())
	if string(data[:2]) != "II" {
		t.Fatal("test encoder changed byte order")
	}
	// Relocate the first IFD and append an ICC tag without shifting pixel offsets.
	old := int(binary.LittleEndian.Uint32(data[4:]))
	n := int(binary.LittleEndian.Uint16(data[old:]))
	profile := clipboardTestICC("RGB ")
	profilePos := len(data)
	data = append(data, profile...)
	newIFD := len(data)
	data = binary.LittleEndian.AppendUint16(data, uint16(n+1))
	data = append(data, data[old+2:old+2+n*12]...)
	data = binary.LittleEndian.AppendUint16(data, 34675)
	data = binary.LittleEndian.AppendUint16(data, 7)
	data = binary.LittleEndian.AppendUint32(data, uint32(len(profile)))
	data = binary.LittleEndian.AppendUint32(data, uint32(profilePos))
	data = binary.LittleEndian.AppendUint32(data, 0)
	binary.LittleEndian.PutUint32(data[4:], uint32(newIFD))
	output, err := NormalizeClipboardImage(data)
	if err != nil {
		t.Fatal(err)
	}
	assertPNGProfile(t, output, profile)
	binary.LittleEndian.PutUint32(data[newIFD+2+n*12+8:], 0xfffffffe)
	if _, err := NormalizeClipboardImage(data); err == nil {
		t.Fatal("accepted overflowing profile offset")
	}
}

func TestClipboardHEICColorEncoding(t *testing.T) {
	profile := clipboardTestICC("RGB ")
	var buf bytes.Buffer
	if err := heic.Encode(&buf, clipboardColorPixels(), heic.EncodeOptions{BitDepth: 10, Lossless: true, Chroma: heic.Chroma444, ICC: profile}); err != nil {
		t.Fatal(err)
	}
	output, err := NormalizeClipboardImage(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	assertPNGProfile(t, output, profile)
	if testPNGChunk(t, output, "cICP") != nil {
		t.Fatal("codec nclx overrode ICC")
	}
	buf.Reset()
	if err := heic.Encode(&buf, clipboardColorPixels(), heic.EncodeOptions{BitDepth: 10, Lossless: true, Chroma: heic.Chroma444}); err != nil {
		t.Fatal(err)
	}
	for _, coding := range [][2]byte{{12, 13}, {9, 16}, {9, 18}} {
		data := bytes.Clone(buf.Bytes())
		pos := bytes.Index(data, []byte("nclx"))
		if pos < 0 {
			t.Fatal("fixture missing nclx")
		}
		binary.BigEndian.PutUint16(data[pos+4:], uint16(coding[0]))
		binary.BigEndian.PutUint16(data[pos+6:], uint16(coding[1]))
		output, err := NormalizeClipboardImage(data)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(testPNGChunk(t, output, "cICP"), []byte{coding[0], coding[1], 0, 1}) {
			t.Fatal("HEIC color encoding changed", coding)
		}
		cfg, err := png.DecodeConfig(bytes.NewReader(output))
		if err != nil || cfg.ColorModel != color.RGBA64Model {
			t.Fatal("HDR bit depth lost", cfg, err)
		}
	}
}

func TestClipboardColorInvalidMetadata(t *testing.T) {
	base := clipboardTestPNG(t, clipboardColorPixels())
	var bomb bytes.Buffer
	bomb.WriteString("ICC\x00\x00")
	w := zlib.NewWriter(&bomb)
	w.Write(make([]byte, clipboardICCMax+1))
	w.Close()
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"iCCP", bomb.Bytes()}, {"iCCP", []byte("ICC\x00\x01bad")},
		{"gAMA", []byte{0, 0, 0, 0}}, {"cICP", []byte{1, 13, 6, 1}},
		{"cICP", []byte{1, 13, 0, 2}}, {"sRGB", []byte{4}}, {"cHRM", []byte{1}},
		{"mDCV", make([]byte, 24)}, {"cLLI", make([]byte, 7)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := appendClipboardPNGChunk(bytes.Clone(base[:33]), tc.name, tc.data)
			input = append(input, base[33:]...)
			if _, err := NormalizeClipboardImage(input); err == nil {
				t.Fatal("invalid color metadata accepted")
			}
		})
	}
	for _, space := range []string{"CMYK", "Lab ", "GRAY"} {
		if _, err := (clipboardColorMetadata{icc: clipboardTestICC(space)}).insertPNG(base); err == nil {
			t.Fatal("wrong profile attached to RGB", space)
		}
	}
	profile := clipboardTestICC("RGB ")
	binary.BigEndian.PutUint32(profile[140:], 0xfffffff0)
	if validateClipboardICC(profile) == nil {
		t.Fatal("overflowing ICC tag accepted")
	}
}

func TestClipboardWebPICCMetadata(t *testing.T) {
	profile := clipboardTestICC("RGB ")
	data := []byte("RIFF\x00\x00\x00\x00WEBPICCP")
	data = binary.LittleEndian.AppendUint32(data, uint32(len(profile)))
	data = append(data, profile...)
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
	c, err := clipboardImageColors(data, "webp")
	if err != nil || !bytes.Equal(c.icc, profile) {
		t.Fatal("WebP profile changed", err)
	}
	duplicate := append(bytes.Clone(data), data[12:]...)
	if c, err := clipboardImageColors(duplicate, "webp"); err != nil || !bytes.Equal(c.icc, profile) {
		t.Fatal("parsed bytes beyond RIFF body", err)
	}
	binary.LittleEndian.PutUint32(duplicate[4:], uint32(len(duplicate)-8))
	for _, invalid := range [][]byte{data[:len(data)-1], duplicate} {
		if _, err := clipboardImageColors(invalid, "webp"); err == nil {
			t.Fatal("invalid WebP profile accepted")
		}
	}
}

package core

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"

	"github.com/gen2brain/h265/heic"
)

const clipboardICCMax = 4 << 20

var errClipboardColor = errors.New("图片颜色配置损坏、过大或与像素格式不匹配")

// The decoders leave RGB samples in their source color space. Retain the
// description in the PNG, instead of silently labelling wide-gamut data sRGB.
// Only color metadata is copied; EXIF orientation has already been applied.
type clipboardColorMetadata struct {
	icc    []byte
	chunks map[string][]byte
}

func clipboardHEICColors(info heic.ColorInfo) (clipboardColorMetadata, error) {
	c := clipboardColorMetadata{icc: info.ICCP}
	if len(c.icc) != 0 {
		// nclx may describe the codec matrix alongside an ICC profile. Do not
		// let a generated cICP override the file's explicit RGB profile.
		return c, validateClipboardICC(c.icc)
	}
	if info.Primaries > 255 || info.Transfer > 255 {
		return c, errClipboardColor
	}
	if info.Primaries != 0 && info.Primaries != 2 && info.Transfer != 0 && info.Transfer != 2 {
		// HEIC decoding has applied the YCbCr matrix and expanded the range,
		// but has not changed primaries or transfer. PNG samples are RGB/full.
		c.chunks = map[string][]byte{"cICP": {byte(info.Primaries), byte(info.Transfer), 0, 1}}
	}
	return c, nil
}

func clipboardImageColors(data []byte, format string) (c clipboardColorMetadata, err error) {
	switch format {
	case "png":
		return clipboardPNGColors(data)
	case "jpeg":
		c.icc, err = clipboardJPEGICC(data)
	case "tiff":
		c.icc, err = clipboardTIFFICC(data)
	case "webp":
		if len(data) < 12 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
			return c, errClipboardColor
		}
		end := uint64(binary.LittleEndian.Uint32(data[4:])) + 8
		if end < 12 || end > uint64(len(data)) {
			return c, errClipboardColor
		}
		data = data[:end] // Ignore bytes outside the image's declared RIFF body.
		for pos := 12; pos+8 <= len(data); {
			n := uint64(binary.LittleEndian.Uint32(data[pos+4:]))
			if n > uint64(len(data)-pos-8) {
				return c, errClipboardColor
			}
			if string(data[pos:pos+4]) == "ICCP" {
				if c.icc != nil || n == 0 {
					return c, errClipboardColor
				}
				c.icc = data[pos+8 : pos+8+int(n)]
			}
			pos += 8 + int(n) + int(n%2)
		}
	}
	if err == nil && c.icc != nil {
		err = validateClipboardICC(c.icc)
	}
	return c, err
}

// Bound both the profile and its tag table. No profile instructions are executed.
func validateClipboardICC(profile []byte) error {
	if len(profile) < 132 || len(profile) > clipboardICCMax ||
		uint64(binary.BigEndian.Uint32(profile)) != uint64(len(profile)) || string(profile[36:40]) != "acsp" {
		return errClipboardColor
	}
	// A CMYK/Lab profile cannot describe the RGB PNG produced by image/png.
	if space := string(profile[16:20]); space != "RGB " && space != "GRAY" {
		return errors.New("暂不支持此图片的颜色空间，请先将图片转换为 RGB 或灰度后粘贴")
	}
	count := uint64(binary.BigEndian.Uint32(profile[128:132]))
	start := uint64(132) + count*12
	if start > uint64(len(profile)) {
		return errClipboardColor
	}
	for pos := 132; uint64(pos) < start; pos += 12 {
		offset := uint64(binary.BigEndian.Uint32(profile[pos+4:]))
		size := uint64(binary.BigEndian.Uint32(profile[pos+8:]))
		if offset < start || offset%4 != 0 || size < 8 || offset+size > uint64(len(profile)) {
			return errClipboardColor
		}
	}
	return nil
}

func clipboardPNGColors(data []byte) (c clipboardColorMetadata, err error) {
	c.chunks = make(map[string][]byte)
	for pos := 8; pos+12 <= len(data); {
		n := uint64(binary.BigEndian.Uint32(data[pos:]))
		if n > uint64(len(data)-pos-12) {
			return c, errClipboardColor
		}
		kind := string(data[pos+4 : pos+8])
		if kind == "IDAT" || kind == "IEND" {
			break
		}
		value := data[pos+8 : pos+8+int(n)]
		switch kind {
		case "iCCP", "sRGB", "gAMA", "cHRM", "cICP", "mDCV", "cLLI":
			if _, exists := c.chunks[kind]; exists || crc32.ChecksumIEEE(data[pos+4:pos+8+int(n)]) != binary.BigEndian.Uint32(data[pos+8+int(n):]) {
				return c, errClipboardColor
			}
			switch kind {
			case "iCCP":
				c.icc, err = decodeClipboardICCP(value)
				if err != nil {
					return c, err
				}
			case "sRGB":
				if len(value) != 1 || value[0] > 3 {
					return c, errClipboardColor
				}
			case "gAMA":
				if len(value) != 4 || binary.BigEndian.Uint32(value) == 0 {
					return c, errClipboardColor
				}
			case "cHRM":
				if len(value) != 32 {
					return c, errClipboardColor
				}
			case "cICP":
				if len(value) != 4 || value[2] != 0 || value[3] > 1 {
					return c, errClipboardColor
				}
			case "mDCV":
				if len(value) != 24 {
					return c, errClipboardColor
				}
			case "cLLI":
				if len(value) != 8 {
					return c, errClipboardColor
				}
			}
			c.chunks[kind] = value
		}
		pos += int(n) + 12
	}
	if c.chunks["mDCV"] != nil && c.chunks["cICP"] == nil {
		return c, errClipboardColor
	}
	if len(c.icc) != 0 {
		// The decoder's RGB representation of gray+alpha is not the source
		// color space. Validate against IHDR before deciding how to encode it.
		if len(data) < 33 || ((data[25] == 0 || data[25] == 4) != (string(c.icc[16:20]) == "GRAY")) {
			return c, errClipboardColor
		}
	}
	return c, nil
}

func decodeClipboardICCP(data []byte) ([]byte, error) {
	nameEnd := bytes.IndexByte(data, 0)
	if nameEnd < 1 || nameEnd > 79 || nameEnd+2 >= len(data) || data[nameEnd+1] != 0 || len(data) > clipboardICCMax {
		return nil, errClipboardColor
	}
	r, err := zlib.NewReader(bytes.NewReader(data[nameEnd+2:]))
	if err != nil {
		return nil, errClipboardColor
	}
	profile, err := io.ReadAll(io.LimitReader(r, clipboardICCMax+1))
	closeErr := r.Close()
	if err != nil || closeErr != nil || validateClipboardICC(profile) != nil {
		return nil, errClipboardColor
	}
	return profile, nil
}

func clipboardJPEGICC(data []byte) ([]byte, error) {
	var parts [256][]byte
	count, total := 0, 0
	for pos := 2; pos < len(data); {
		if data[pos] != 0xff {
			return nil, errClipboardColor
		}
		for pos < len(data) && data[pos] == 0xff {
			pos++
		}
		if pos >= len(data) {
			return nil, errClipboardColor
		}
		marker := data[pos]
		pos++
		if marker == 0xda || marker == 0xd9 {
			break
		}
		if marker == 0x01 || marker >= 0xd0 && marker <= 0xd7 {
			continue
		}
		if pos+2 > len(data) {
			return nil, errClipboardColor
		}
		n := int(binary.BigEndian.Uint16(data[pos:]))
		if n < 2 || n > len(data)-pos {
			return nil, errClipboardColor
		}
		payload := data[pos+2 : pos+n]
		if marker == 0xe2 && bytes.HasPrefix(payload, []byte("ICC_PROFILE\x00")) {
			if len(payload) <= 14 {
				return nil, errClipboardColor
			}
			seq, all := int(payload[12]), int(payload[13])
			if seq == 0 || seq > all || count != 0 && count != all || parts[seq] != nil {
				return nil, errClipboardColor
			}
			count = all
			parts[seq] = payload[14:]
			total += len(parts[seq])
			if total > clipboardICCMax {
				return nil, errClipboardColor
			}
		}
		pos += n
	}
	if count == 0 {
		return nil, nil
	}
	profile := make([]byte, 0, total)
	for i := 1; i <= count; i++ {
		if parts[i] == nil {
			return nil, errClipboardColor
		}
		profile = append(profile, parts[i]...)
	}
	return profile, nil
}

func clipboardTIFFICC(data []byte) ([]byte, error) {
	if len(data) < 8 {
		return nil, errClipboardColor
	}
	var order binary.ByteOrder = binary.BigEndian
	if string(data[:4]) == "II*\x00" {
		order = binary.LittleEndian
	} else if string(data[:4]) != "MM\x00*" {
		return nil, errClipboardColor
	}
	pos := uint64(order.Uint32(data[4:]))
	if pos < 8 || pos+2 > uint64(len(data)) {
		return nil, errClipboardColor
	}
	count := uint64(order.Uint16(data[pos:]))
	pos += 2
	if pos+12*count+4 > uint64(len(data)) {
		return nil, errClipboardColor
	}
	var profile []byte
	photo := uint16(0xffff)
	for end := pos + 12*count; pos < end; pos += 12 {
		if order.Uint16(data[pos:]) == 262 {
			if photo != 0xffff || order.Uint16(data[pos+2:]) != 3 || order.Uint32(data[pos+4:]) != 1 {
				return nil, errClipboardColor
			}
			photo = order.Uint16(data[pos+8:])
		}
		if order.Uint16(data[pos:]) != 34675 {
			continue
		}
		n := uint64(order.Uint32(data[pos+4:]))
		offset := uint64(order.Uint32(data[pos+8:]))
		if profile != nil || order.Uint16(data[pos+2:]) != 7 || n < 132 || n > clipboardICCMax || offset+n > uint64(len(data)) {
			return nil, errClipboardColor
		}
		profile = data[offset : offset+n]
	}
	if len(profile) != 0 && ((photo == 0 || photo == 1) != (string(profile[16:20]) == "GRAY")) {
		return nil, errClipboardColor
	}
	return profile, nil
}

func appendClipboardPNGChunk(dst []byte, name string, data []byte) []byte {
	dst = binary.BigEndian.AppendUint32(dst, uint32(len(data)))
	start := len(dst)
	dst = append(dst, name...)
	dst = append(dst, data...)
	return binary.BigEndian.AppendUint32(dst, crc32.ChecksumIEEE(dst[start:]))
}

func (c clipboardColorMetadata) insertPNG(png []byte) ([]byte, error) {
	if len(c.icc) == 0 && len(c.chunks) == 0 {
		return png, nil
	}
	// This is image/png output: the signature and 13-byte IHDR occupy 33 bytes.
	if len(png) < 33 {
		return nil, errClipboardColor
	}
	var extra []byte
	if len(c.icc) != 0 {
		if err := validateClipboardICC(c.icc); err != nil {
			return nil, err
		}
		gray := png[25] == 0 || png[25] == 4
		if gray != (string(c.icc[16:20]) == "GRAY") {
			return nil, errClipboardColor
		}
		var compressed bytes.Buffer
		compressed.WriteString("ICC\x00\x00")
		w := zlib.NewWriter(&compressed)
		_, err := w.Write(c.icc)
		err = errors.Join(err, w.Close())
		if err != nil {
			return nil, err
		}
		extra = appendClipboardPNGChunk(extra, "iCCP", compressed.Bytes())
	}
	for _, kind := range []string{"cICP", "sRGB", "gAMA", "cHRM", "mDCV", "cLLI"} {
		if value := c.chunks[kind]; value != nil && !(kind == "sRGB" && len(c.icc) != 0) {
			extra = appendClipboardPNGChunk(extra, kind, value)
		}
	}
	result := make([]byte, 0, len(png)+len(extra))
	result = append(result, png[:33]...)
	result = append(result, extra...)
	return append(result, png[33:]...), nil
}

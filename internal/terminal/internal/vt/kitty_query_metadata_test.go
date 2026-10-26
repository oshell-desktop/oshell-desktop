package vt

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"image"
	"strings"
	"testing"
)

// A valid PNG header and end marker, with no pixel stream. Image dimensions
// are available independently of whether a raster can be produced.
func kittyMetadataPNG(width, height uint32) []byte {
	data := []byte("\x89PNG\r\n\x1a\n")
	chunk := func(kind string, payload []byte) {
		data = binary.BigEndian.AppendUint32(data, uint32(len(payload)))
		start := len(data)
		data = append(data, kind...)
		data = append(data, payload...)
		data = binary.BigEndian.AppendUint32(data, crc32.ChecksumIEEE(data[start:]))
	}
	header := binary.BigEndian.AppendUint32(nil, width)
	header = binary.BigEndian.AppendUint32(header, height)
	header = append(header, 8, 6, 0, 0, 0)
	chunk("IHDR", header)
	chunk("IEND", nil)
	return data
}

func TestKittyQueryMetadataWithoutPixels(t *testing.T) {
	for _, compressed := range []bool{false, true} {
		for _, fragmented := range []bool{false, true} {
			t.Run(fmt.Sprintf("compressed%v/fragmented%v", compressed, fragmented), func(t *testing.T) {
				term, replies := kittyCacheTerminal(t)
				term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=71,p=3,c=1,r=1,C=1", []byte{255, 0, 0}))
				before := term.Graphics(10, 20)
				term.Write([]byte("\x1b[4;7H"))
				x, y := term.Cursor()
				data := kittyMetadataPNG(300, 100)
				control := "a=q,f=100,i=71,I=72,p=3,s=0,v=10001,t=f,S=999,O=999"
				if fragmented {
					control += ",t=d"
				}
				if compressed {
					var b bytes.Buffer
					w := zlib.NewWriter(&b)
					_, _ = w.Write(data)
					if err := w.Close(); err != nil {
						t.Fatal(err)
					}
					data, control = b.Bytes(), control+",o=z"
				}
				for _, quiet := range []bool{false, true} {
					replies.Reset()
					c := control
					if quiet {
						c += ",q=1"
					}
					if fragmented {
						encoded := base64.StdEncoding.EncodeToString(data)
						term.Write(kittyEncodedCommand(c+",m=1", encoded[:7]))
						if replies.Len() != 0 {
							t.Fatal("metadata query replied before final fragment")
						}
						for _, b := range kittyEncodedCommand("m=0", encoded[7:]) {
							term.Write([]byte{b})
						}
					} else {
						term.Write(kittyCommand(c, data))
					}
					want := "\x1b_Gi=71,I=72,p=3;OK\x1b\\"
					if quiet {
						want = ""
					}
					if replies.String() != want {
						t.Fatalf("metadata-only query: got %q, want %q", replies.String(), want)
					}
					if cx, cy := term.Cursor(); cx != x || cy != y {
						t.Fatal("query moved cursor", cx, cy)
					}
					after := term.Graphics(10, 20)
					if len(after) != 1 || after[0].Image != before[0].Image {
						t.Fatal("query changed existing raster")
					}
				}
				replies.Reset()
				term.Write(kittyCommand("a=p,i=71,p=4,C=1", nil))
				g := term.Graphics(10, 20)
				if len(g) != 2 || !bytes.Equal(g[1].Image.Pix, []byte{255, 0, 0, 255}) {
					t.Fatal("query replaced cached image", replies.String())
				}
				term.Write(kittyCommand("a=p,i=72", nil))
				if !strings.HasSuffix(replies.String(), "\x1b_Gi=72;ENOENT: image not found\x1b\\") {
					t.Fatal("query created image", replies.String())
				}
			})
		}
	}
}

func TestKittyQueryMetadataDimensions(t *testing.T) {
	for _, tc := range []struct {
		width, height uint32
		valid         bool
	}{
		{1, 1, true}, {10000, 10000, true}, {10001, 1, false},
		{1, 10001, false}, {0, 1, false}, {1, 0, false}, {0xffffffff, 1, false},
	} {
		term, replies := kittyCacheTerminal(t)
		term.Write(kittyCommand("a=q,f=100", kittyMetadataPNG(tc.width, tc.height)))
		want := "\x1b_G;OK\x1b\\"
		if !tc.valid {
			want = "\x1b_G;EINVAL: bad payload\x1b\\"
		}
		if replies.String() != want {
			t.Fatal("metadata dimensions", tc.width, tc.height, replies.String())
		}
		if len(term.Graphics(10, 20)) != 0 {
			t.Fatal("metadata query produced pixels")
		}
	}
}

func TestKittyQueryEncodedMetadataFormats(t *testing.T) {
	for _, format := range []string{"png", "jpeg", "gif", "bmp", "tiff", "webp"} {
		t.Run(format, func(t *testing.T) {
			data, _ := kittyFormatFixture(t, format)
			var header []byte
			if format == "tiff" {
				// TIFF puts its directory after the pixels. Point the single
				// strip beyond EOF while preserving its dimensions and metadata.
				header = bytes.Clone(data)
				offset := int(binary.LittleEndian.Uint32(header[4:]))
				count := int(binary.LittleEndian.Uint16(header[offset:]))
				changed := false
				for i := 0; i < count; i++ {
					entry := header[offset+2+i*12:]
					if binary.LittleEndian.Uint16(entry) == 273 {
						if binary.LittleEndian.Uint32(entry[4:]) != 1 {
							t.Fatal("fixture must have one strip")
						}
						binary.LittleEndian.PutUint32(entry[8:], uint32(len(header)+1))
						changed = true
					}
				}
				if !changed {
					t.Fatal("fixture has no strip")
				}
			} else {
				for end := 1; end < len(data); end++ {
					if _, _, err := image.DecodeConfig(bytes.NewReader(data[:end])); err == nil {
						header = data[:end]
						break
					}
				}
			}
			if len(header) == 0 {
				t.Fatal("fixture has no separate metadata")
			}
			if _, _, err := image.DecodeConfig(bytes.NewReader(header)); err != nil {
				t.Fatal("fixture metadata unreadable", err)
			}
			if _, _, err := image.Decode(bytes.NewReader(header)); err == nil {
				t.Fatal("fixture unexpectedly has decodable pixels")
			}
			term, replies := kittyCacheTerminal(t)
			term.Write(kittyCommand("a=q,f=100,i=71", header))
			if replies.String() != "\x1b_Gi=71;OK\x1b\\" {
				t.Fatal("metadata rejected", replies.String())
			}
		})
	}
}

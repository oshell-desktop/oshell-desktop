package vt

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"fmt"
	"image"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/image/bmp"
)

func TestKittyEncodedTransmitDefersPixels(t *testing.T) {
	for _, compressed := range []bool{false, true} {
		t.Run(fmt.Sprint(compressed), func(t *testing.T) {
			term, replies := kittyCacheTerminal(t)
			term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=71,p=3,c=1,r=1,C=1", []byte{255, 0, 0}))
			old := term.Graphics(10, 20)
			data := kittyMetadataPNG(30, 10)
			control := "a=t,f=100,i=71"
			if compressed {
				var b bytes.Buffer
				w := zlib.NewWriter(&b)
				_, _ = w.Write(data)
				if err := w.Close(); err != nil {
					t.Fatal(err)
				}
				data, control = b.Bytes(), control+",o=z"
			}
			replies.Reset()
			encoded := base64.StdEncoding.EncodeToString(data)
			term.Write(kittyEncodedCommand(control+",m=1", encoded[:7]))
			if replies.Len() != 0 {
				t.Fatal("early transmit reply")
			}
			for _, b := range kittyEncodedCommand("m=0", encoded[7:]) {
				term.Write([]byte{b})
			}
			if replies.String() != "\x1b_Gi=71;OK\x1b\\" {
				t.Fatal("metadata transmit", replies.String())
			}
			if g := term.Graphics(10, 20); len(g) != 1 || g[0].Image != old[0].Image {
				t.Fatal("transmit changed old raster")
			}
			for _, crop := range []string{"x=1", "x=1000", "w=1,h=1"} {
				replies.Reset()
				term.Write(kittyCommand("a=p,i=71,p=3,C=1,"+crop, nil))
				if replies.String() != "\x1b_Gi=71;ENOTSUP: cannot crop png\x1b\\" {
					t.Fatal("crop decode precedence", crop, replies.String())
				}
				if g := term.Graphics(10, 20); len(g) != 1 || g[0].Image != old[0].Image {
					t.Fatal("failed crop replaced old raster")
				}
			}
			// Virtual placement needs metadata only and can act as a parent.
			replies.Reset()
			term.Write(kittyCommand("a=p,i=71,p=4,U=1,c=3,r=1", nil))
			if replies.String() != "\x1b_Gi=71,p=4;OK\x1b\\" {
				t.Fatal("virtual placement decoded pixels", replies.String())
			}
			term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=72,p=1,P=71,Q=4,c=1,r=1,C=1", []byte{0, 0, 255}))
			if !strings.HasSuffix(replies.String(), "\x1b_Gi=72,p=1;OK\x1b\\") || len(term.Graphics(10, 20)) != 2 {
				t.Fatal("metadata virtual parent unavailable", replies.String())
			}
		})
	}
}

func TestKittyEncodedEvictionBeforeFirstRaster(t *testing.T) {
	term, replies := kittyCacheTerminal(t)
	src := image.NewNRGBA(image.Rect(0, 0, 1400, 1100))
	for i := 0; i < len(src.Pix); i += 4 {
		src.Pix[i], src.Pix[i+3] = 255, 255
	}
	var data bytes.Buffer
	if err := bmp.Encode(&data, src); err != nil {
		t.Fatal(err)
	}
	if data.Len() <= KittyImageLimit {
		t.Fatal("fixture must exceed encoded cache quota")
	}
	term.Write(kittyCommand("a=T,f=100,i=71,p=1,c=3,r=1,C=1", data.Bytes()))
	if replies.String() != "\x1b_Gi=71,p=1;OK\x1b\\" {
		t.Fatal("evicted transmission failed", replies.String())
	}
	g := term.Graphics(10, 20)
	if len(g) != 1 || g[0].Image.Bounds() != src.Bounds() || !bytes.Equal(g[0].Image.Pix, src.Pix) {
		t.Fatal("cache eviction freed payload before deferred decode")
	}
	replies.Reset()
	term.Write(kittyCommand("a=p,i=71,p=2", nil))
	if replies.String() != "\x1b_Gi=71;ENOENT: image not found\x1b\\" {
		t.Fatal("oversized encoded image remained cached", replies.String())
	}
	if g = term.Graphics(10, 20); len(g) != 1 {
		t.Fatal("cache miss removed retained raster")
	}
	term.Write(kittyCommand("a=d,d=I,i=71", nil))
	if len(term.Graphics(10, 20)) != 0 {
		t.Fatal("free retained encoded raster failed")
	}
}

func TestKittyEncodedLazyRasterMemory(t *testing.T) {
	term, replies := kittyCacheTerminal(t)
	imageBytes := func() uint64 {
		// Public GhosttyTerminalMemoryUsage, including the explicit size field.
		var usage struct {
			size       uintptr
			compressed bool
			fields     [12]uint64
		}
		usage.size = unsafe.Sizeof(usage)
		if err := term.get(42, unsafe.Pointer(&usage)); err != nil {
			t.Fatal(err)
		}
		return usage.fields[5] + usage.fields[11]
	}
	data, raw := kittyFormatFixture(t, "png")
	encoded := uint64(len(data))
	pixels := uint64(raw.Bounds().Dx() * raw.Bounds().Dy() * 4)
	for round := 0; round < 12; round++ {
		replies.Reset()
		term.Write(kittyCommand("a=t,f=100,i=71", data))
		if imageBytes() != encoded {
			t.Fatal("transmit decoded or miscounted payload", imageBytes(), encoded)
		}
		term.Write(kittyCommand("a=p,i=71,p=1,U=1", nil))
		if imageBytes() != encoded {
			t.Fatal("virtual placement decoded pixels", imageBytes())
		}
		term.Write(kittyCommand("a=p,i=71,p=2,C=1", nil))
		if imageBytes() != encoded+pixels {
			t.Fatal("raster and encoding memory not counted once", imageBytes(), encoded+pixels)
		}
		term.Write([]byte("\x1b[?47h"))
		term.Write(kittyCommand("a=p,i=71,p=3,C=1", nil))
		if imageBytes() != encoded+pixels {
			t.Fatal("cross-screen placement duplicated raster", imageBytes())
		}
		term.Write(kittyCommand("a=d,d=I,i=71", nil))
		if imageBytes() != 0 {
			t.Fatal("final deletion retained encoded or decoded allocation", imageBytes())
		}
		term.Write([]byte("\x1b[?47l"))
	}
	large := kittyMetadataPNG(10000, 10000)
	term.Write(kittyCommand("a=T,f=100,i=71,U=1", large))
	if imageBytes() != uint64(len(large)) {
		t.Fatal("large virtual image allocated raster memory", imageBytes())
	}
	term.Write(kittyCommand("a=d,d=I,i=71", nil))
	if imageBytes() != 0 {
		t.Fatal("large virtual image retained bytes")
	}
}

func TestKittyAnonymousDisplayErrorsRemainVisible(t *testing.T) {
	for _, quiet := range []bool{false, true} {
		for _, tc := range []struct {
			control string
			data    []byte
			want    string
		}{
			{"f=100,x=1", kittyMetadataPNG(30, 10), "ENOTSUP: cannot crop png"},
			{"f=100,x=1,P=99,Q=1", kittyMetadataPNG(30, 10), "ENOPARENT: parent placement not found"},
			{"f=24,s=1,v=1,x=2", []byte{255, 0, 0}, "EINVAL: bad crop"},
		} {
			term, replies := kittyCacheTerminal(t)
			control := "a=T," + tc.control
			if quiet {
				control += ",q=1"
			}
			term.Write(kittyCommand(control, tc.data))
			want := "\x1b_G;" + tc.want + "\x1b\\"
			if quiet {
				want = ""
			}
			if replies.String() != want {
				t.Fatal("anonymous display error", tc.control, replies.String(), want)
			}
		}
	}
}

func TestKittyEncodedDisplayFailureStages(t *testing.T) {
	for _, tc := range []struct {
		control, want string
		remains       bool
	}{
		{"x=1", "ENOTSUP: cannot crop png", true},
		{"x=1000", "ENOTSUP: cannot crop png", true},
		{"x=1,P=99,Q=1", "ENOPARENT: parent placement not found", true},
		{"U=1,P=99", "EINVAL: virtual placement cannot refer to parent", true},
		{"", "OK", false},
	} {
		t.Run(tc.control, func(t *testing.T) {
			term, replies := kittyCacheTerminal(t)
			term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=71,p=3,c=1,r=1,C=1", []byte{255, 0, 0}))
			old := term.Graphics(10, 20)
			replies.Reset()
			term.Write(kittyCommand("a=T,f=100,i=71,p=3,C=1,"+tc.control, kittyMetadataPNG(30, 10)))
			want := "\x1b_Gi=71;" + tc.want + "\x1b\\"
			if tc.want == "OK" {
				want = "\x1b_Gi=71,p=3;OK\x1b\\"
			}
			if replies.String() != want {
				t.Fatal("display failure stage", replies.String(), want)
			}
			g := term.Graphics(10, 20)
			if tc.remains {
				if len(g) != 1 || g[0].Image != old[0].Image {
					t.Fatal("pre-display failure removed old pixels")
				}
			} else if len(g) != 0 {
				t.Fatal("failed uncropped render kept replaced pixels")
			}
			// Even a failed display has replaced the cached payload.
			replies.Reset()
			term.Write(kittyCommand("a=p,i=71,x=1", nil))
			if replies.String() != "\x1b_Gi=71;ENOTSUP: cannot crop png\x1b\\" {
				t.Fatal("failed display did not commit payload", replies.String())
			}
		})
	}
}

func TestKittyEncodedFailedRenderCursorAndNumber(t *testing.T) {
	term, replies := kittyCacheTerminal(t)
	term.Write([]byte("\x1b[4;7H"))
	term.Write(kittyCommand("a=T,f=100,I=72,p=3", kittyMetadataPNG(30, 10)))
	if replies.String() != "\x1b_Gi=1,I=72,p=3;OK\x1b\\" {
		t.Fatal("metadata did not allocate numbered identity", replies.String())
	}
	if x, y := term.Cursor(); x != 9 || y != 4 {
		t.Fatal("failed render did not apply protocol cursor", x, y)
	}
	replies.Reset()
	term.Write(kittyCommand("a=p,I=72,x=1", nil))
	if replies.String() != "\x1b_GI=72;ENOTSUP: cannot crop png\x1b\\" {
		t.Fatal("number lookup lost failed image", replies.String())
	}
	term.Write(kittyCommand("a=d,d=N,I=72", nil))
	replies.Reset()
	term.Write(kittyCommand("a=p,I=72", nil))
	if replies.String() != "\x1b_GI=72;ENOENT: image not found\x1b\\" {
		t.Fatal("metadata image survived free", replies.String())
	}
}

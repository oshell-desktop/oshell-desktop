package vt

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

func kittyCommand(control string, data []byte) []byte {
	return []byte("\x1b_G" + control + ";" + base64.StdEncoding.EncodeToString(data) + "\x1b\\")
}

func graphicTerminal(t *testing.T) (*Terminal, *strings.Builder) {
	t.Helper()
	loadLib(t)
	replies := new(strings.Builder)
	term, err := NewTerminal(20, 10, Effects{WritePTY: func(data []byte) { replies.Write(data) }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(term.Free)
	if err = term.Resize(20, 10, 10, 20); err != nil {
		t.Fatal(err)
	}
	return term, replies
}

func TestKittyGraphicsPixelsGeometryAndReplacement(t *testing.T) {
	term, replies := graphicTerminal(t)
	raw := []byte{255, 0, 0, 255, 0, 255, 0, 128}
	term.Write(kittyCommand("a=T,f=32,s=2,v=1,i=7,p=1,c=2,r=2,C=1,X=3,Y=4,z=-1", raw))
	if !strings.Contains(replies.String(), "i=7,p=1;OK") {
		t.Fatal(replies.String())
	}
	images := term.Graphics(10, 20)
	if len(images) != 1 {
		t.Fatal("missing placement", len(images))
	}
	g := images[0]
	if g.X != 3 || g.Y != 4 || g.Width != 20 || g.Height != 40 || g.Z != -1 || g.Source != image.Rect(0, 0, 2, 1) {
		t.Fatalf("geometry %+v", g)
	}
	if !bytes.Equal(g.Image.Pix, raw) {
		t.Fatal("RGBA pixels", g.Image.Pix)
	}
	if again := term.Graphics(10, 20); again[0].Image != g.Image {
		t.Fatal("unchanged image was recopied")
	}
	if err := term.Resize(20, 10, 12, 24); err != nil {
		t.Fatal(err)
	}
	if resized := term.Graphics(12, 24); len(resized) != 2 || resized[0].Width != 20 || resized[0].Height != 40 || resized[0].Image != g.Image ||
		!resized[0].Clipped || resized[0].RasterClip != image.Rect(3, 4, 23, 24) || resized[1].RasterClip != image.Rect(3, 28, 23, 48) {
		t.Fatal("resize must space frozen raster stripes on the new grid", resized)
	}
	term.Write(kittyCommand("a=t,f=32,s=2,v=1,i=7", []byte{0, 0, 255, 255, 255, 255, 0, 255}))
	term.Write(kittyCommand("a=p,i=7,p=1,c=2,r=2,C=1", nil))
	replaced := term.Graphics(12, 24)
	if len(replaced) != 1 || replaced[0].Width != 24 || replaced[0].Height != 48 || replaced[0].Generation == g.Generation || replaced[0].Image == g.Image || replaced[0].Image.Pix[2] != 255 {
		t.Fatal("same-size replacement stale", replaced)
	}
	if !bytes.Equal(g.Image.Pix, raw) {
		t.Fatal("native mutation changed snapshot")
	}
	term.Write(kittyCommand("a=d,d=I,i=7", nil))
	if len(term.Graphics(10, 20)) != 0 || len(term.graphicsCache) != 0 {
		t.Fatal("delete retained images")
	}
}

func TestKittyGraphicsPNGChunkingAndCompression(t *testing.T) {
	term, replies := graphicTerminal(t)
	im := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	im.SetNRGBA(0, 0, color.NRGBA{R: 123, G: 45, B: 67, A: 128})
	im.SetNRGBA(1, 0, color.NRGBA{R: 40, G: 200, B: 100, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, im); err != nil {
		t.Fatal(err)
	}
	packet := kittyCommand("a=T,f=100,i=8,c=2,r=1,C=1", buf.Bytes())
	for _, b := range packet {
		term.Write([]byte{b})
	}
	images := term.Graphics(10, 20)
	if len(images) != 1 || !bytes.Equal(images[0].Image.Pix, im.Pix) {
		t.Fatal("PNG callback/fragmented escape failed", replies.String(), images)
	}
	buf.Reset()
	z := zlib.NewWriter(&buf)
	z.Write([]byte{4, 5, 6})
	z.Close()
	term.Write(kittyCommand("a=T,f=24,o=z,s=1,v=1,i=9,c=1,r=1,C=1", buf.Bytes()))
	images = term.Graphics(10, 20)
	if len(images) != 2 || !bytes.Equal(images[1].Image.Pix, []byte{4, 5, 6, 255}) {
		t.Fatal("zlib RGB failed", replies.String(), images)
	}
	// Protocol chunks, independently base64 encoded, are different from input
	// stream fragmentation. Neither may expose an incomplete image.
	term.Write(kittyCommand("a=T,f=24,s=2,v=1,i=10,c=2,r=1,C=1,m=1", []byte{7, 8, 9}))
	if len(term.Graphics(10, 20)) != 2 {
		t.Fatal("partial image visible")
	}
	term.Write(kittyCommand("m=0", []byte{10, 11, 12}))
	images = term.Graphics(10, 20)
	if len(images) != 3 || !bytes.Equal(images[2].Image.Pix, []byte{7, 8, 9, 255, 10, 11, 12, 255}) {
		t.Fatal("multipart transfer failed", replies.String(), images)
	}
}

func TestKittyGraphicsScrollScreenIsolationAndCrop(t *testing.T) {
	term, _ := graphicTerminal(t)
	term.Write(kittyCommand("a=T,f=24,s=2,v=1,i=1,c=2,r=1,C=1,x=1,w=1", []byte{255, 0, 0, 0, 255, 0}))
	g := term.Graphics(10, 20)
	if len(g) != 1 || g[0].Source != image.Rect(1, 0, 2, 1) {
		t.Fatal("crop", g)
	}
	term.Write([]byte("\x1b[?1049h"))
	if len(term.Graphics(10, 20)) != 0 {
		t.Fatal("main image leaked into alternate screen")
	}
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=1,c=1,r=1,C=1", []byte{0, 0, 255}))
	alt := term.Graphics(10, 20)
	if len(alt) != 1 || alt[0].Image.Pix[2] != 255 {
		t.Fatal("alternate image missing")
	}
	term.Write([]byte("\x1b[?1049l"))
	if main := term.Graphics(10, 20); len(main) != 1 || main[0].Generation == alt[0].Generation || main[0].Image.Pix[0] != 255 {
		t.Fatal("screen cache identity collision")
	}
	term.Write([]byte(strings.Repeat("\r\n", 12)))
	if len(term.Graphics(10, 20)) != 0 {
		t.Fatal("offscreen image stayed in viewport")
	}
	term.ScrollBy(-20)
	if len(term.Graphics(10, 20)) != 1 {
		t.Fatal("scrollback image disappeared")
	}
	term.Write([]byte("\x1bc"))
	if len(term.Graphics(10, 20)) != 0 {
		t.Fatal("reset retained image")
	}
}

func TestKittyGraphicsRejectInvalidAndLocalFileMedia(t *testing.T) {
	term, replies := graphicTerminal(t)
	var pngData bytes.Buffer
	png.Encode(&pngData, image.NewNRGBA(image.Rect(0, 0, 1, 1)))
	oversized := bytes.Clone(pngData.Bytes())
	binary.BigEndian.PutUint32(oversized[16:], KittyImageLimit)
	binary.BigEndian.PutUint32(oversized[29:], crc32.ChecksumIEEE(oversized[12:29]))
	overflow := bytes.Clone(pngData.Bytes())
	binary.BigEndian.PutUint32(overflow[16:], 1<<31-1)
	binary.BigEndian.PutUint32(overflow[20:], 1<<31-1)
	binary.BigEndian.PutUint32(overflow[29:], crc32.ChecksumIEEE(overflow[12:29]))
	for _, packet := range [][]byte{
		kittyCommand("a=T,f=100,i=6", oversized),
		kittyCommand("a=T,f=100,i=7", overflow),
		kittyCommand("a=T,f=100,i=1", []byte("not png")),
		kittyCommand("a=T,f=32,s=1,v=1,i=2", []byte{1, 2, 3}),
		kittyCommand("a=T,f=32,s=65535,v=65535,i=3", []byte{1, 2, 3, 4}),
		kittyCommand("a=T,f=100,t=f,i=4", []byte("private-local-file.png")),
		kittyCommand("a=T,f=32,t=s,i=5,s=1,v=1", []byte("shared-object")),
	} {
		replies.Reset()
		term.Write(packet)
		if strings.Contains(replies.String(), ";OK") || len(term.Graphics(10, 20)) != 0 {
			t.Fatal("invalid/forbidden image accepted", replies.String())
		}
	}
}

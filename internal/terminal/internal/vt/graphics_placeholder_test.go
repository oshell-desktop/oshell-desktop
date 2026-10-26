package vt

import (
	"fmt"
	"image"
	"image/color"
	"testing"
)

func TestKittyVirtualRenderSnapshot(t *testing.T) {
	term, replies := graphicTerminal(t)
	term.Write(kittyCommand("a=T,U=1,f=24,s=1,v=1,i=7,p=9,c=3,r=2,X=2,Y=3,x=99,z=-4", []byte{255, 0, 0}))
	if len(term.Graphics(10, 20)) != 0 {
		t.Fatal("virtual placement acquired a cursor raster")
	}
	g := term.RenderGraphics(10, 20)
	if len(g) != 1 {
		t.Fatalf("no virtual snapshot: %s", replies.String())
	}
	a := g[0]
	if !a.Virtual || a.LogicalImageID() != 7 || a.PlacementID != 9 || a.Columns != 3 || a.Rows != 2 || a.X != 2 || a.Y != 3 || a.Source != image.Rect(0, 0, 1, 1) {
		t.Fatalf("virtual metadata: %+v", a)
	}
	if term.RenderGraphics(10, 20)[0].Image != a.Image {
		t.Fatal("unchanged snapshot recopied pixels")
	}
	term.Write(kittyCommand("a=t,f=24,s=1,v=1,i=7", []byte{0, 255, 0}))
	g = term.RenderGraphics(10, 20)
	if len(g) != 1 || g[0].Image.NRGBAAt(0, 0) != (color.NRGBA{0, 255, 0, 255}) || g[0].Generation == a.Generation {
		t.Fatal("virtual placement failed to resolve current image")
	}
	term.Write([]byte("\x1b[?1049h"))
	if len(term.RenderGraphics(10, 20)) != 0 {
		t.Fatal("primary virtual placement leaked to alternate buffer")
	}
	term.Write([]byte("\x1b[?1049l"))
	if len(term.RenderGraphics(10, 20)) != 1 {
		t.Fatal("primary virtual placement lost")
	}
}

func TestKittyVirtualEncodedOrientation(t *testing.T) {
	for orientation := byte(1); orientation <= 8; orientation++ {
		t.Run(fmt.Sprint(orientation), func(t *testing.T) {
			term, replies := graphicTerminal(t)
			data, _ := inlineOrientedImage(t, "png", orientation)
			term.Write(kittyCommand("a=T,U=1,f=100,i=7,p=1,c=3,r=2", data))
			g := term.RenderGraphics(10, 20)
			if len(g) != 1 {
				t.Fatalf("missing encoded virtual image: %s", replies.String())
			}
			if g[0].Image != nil || g[0].lazyImage == nil || g[0].lazyImage.decoded != nil {
				t.Fatal("virtual metadata eagerly decoded pixels without a placeholder")
			}
			w, h := 30, 10
			if orientation >= 5 {
				w, h = h, w
			}
			if g[0].Source != image.Rect(0, 0, w, h) {
				t.Fatal("virtual encoded orientation", g[0].Source)
			}
			decoded := g[0].ResolveImage()
			if decoded == nil || g[0].lazyImage.encoded != nil {
				t.Fatal("lazy decode did not release owned compressed bytes")
			}
			again := term.RenderGraphics(10, 20)
			if again[0].ResolveImage() != decoded {
				t.Fatal("unchanged encoded generation decoded again")
			}
			term.Write(kittyCommand("a=p,i=7,p=2,C=1,w=30,h=10", nil))
			g = term.RenderGraphics(10, 20)
			if len(g) != 2 || g[0].ResolveImage() == g[1].ResolveImage() {
				t.Fatal("virtual and raw crop shared an incompatible pixel view")
			}
		})
	}
}

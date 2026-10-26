package vt

import (
	"fmt"
	"image"
	"reflect"
	"testing"
	"time"
	"unsafe"
)

func kittyStripeFixture(t *testing.T) *Terminal {
	term, _ := graphicTerminal(t)
	data := make([]byte, 2*100*3)
	for y := 0; y < 100; y++ {
		for x := 0; x < 2; x++ {
			data[(y*2+x)*3] = byte(30 + y/20*40)
		}
	}
	term.Write(kittyCommand("a=T,f=24,s=2,v=100,i=91,p=1,C=1", data))
	return term
}

func TestKittyStripesLongInsertionKeepsOnlyVisibleTail(t *testing.T) {
	term, _ := graphicTerminal(t)
	term.Write([]byte("\x1b[?1049h\x1b[10;1H"))
	start := time.Now()
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=91,p=1,c=1,r=100000,C=1", []byte{255, 0, 0}))
	t.Logf("100000 raster row insertion: %s", time.Since(start))
	g := term.Graphics(10, 20)
	if len(g) != 1 || !g[0].Clipped || g[0].RasterClip != image.Rect(0, 0, 10, 180) {
		t.Fatal("long insertion lost or duplicated its visible tail", g)
	}
	term.Write(kittyCommand("a=d,d=I,i=91", nil))
	if len(term.Graphics(10, 20)) != 0 {
		t.Fatal("long insertion retained stripes after deletion")
	}
}

func TestKittyStripesReleasePixelsAfterLastRow(t *testing.T) {
	term, _ := graphicTerminal(t)
	for i := 0; i < 12; i++ {
		term.Write([]byte("\x1b[r\x1b[H"))
		term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=91,p=1,c=1,r=5,C=1", []byte{255, 0, 0}))
		g := term.Graphics(10, 20)
		if len(g) != 1 {
			t.Fatal("missing display", g)
		}
		id := g[0].ImageID
		term.Write([]byte("\x1b[4M"))
		if len(term.Graphics(10, 20)) != 1 {
			t.Fatal("last stripe disappeared with earlier rows")
		}
		term.Write([]byte("\x1b[M"))
		if len(term.Graphics(10, 20)) != 0 {
			t.Fatal("last deleted stripe remained visible")
		}
		var storage uintptr
		if err := term.get(30, unsafe.Pointer(&storage)); err != nil {
			t.Fatal(err)
		}
		if call(fnGraphicsImage, storage, uintptr(id)) != 0 {
			t.Fatal("last deleted stripe retained its display allocation")
		}
	}
}

func kittyStripeRows(t *testing.T, term *Terminal, cw, ch int) map[int]int {
	t.Helper()
	rows := make(map[int]int)
	for _, g := range term.Graphics(cw, ch) {
		for i := 0; i < 5; i++ {
			sample := image.Pt(1, i*20+5)
			if !sample.In(g.Source) {
				continue
			}
			x := g.X + (sample.X-g.Source.Min.X)*g.Width/g.Source.Dx()
			y := g.Y + (sample.Y-g.Source.Min.Y)*g.Height/g.Source.Dy()
			if g.Clipped && !image.Pt(x, y).In(g.RasterClip) {
				continue
			}
			if _, exists := rows[i]; exists {
				t.Fatalf("source stripe %d painted twice: %+v", i, term.Graphics(cw, ch))
			}
			rows[i] = (y - 5) / ch
		}
	}
	return rows
}

func TestKittyStripesFollowIndependentBufferRows(t *testing.T) {
	for _, tc := range []struct {
		name, command string
		want          map[int]int
	}{
		{"insert", "\x1b[3;1H\x1b[L", map[int]int{0: 0, 1: 1, 2: 3, 3: 4, 4: 5}},
		{"delete", "\x1b[3;1H\x1b[M", map[int]int{0: 0, 1: 1, 3: 2, 4: 3}},
		{"scroll-region", "\x1b[2;4r\x1b[S", map[int]int{0: 0, 2: 1, 3: 2, 4: 4}},
		{"reverse-region", "\x1b[2;4r\x1b[T", map[int]int{0: 0, 1: 2, 2: 3, 4: 4}},
		{"margin-explicit", "\x1b[?69h\x1b[5;9s\x1b[2;4r\x1b[S", map[int]int{0: 0, 1: 1, 2: 2, 3: 3, 4: 4}},
		{"narrow-index", "\x1b[?69h\x1b[5;9s\x1b[2;4r\x1b[4;6H\n", map[int]int{0: 0, 4: 4}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			term := kittyStripeFixture(t)
			term.Write([]byte(tc.command))
			if got := kittyStripeRows(t, term, 10, 20); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("rows %v, want %v; graphics %+v", got, tc.want, term.Graphics(10, 20))
			}
		})
	}
}

func TestKittyStripesKeepRasterSizeAcrossFontChanges(t *testing.T) {
	for _, ch := range []int{10, 40} {
		t.Run(fmt.Sprint(ch), func(t *testing.T) {
			term := kittyStripeFixture(t)
			if err := term.Resize(20, 10, 15, ch); err != nil {
				t.Fatal(err)
			}
			want := map[int]int{0: 0, 1: 1, 2: 2, 3: 3, 4: 4}
			if got := kittyStripeRows(t, term, 15, ch); !reflect.DeepEqual(got, want) {
				t.Fatalf("font change moved raster sources: %v", got)
			}
			for _, g := range term.Graphics(15, ch) {
				if g.Width != 2 || g.Height != g.Source.Dy() {
					t.Fatalf("font change rescaled frozen pixels: %+v", g)
				}
			}
			term.Write([]byte("\x1b[2;4r\x1b[S"))
			want = map[int]int{0: 0, 2: 1, 3: 2, 4: 4}
			if got := kittyStripeRows(t, term, 15, ch); !reflect.DeepEqual(got, want) {
				t.Fatalf("resized row movement: %v, want %v", got, want)
			}
		})
	}
}

func TestKittyVisibleStripeDeletionPreservesHistory(t *testing.T) {
	for _, identity := range []string{"i=91,p=1", "i=91", "p=1"} {
		for _, command := range []string{"a", "A"} {
			t.Run(identity+"/"+command, func(t *testing.T) {
				term, _ := graphicTerminal(t)
				term.Write([]byte("\x1b[10;1H"))
				term.Write(kittyCommand("a=T,f=100,"+identity+",C=1", inlinePNG(t, 2, 240)))
				g := term.Graphics(10, 20)
				if len(g) != 1 {
					t.Fatal("missing initial tail", g)
				}
				id := g[0].ImageID
				term.Write(kittyCommand("a=d,d="+command, nil))
				if len(term.Graphics(10, 20)) != 0 {
					t.Fatal("visible stripes survived deletion")
				}
				term.ScrollToTop()
				g = term.Graphics(10, 20)
				if len(g) != 1 || g[0].Source.Min.Y != 0 || g[0].Y != 180 {
					t.Fatal("visible deletion removed history stripes", g)
				}
				term.Write([]byte("\x1b[3J"))
				if len(term.Graphics(10, 20)) != 0 {
					t.Fatal("history erase left a stripe")
				}
				var storage uintptr
				if err := term.get(30, unsafe.Pointer(&storage)); err != nil {
					t.Fatal(err)
				}
				if call(fnGraphicsImage, storage, uintptr(id)) != 0 {
					t.Fatal("history erase retained stripe pixels")
				}
			})
		}
	}
}

func TestKittyRelativeStripesPreserveGapsAndUseSurvivingOrigin(t *testing.T) {
	term, _ := graphicTerminal(t)
	term.Write([]byte("\x1b[2;1H"))
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=7,p=7,U=1", []byte{255, 0, 0}))
	term.Write(kittyCommand("a=T,f=100,i=91,p=1,P=7,Q=7,V=1", inlinePNG(t, 2, 100)))
	term.Write([]byte("\x1b[5;1H\x1b[L\x1b[1;1H"))
	term.Write(kittyCommand("a=p,i=7,p=7,U=1", nil))
	term.Write([]byte("\x1b[10;1H\n"))
	want := map[int]int{0: 0, 1: 1, 2: 3, 3: 4, 4: 5}
	if got := kittyStripeRows(t, term, 10, 20); !reflect.DeepEqual(got, want) {
		t.Fatalf("relative move collapsed gaps: %v, want %v", got, want)
	}
	term.Write([]byte("\x1b[1;1H\x1b[M\x1b[4;1H"))
	term.Write(kittyCommand("a=p,i=7,p=7,U=1", nil))
	term.Write([]byte("\x1b[10;1H\n"))
	want = map[int]int{1: 3, 2: 5, 3: 6, 4: 7}
	if got := kittyStripeRows(t, term, 10, 20); !reflect.DeepEqual(got, want) {
		t.Fatalf("relative move used the deleted first stripe: %v, want %v", got, want)
	}
}

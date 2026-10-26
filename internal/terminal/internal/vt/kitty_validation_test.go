package vt

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"strings"
	"testing"
)

func TestKittyPayloadErrorResponses(t *testing.T) {
	for _, action := range []string{"q", "t", "T"} {
		for _, tc := range []struct {
			name, control string
			data          []byte
		}{
			{"missing dimensions", "f=24", []byte{1, 2, 3}},
			{"zero width", "f=24,s=0,v=1", []byte{1, 2, 3}},
			{"oversized width", "f=24,s=10001,v=1", []byte{1, 2, 3}},
			{"short rgb", "f=24,s=2,v=1", []byte{1, 2, 3}},
			{"long rgb", "f=24,s=1,v=1", []byte{1, 2, 3, 4}},
			{"short rgba", "f=32,s=1,v=1", []byte{1, 2, 3}},
			{"unknown format", "f=99,s=1,v=1", []byte{1, 2, 3}},
			{"explicit zero format", "f=0,s=1,v=1", []byte{1, 2, 3, 4}},
			{"empty", "f=24,s=1,v=1", nil},
			{"png invalid", "f=100", []byte{1, 2, 3}},
			{"zlib invalid", "f=24,s=1,v=1,o=z", []byte{1, 2, 3}},
			{"unknown compression", "f=24,s=1,v=1,o=x", []byte{1, 2, 3}},
		} {
			t.Run(action+"/"+tc.name, func(t *testing.T) {
				term, replies := kittyCacheTerminal(t)
				term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=71,p=4,c=1,r=1,C=1", []byte{255, 0, 0}))
				before := term.Graphics(10, 20)
				for _, quiet := range []int{0, 1} {
					replies.Reset()
					packet := kittyCommand(fmt.Sprintf("a=%s,i=71,p=4,q=%d,%s", action, quiet, tc.control), tc.data)
					for _, b := range packet {
						term.Write([]byte{b})
					}
					want := "\x1b_Gi=71;EINVAL: bad payload\x1b\\"
					if quiet != 0 {
						want = ""
					}
					if replies.String() != want {
						t.Fatalf("got %q, want %q", replies.String(), want)
					}
					after := term.Graphics(10, 20)
					if len(after) != 1 || after[0].Image != before[0].Image {
						t.Fatal("invalid payload changed old display")
					}
				}
				replies.Reset()
				term.Write(kittyCommand("a=p,i=71,p=5,c=1,r=1,C=1", nil))
				g := term.Graphics(10, 20)
				if len(g) != 2 || !bytes.Equal(g[1].Image.Pix, []byte{255, 0, 0, 255}) {
					t.Fatal("invalid payload replaced protocol cache", replies.String(), g)
				}
			})
		}
	}
}

func TestKittyQueryIgnoresTransmissionMedium(t *testing.T) {
	for _, medium := range []string{"d", "f", "t", "s", "X", "file"} {
		term, replies := kittyCacheTerminal(t)
		// Swift query validates the supplied bytes directly, even when t/m/S/O
		// would describe a file or unfinished upload for a transmit action.
		more := ",m=1"
		if medium == "d" {
			more = ",m=0"
		}
		term.Write(kittyCommand("a=q,f=24,s=1,v=1,i=71,I=72,p=3,t="+medium+",S=999,O=999"+more, []byte{1, 2, 3}))
		if replies.String() != "\x1b_Gi=71,I=72,p=3;OK\x1b\\" {
			t.Fatal("query interpreted medium", medium, replies.String())
		}
		replies.Reset()
		term.Write(kittyCommand("a=p,i=71", nil))
		if replies.String() != "\x1b_Gi=71;ENOENT: image not found\x1b\\" {
			t.Fatal("query populated cache", replies.String())
		}
	}
}

func TestKittyQueryPreservesExistingImageAndCursor(t *testing.T) {
	term, replies := kittyCacheTerminal(t)
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=71,p=1,c=1,r=1,C=1", []byte{255, 0, 0}))
	before := term.Graphics(10, 20)
	term.Write([]byte("\x1b[5;7H"))
	var compressed bytes.Buffer
	w := zlib.NewWriter(&compressed)
	_, _ = w.Write([]byte{0, 255, 0})
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		control string
		data    []byte
	}{
		{"f=24,s=1,v=1,t=f,o=z", compressed.Bytes()},
		{"f=100,t=X", inlinePNG(t, 2, 1)},
	} {
		replies.Reset()
		term.Write(kittyCommand("a=q,i=71,I=72,p=1,c=20,r=10,P=999,Q=1,U=1,"+tc.control, tc.data))
		if replies.String() != "\x1b_Gi=71,I=72,p=1;OK\x1b\\" {
			t.Fatal("query performed display validation", replies.String())
		}
		if x, y := term.Cursor(); x != 6 || y != 4 {
			t.Fatal("query moved cursor", x, y)
		}
		after := term.Graphics(10, 20)
		if len(after) != 1 || after[0].Image != before[0].Image {
			t.Fatal("query changed existing display")
		}
	}
	replies.Reset()
	term.Write(kittyCommand("a=p,i=71,p=2,c=1,r=1,C=1", nil))
	g := term.Graphics(10, 20)
	if len(g) != 2 || !bytes.Equal(g[1].Image.Pix, []byte{255, 0, 0, 255}) {
		t.Fatal("query overwrote cache", replies.String(), g)
	}
}

func TestKittyFailedVirtualPutTouchesCache(t *testing.T) {
	term, replies := kittyCacheTerminal(t)
	data := bytes.Repeat([]byte{1, 2, 3, 255}, 1024*512)
	for _, id := range []int{71, 72} {
		term.Write(kittyCommand(fmt.Sprintf("a=t,f=32,s=1024,v=512,i=%d", id), data))
	}
	replies.Reset()
	term.Write(kittyCommand("a=p,i=71,U=1,P=99,Q=1", nil))
	if replies.String() != "\x1b_Gi=71;EINVAL: virtual placement cannot refer to parent\x1b\\" {
		t.Fatal(replies.String())
	}
	term.Write(kittyCommand("a=t,f=32,s=1024,v=512,i=73", data))
	replies.Reset()
	term.Write(kittyCommand("a=p,i=72,U=1", nil))
	if !strings.Contains(replies.String(), "ENOENT") {
		t.Fatal("failed put did not update cache access ordering", replies.String())
	}
	replies.Reset()
	term.Write(kittyCommand("a=p,i=71,U=1", nil))
	if !strings.Contains(replies.String(), ";OK") {
		t.Fatal("recently accessed image was evicted", replies.String())
	}
	// The failed put and missing image must not consume virtual placement IDs.
	replies.Reset()
	term.Write(kittyCommand("a=p,i=73,p=1,P=71,Q=1,c=1,r=1,C=1", nil))
	if !strings.Contains(replies.String(), ";OK") {
		t.Fatal("failed virtual display consumed automatic placement ID", replies.String())
	}
}

func TestKittyFailureIdentityAndPriority(t *testing.T) {
	for _, tc := range []struct{ control, want string }{
		{"a=p", "\x1b_G;ENOENT: image not found\x1b\\"},
		{"a=p,U=1,P=99,Q=1", "\x1b_G;ENOENT: image not found\x1b\\"},
		{"a=p,i=71,I=72,p=3,U=1,P=99,Q=1", "\x1b_Gi=71;ENOENT: image not found\x1b\\"},
		{"a=q,i=71,I=72,p=3,f=24", "\x1b_Gi=71;EINVAL: bad payload\x1b\\"},
		{"a=q,I=72,p=3,f=24", "\x1b_GI=72;EINVAL: bad payload\x1b\\"},
		{"a=q,f=24,s=1,v=1", "\x1b_G;EINVAL: bad payload\x1b\\"},
		{"a=t,i=71,I=72,p=3,t=X,o=X", "\x1b_Gi=71;EINVAL: i and I are mutually exclusive\x1b\\"},
		{"a=t,i=71,p=3,t=X,o=X", "\x1b_Gi=71;ENOTSUP: unsupported transmission\x1b\\"},
		{"a=t,t=X", "\x1b_G;ENOTSUP: unsupported transmission\x1b\\"},
		{"a=d,d=i", "\x1b_G;EINVAL: missing image id\x1b\\"},
		{"a=d,d=n", "\x1b_G;EINVAL: missing image number\x1b\\"},
		{"a=d,d=p,x=1", "\x1b_G;EINVAL: missing cell position\x1b\\"},
		{"a=d,d=q,y=1", "\x1b_G;EINVAL: missing cell position\x1b\\"},
		{"a=d,d=x", "\x1b_G;EINVAL: missing column\x1b\\"},
		{"a=d,d=y", "\x1b_G;EINVAL: missing row\x1b\\"},
		{"a=d,d=r,x=1", "\x1b_G;EINVAL: missing id range\x1b\\"},
		{"a=d,d=b,i=71,I=72,p=3", "\x1b_Gi=71;EINVAL: unsupported delete\x1b\\"},
	} {
		t.Run(tc.control, func(t *testing.T) {
			term, replies := kittyCacheTerminal(t)
			term.Write(kittyEncodedCommand(tc.control, "=bad"))
			if replies.String() != tc.want {
				t.Fatalf("got %q, want %q", replies.String(), tc.want)
			}
		})
	}
}

func TestKittyUnsupportedDeleteCleanup(t *testing.T) {
	for _, mode := range []string{"b", "B", "f", "F", "1"} {
		for _, quiet := range []int{0, 1} {
			term, replies := kittyCacheTerminal(t)
			term.Write(kittyCommand("a=t,f=24,s=1,v=1,i=71", []byte{1, 2, 3}))
			term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=72,p=1,c=1,r=1,C=1", []byte{4, 5, 6}))
			replies.Reset()
			term.Write(kittyEncodedCommand(fmt.Sprintf("a=d,d=%s,i=99,q=%d", mode, quiet), "ignored=payload"))
			want := "\x1b_Gi=99;EINVAL: unsupported delete\x1b\\"
			if quiet != 0 {
				want = ""
			}
			if replies.String() != want {
				t.Fatalf("d=%s got %q, want %q", mode, replies.String(), want)
			}
			replies.Reset()
			term.Write(kittyCommand("a=p,i=71,p=2,c=1,r=1,C=1", nil))
			freed := strings.ToUpper(mode) == mode
			if strings.Contains(replies.String(), "ENOENT") != freed {
				t.Fatal("unsupported delete cleanup differs", mode, replies.String())
			}
			replies.Reset()
			term.Write(kittyCommand("a=p,i=72,p=2,c=1,r=1,C=1", nil))
			if !strings.Contains(replies.String(), ";OK") {
				t.Fatal("cleanup freed still-used image", mode, replies.String())
			}
		}
	}
}

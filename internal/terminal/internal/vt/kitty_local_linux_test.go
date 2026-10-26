package vt

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKittyPOSIXSharedMemoryMedia(t *testing.T) {
	term, replies := kittyCacheTerminal(t)
	for _, tc := range []struct {
		control     string
		ok, removed bool
	}{
		{"f=24,s=1,v=1", true, true},
		{"f=24,s=1,v=1,S=9223372036854775807", true, true},
		{"f=24,s=1,v=1,O=1", false, true},
		{"f=24,s=1,v=1,S=-1", false, false},
		{"f=24,s=1,v=1,O=-1", false, false},
		{"f=24,s=1,v=1,O=9223372036854775807", false, true},
		{"f=99,s=1,v=1", false, false},
		{"f=24,s=1,v=1,o=X", false, true},
	} {
		t.Run(tc.control, func(t *testing.T) {
			f, err := os.CreateTemp("/dev/shm", "oshell-kitty-fixture-")
			if err != nil {
				t.Fatal(err)
			}
			path := f.Name()
			t.Cleanup(func() { os.Remove(path) })
			data := make([]byte, 4096)
			copy(data, []byte{1, 2, 3})
			_, err = f.Write(data)
			closeErr := f.Close()
			if err != nil || closeErr != nil {
				t.Fatal(err, closeErr)
			}
			replies.Reset()
			term.Write(kittyCommand("a=T,t=s,i=71,p=1,c=1,r=1,C=1,"+tc.control, []byte("/"+filepath.Base(path))))
			if strings.Contains(replies.String(), ";OK") != tc.ok {
				t.Fatal(replies.String())
			}
			_, err = os.Stat(path)
			if os.IsNotExist(err) != tc.removed {
				t.Fatal("shared-memory lifetime", err)
			}
			if tc.ok {
				g := term.Graphics(10, 20)
				if len(g) != 1 || !bytes.Equal(g[0].Image.Pix, []byte{1, 2, 3, 255}) {
					t.Fatal("shared-memory pixels", g)
				}
			}
		})
	}
}

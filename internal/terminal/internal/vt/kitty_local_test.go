package vt

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestKittyWindowsSharedMemoryRejectsFilePathsAndBadPayload(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("OShell's Windows branch")
	}
	term, replies := kittyCacheTerminal(t)
	path := filepath.Join(t.TempDir(), "tty-graphics-protocol-rgb")
	data := []byte{1, 2, 3}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	for _, medium := range []string{"s"} {
		for _, action := range []string{"t", "T"} {
			for _, payload := range []string{"", "=bad", "AA==", "//8="} {
				for _, quiet := range []int{0, 1, 2} {
					replies.Reset()
					term.Write(kittyEncodedCommand(fmt.Sprintf("a=%s,t=%s,i=71,p=2,f=99,o=X,q=%d", action, medium, quiet), payload))
					want := "\x1b_Gi=71;EINVAL: bad payload\x1b\\"
					if quiet != 0 {
						want = ""
					}
					if got := replies.String(); got != want {
						t.Fatalf("%s/%s/%q/q=%d: %q, want %q", medium, action, payload, quiet, got, want)
					}
				}
			}
			replies.Reset()
			term.Write(kittyCommand("a="+action+",t="+medium+",f=24,s=1,v=1,i=71", []byte(path)))
			if !strings.Contains(replies.String(), "EINVAL") || len(term.Graphics(10, 20)) != 0 {
				t.Fatal("Windows read local image", replies.String())
			}
		}
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, data) {
		t.Fatal("Windows local-media request changed the file", err)
	}
}

func TestKittyPOSIXLocalFileMedia(t *testing.T) {
	term, replies := kittyCacheTerminal(t)
	dir := t.TempDir()
	for _, medium := range []string{"f", "t"} {
		path := filepath.Join(dir, "tty-graphics-protocol-rgb")
		if err := os.WriteFile(path, []byte{9, 9, 1, 2, 3}, 0600); err != nil {
			t.Fatal(err)
		}
		replies.Reset()
		term.Write(kittyCommand("a=T,t="+medium+",f=24,s=1,v=1,i=71,p=1,c=1,r=1,C=1,O=2,S=9223372036854775807", []byte(path)))
		g := term.Graphics(10, 20)
		if !strings.Contains(replies.String(), ";OK") || len(g) != 1 || !bytes.Equal(g[0].Image.Pix, []byte{1, 2, 3, 255}) {
			t.Fatal("file pixels", medium, replies.String(), g)
		}
		_, err := os.Stat(path)
		if (medium == "t") != os.IsNotExist(err) {
			t.Fatal("file lifetime", medium, err)
		}
	}
	// Invalid pixel/compression fields are checked after temporary data is
	// read, so they must not retain the successfully opened temporary file.
	path := filepath.Join(dir, "tty-graphics-protocol-invalid")
	if err := os.WriteFile(path, []byte{1, 2, 3}, 0600); err != nil {
		t.Fatal(err)
	}
	replies.Reset()
	term.Write(kittyCommand("a=t,t=t,f=99,o=X,i=71", []byte(path)))
	if !strings.Contains(replies.String(), "EINVAL: bad payload") {
		t.Fatal(replies.String())
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("failed decode retained temporary file", err)
	}
}

func TestP2KittyLocalFileMissingAndRange(t *testing.T) {
	term, replies := kittyCacheTerminal(t)
	file := filepath.Join(t.TempDir(), "tty-graphics-protocol-ranges")
	for _, controls := range []string{"O=0", "O=4"} {
		if controls == "O=4" {
			if err := os.WriteFile(file, []byte{1, 2, 3}, 0600); err != nil {
				t.Fatal(err)
			}
		}
		replies.Reset()
		term.Write(kittyCommand("a=T,t=f,f=24,s=1,v=1,i=72,"+controls, []byte(file)))
		if strings.Contains(replies.String(), ";OK") || replies.Len() == 0 || len(term.Graphics(10, 20)) != 0 {
			t.Fatal("invalid medium acknowledged", replies.String())
		}
	}
}

package vt

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows"
	"oshell-desktop/internal/testsupport/winshm"
)

func TestI22WindowsSharedMemoryChild(t *testing.T) {
	if os.Getenv("OSHELL_SHARED_MEMORY_CHILD") != "1" {
		t.Skip("child fixture only")
	}
	m := winshm.New(t, []byte{4, 5, 6}, 0, 0)
	fmt.Fprintln(os.Stdout, m.Name)
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	m.Close()
}

func TestI22WindowsSharedMemoryCrossProcess(t *testing.T) {
	term, replies := kittyCacheTerminal(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-test.run=^TestI22WindowsSharedMemoryChild$")
	cmd.Env = append(os.Environ(), "OSHELL_SHARED_MEMORY_CHILD=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { in.Close(); cmd.Wait() }()
	name, err := bufio.NewReader(out).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	name = strings.TrimSpace(name)
	if !strings.HasPrefix(name, "oshell-kitty-") {
		t.Fatal("bad producer handshake", name)
	}
	term.Write(kittyCommand("a=T,t=s,f=24,s=1,v=1,i=71,c=1,r=1,C=1", []byte("/"+name)))
	g := term.Graphics(10, 20)
	if !strings.Contains(replies.String(), ";OK") || len(g) != 1 || !bytes.Equal(g[0].Image.Pix, []byte{4, 5, 6, 255}) {
		t.Fatal("cross-process pixels", replies.String())
	}
	if _, err = in.Write([]byte("release\n")); err != nil {
		t.Fatal(err)
	}
	in.Close()
	if err = cmd.Wait(); err != nil {
		t.Fatal(err, stderr.String())
	}
	if winshm.Exists(name) {
		t.Fatal("shared section survives all owners closing")
	}
}

func TestI22WindowsSharedMemoryEncodedOffset(t *testing.T) {
	term, replies := kittyCacheTerminal(t)
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.SetRGBA(0, 0, color.RGBA{12, 34, 56, 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	m := winshm.New(t, append([]byte{9, 9}, buf.Bytes()...), 0, 0)
	term.Write(kittyCommand(fmt.Sprintf("a=T,t=s,f=100,O=2,S=%d,i=71,c=1,r=1,C=1", buf.Len()), []byte(m.Name)))
	g := term.Graphics(10, 20)
	if !strings.Contains(replies.String(), ";OK") || len(g) != 1 || !bytes.Equal(g[0].Image.Pix, []byte{12, 34, 56, 255}) {
		t.Fatal("encoded offset", replies.String())
	}
	m.Close()
	if winshm.Exists(m.Name) {
		t.Fatal("encoded mapping leaked")
	}
	// Raw ranges clamp an enormous requested length to the fixed pixel size.
	m = winshm.New(t, []byte{1, 2, 3}, 0, 0)
	replies.Reset()
	term.Write(kittyCommand("a=T,t=s,f=24,s=1,v=1,S=9223372036854775807,i=71,c=1,r=1,C=1", []byte(m.Name)))
	if !strings.Contains(replies.String(), ";OK") {
		t.Fatal("clamped length", replies.String())
	}
}

func TestI22WindowsSharedMemoryPixelsAndLifetime(t *testing.T) {
	for _, alias := range []string{"/", "", "Local\\"} {
		for _, format := range []int{24, 32, 100} {
			t.Run(fmt.Sprintf("%q/f=%d", alias, format), func(t *testing.T) {
				term, replies := kittyCacheTerminal(t)
				data := []byte{1, 2, 3}
				if format == 32 {
					data = append(data, 255)
				}
				if format == 100 {
					img := image.NewRGBA(image.Rect(0, 0, 1, 1))
					img.SetRGBA(0, 0, color.RGBA{1, 2, 3, 255})
					var buf bytes.Buffer
					if err := png.Encode(&buf, img); err != nil {
						t.Fatal(err)
					}
					data = buf.Bytes()
				}
				m := winshm.New(t, data, 0, 0)
				term.Write(kittyCommand(fmt.Sprintf("a=T,t=s,f=%d,s=1,v=1,i=71,c=1,r=1,C=1", format), []byte(alias+m.Name)))
				g := term.Graphics(10, 20)
				if !strings.Contains(replies.String(), ";OK") || len(g) != 1 || !bytes.Equal(g[0].Image.Pix, []byte{1, 2, 3, 255}) {
					t.Fatal("shared pixels", replies.String(), g)
				}
				// Receiver must not close the sender's handle, nor keep its own
				// handle/view after copying. Cached pixels own their memory.
				if !winshm.Exists(m.Name) {
					t.Fatal("sender mapping destroyed")
				}
				m.Close()
				if winshm.Exists(m.Name) {
					t.Fatal("receiver leaked section handle/view")
				}
				if !bytes.Equal(term.Graphics(10, 20)[0].Image.Pix, []byte{1, 2, 3, 255}) {
					t.Fatal("pixels borrowed producer memory")
				}
				term.Write(kittyCommand("a=d,d=I,i=71", nil))
				if len(term.Graphics(10, 20)) != 0 {
					t.Fatal("image not deleted")
				}
			})
		}
	}
}

func TestI22WindowsSharedMemoryBoundsAndTypes(t *testing.T) {
	term, replies := kittyCacheTerminal(t)
	// Each failure must preserve an already displayed image.
	term.Write(kittyCommand("a=T,f=24,s=1,v=1,i=71,c=1,r=1,C=1", []byte{9, 8, 7}))
	for _, tc := range []struct {
		control    string
		size       uint32
		protection uint32
	}{
		{"O=-1", 3, 0}, {"S=-1", 3, 0}, {"O=3", 3, 0}, {"O=9223372036854775807", 3, 0},
		{"s=10000,v=10000", 3, 0}, {"S=2", 3, 0}, {"O=1", 3, 0}, {"o=X", 3, 0},
		{"", 400*1024*1024 + 4096, 0}, {"", 4096, windows.PAGE_READWRITE | 0x04000000},
	} {
		t.Run(tc.control+fmt.Sprint(tc.size, tc.protection), func(t *testing.T) {
			var data = []byte{1, 2, 3}
			if tc.protection != 0 {
				data = nil
			}
			m := winshm.New(t, data, tc.size, tc.protection)
			replies.Reset()
			term.Write(kittyCommand("a=T,t=s,f=24,s=1,v=1,i=71,"+tc.control, []byte("/"+m.Name)))
			g := term.Graphics(10, 20)
			if replies.Len() == 0 || strings.Contains(replies.String(), ";OK") || len(g) != 1 || !bytes.Equal(g[0].Image.Pix, []byte{9, 8, 7, 255}) {
				t.Fatal("bad range replaced display", replies.String(), g)
			}
			m.Close()
			if winshm.Exists(m.Name) {
				t.Fatal("error leaked mapping")
			}
		})
	}
	for _, name := range []string{"/missing-oshell-fixture", "Global\\forbidden", "/a/b", "../x", "C:\\tmp\\image", "//server/share", "/", "Local\\", "/bad\x00name"} {
		replies.Reset()
		term.Write(kittyCommand("a=T,t=s,f=24,s=1,v=1,i=71", []byte(name)))
		if replies.Len() == 0 || strings.Contains(replies.String(), ";OK") {
			t.Fatalf("invalid name %q accepted: %s", name, replies.String())
		}
	}
	file, err := os.Create(filepath.Join(t.TempDir(), "backing"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err = file.Truncate(4096); err != nil {
		t.Fatal(err)
	}
	m := winshm.NewFile(t, windows.Handle(file.Fd()), []byte{1, 2, 3}, 4096, 0)
	replies.Reset()
	term.Write(kittyCommand("a=T,t=s,f=24,s=1,v=1,i=71", []byte(m.Name)))
	if strings.Contains(replies.String(), ";OK") || replies.Len() == 0 {
		t.Fatal("file-backed section accepted", replies.String())
	}
	m.Close()
	if winshm.Exists(m.Name) {
		t.Fatal("rejected file mapping leaked")
	}
}

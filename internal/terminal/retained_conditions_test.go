package terminal

import (
	"bytes"
	"encoding/base64"
	"github.com/egoist/mygo/ui"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRetainedA10CloseReleasesNativeImagesAndLog(t *testing.T) {
	loadLib(t)
	term, err := New(Options{Conn: newPipe()})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill() }, 400, 180)
	var encoded bytes.Buffer
	png.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 2, 2)))
	term.Feed([]byte("\x1b]1337;File=inline=1;width=4;height=2:" + base64.StdEncoding.EncodeToString(encoded.Bytes()) + "\a"))
	tt.Frame()
	if len(term.v.graphics) == 0 {
		t.Fatal("image fixture did not allocate")
	}
	file := filepath.Join(t.TempDir(), "close.log")
	if err = term.StartLog(file); err != nil {
		t.Fatal(err)
	}
	term.Feed([]byte("before-close"))
	if err = term.Close(); err != nil {
		t.Fatal(err)
	}
	term.WaitLog()
	term.mu.Lock()
	released := term.closed && term.term == nil && term.rs == nil && term.v.graphics == nil && term.v.graphicBitmaps == nil
	if term.timer != nil && term.timer.Stop() {
		released = false
	}
	term.mu.Unlock()
	if !released {
		t.Fatal("terminal retained native state, images or active timer")
	}
	data, err := os.ReadFile(file)
	if err != nil || string(data) != "before-close" {
		t.Fatal("close did not drain log", err)
	}
}

func TestRetainedI17InvisiblePresentationCopyAndLog(t *testing.T) {
	loadLib(t)
	term, err := New(Options{Conn: newPipe(), NoBlink: true})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	file := filepath.Join(t.TempDir(), "log.txt")
	if err = term.StartLog(file); err != nil {
		t.Fatal(err)
	}
	term.Feed([]byte("\x1b[?25lA\x1b[8mSECRET\x1b[0mZ"))
	term.StopLog()
	term.WaitLog()
	tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill().AutoFocus() }, 400, 180)
	tt.Frame()
	hidden := bytes.Clone(tt.Image().Pix)
	term.v.selectAll()
	tt.Command("copy")
	if !strings.Contains(tt.Clipboard(), "ASECRETZ") {
		t.Fatal("invisible text lost on copy")
	}
	raw, err := os.ReadFile(file)
	if err != nil || !strings.Contains(string(raw), "ASECRETZ") {
		t.Fatal("log lost hidden text", string(raw), err)
	}
	save(t, tt, "retained-invisible")
	term.Feed([]byte("\x1bc\x1b[?25lA      Z"))
	tt.Frame()
	if !bytes.Equal(hidden, tt.Image().Pix) {
		t.Fatal("hidden text does not render like blank cells")
	}
}

func TestRetainedI24SixModesMixedWideAndWrap(t *testing.T) {
	modes := []string{"\x1b[?2501l\x1b[1 k", "\x1b[?2501l\x1b[2 k", "\x1b[1 k", "\x1b[2 k", "\x1b[8l\x1b[1 k", "\x1b[8l\x1b[2 k"}
	for i, mode := range modes {
		term, tt, conn := bidiViewFixture(t)
		text := strings.Repeat("abc אב 界 مرحبا ", 6)
		term.Feed([]byte(mode + text))
		tt.Frame()
		if strings.ReplaceAll(term.Text(), "\n", "") != strings.TrimRight(text, " ") {
			t.Fatal("logical text changed")
		}
		term.v.selectAll()
		tt.Command("copy")
		if !strings.Contains(strings.ReplaceAll(tt.Clipboard(), "\n", ""), "abc אב 界 مرحبا") {
			t.Fatal("copy lost mixed content")
		}
		tt.Key(0, ui.KeyLeft)
		if got := conn.take(3); got != "\x1b[D" {
			t.Fatalf("mode %d key: %q", i, got)
		}
		save(t, tt, "retained-bidi-mode-"+string(rune('1'+i)))
	}
}

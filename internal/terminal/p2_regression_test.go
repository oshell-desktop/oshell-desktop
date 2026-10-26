package terminal

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/egoist/mygo/ui"
	imagecolor "image/color"
	"io"
	"os"
	"oshell-desktop/internal/terminal/internal/vt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestP2I22LocalMediaActuallyDraws(t *testing.T) {
	loadLib(t)
	for _, medium := range []string{"d", "f", "t"} {
		t.Run(medium, func(t *testing.T) {
			term, err := New(Options{Conn: newPipe(), NoBlink: true})
			if err != nil {
				t.Fatal(err)
			}
			defer term.Close()
			view := ui.NewTester(func(c *ui.Context) { View(c, term).Fill() }, 400, 220)
			term.Feed([]byte("\x1b[?25l"))
			data := []byte{255, 0, 0}
			path := ""
			if medium != "d" {
				path = filepath.Join(t.TempDir(), "tty-graphics-protocol-p2")
				if err = os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
				data = []byte(path)
			}
			feedGraphic(term, "a=T,t="+medium+",f=24,s=1,v=1,i=71,c=2,r=2,C=1", data)
			view.Frame()
			graphicColor(t, term, view, 0, 0, imagecolor.RGBA{255, 0, 0, 255})
			graphicColor(t, term, view, 1, 1, imagecolor.RGBA{255, 0, 0, 255})
			if medium == "t" {
				if _, err = os.Stat(path); !os.IsNotExist(err) {
					t.Fatal("temporary resource retained", err)
				}
			}
		})
	}
}

func TestP2I10PromptByteLimit(t *testing.T) {
	e := newPromptEditor(io.Discard)
	defer e.Close()
	e.insert(strings.Repeat("中", 2730))
	e.insert("ab")
	if len(e.text()) != 8192 {
		t.Fatal(len(e.text()))
	}
	e.insert("中")
	if len(e.text()) != 8192 {
		t.Fatal("input overflow")
	}
}

type p2LogWriter struct {
	bytes.Buffer
	entered, release chan struct{}
	fail             bool
}

func (w *p2LogWriter) Write(b []byte) (int, error) {
	if w.entered != nil {
		close(w.entered)
		w.entered = nil
		<-w.release
	}
	if w.fail {
		return 0, io.ErrClosedPipe
	}
	return w.Buffer.Write(b)
}
func (w *p2LogWriter) Close() error { return nil }
func TestP2I06FilteredAsyncBoundedLog(t *testing.T) {
	w := &p2LogWriter{entered: make(chan struct{}), release: make(chan struct{})}
	entered := w.entered
	l := newOutputLog(w, nil)
	l.append([]byte("开始\x1b[31"))
	<-entered
	l.append([]byte("m红\x1b[0m\x1b]title"))
	l.append([]byte("\x1b\\\r\n\t结束\x07"))
	l.append(bytes.Repeat([]byte("x"), outputLogLimit))
	active, err := l.state()
	if active || err == nil {
		t.Fatal("overflow not visible")
	}
	close(w.release)
	<-l.done
	if w.String() != "开始红\r\n\t结束" {
		t.Fatalf("stream filter: %q", w.String())
	}
	failed := newOutputLog(&p2LogWriter{fail: true}, nil)
	failed.append([]byte("data"))
	<-failed.done
	if active, err := failed.state(); active || !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(active, err)
	}
}
func TestP2I20ProgressAndBell(t *testing.T) {
	loadLib(t)
	bells := 0
	term, err := New(Options{Conn: newPipe(), NoBlink: true, OnBell: func() { bells++ }})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	term.Feed([]byte("\a"))
	if bells != 1 {
		t.Fatal("BEL callback", bells)
	}
	for _, state := range []int{1, 2, 3, 4, 0} {
		term.Feed([]byte(fmt.Sprintf("\x1b]9;4;%d;150\a", state)))
		s, v := term.ProgressState(time.Now())
		if s != state || state != 0 && state != 3 && v != 100 {
			t.Fatal(state, s, v)
		}
	}
	term.Feed([]byte("\x1b]9;4;1;60\a"))
	if s, _ := term.ProgressState(time.Now().Add(16 * time.Second)); s != 0 {
		t.Fatal("TTL")
	}
	term.Feed([]byte("\x1b]9;4;2;10\a"))
	term.clearProgress()
	if s, _ := term.ProgressState(time.Now()); s != 0 {
		t.Fatal("session end")
	}
}
func TestP2I05CellBudgetNotUTF8Bytes(t *testing.T) {
	loadLib(t)
	term, err := New(Options{Conn: newPipe(), NoBlink: true})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	if err = term.SetHistoryLimit(10000); err != nil {
		t.Fatal(err)
	}
	term.mu.Lock()
	term.resize(gridSize{cols: 80, rows: 5, cellW: 8, cellH: 16})
	term.mu.Unlock()
	term.Feed([]byte(strings.Repeat("中", 23000) + "TARGET")) // 69 KiB, 46,006 cells.
	hits, err := term.Search("TARGET", true, true)
	if err != nil || len(hits) != 1 {
		t.Fatal("UTF-8 bytes incorrectly consumed cell budget", len(hits), err)
	}
	term.Feed([]byte(strings.Repeat("中", 10000) + "TOOLONG"))
	hits, err = term.Search("TOOLONG", true, true)
	if err == nil || len(hits) != 0 {
		t.Fatal("over 65,536 cells matched", len(hits), err)
	}
}
func TestP2O07HighlightPriorityUTF16AndRenderCap(t *testing.T) {
	loadLib(t)
	term, err := New(Options{Conn: newPipe(), NoBlink: true})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	first := HighlightRule{Pattern: "a", Foreground: ui.Hex("#ff0000")}
	second := HighlightRule{Pattern: "a", Foreground: ui.Hex("#00ff00")}
	if err = term.SetHighlights([]HighlightRule{first, second}); err != nil {
		t.Fatal(err)
	}
	v := view{rules: term.highlights}
	cells := []vt.Cell{{Codepoint: 'a'}}
	if v.rowHighlights(nil, cells)[0].Foreground != first.Foreground {
		t.Fatal("last rule overwrote first")
	}
	cells = make([]vt.Cell, 2049)
	for i := range cells {
		cells[i].Codepoint = '🙂'
	}
	if got := v.rowHighlights(nil, cells); got != nil {
		t.Fatal("UTF-16 line cap bypassed")
	}
	rules := make([]HighlightRule, 33)
	for i := range rules {
		rules[i] = first
	}
	if err = term.SetHighlights(rules); err != nil || len(term.highlights) != 32 {
		t.Fatal("render cap", len(term.highlights), err)
	}
}

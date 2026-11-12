package terminal

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestLiveOutputDuringMouseSelection(t *testing.T) {
	loadLib(t)
	for _, end := range []string{"release", "cancel", "hide", "remote mouse"} {
		t.Run(end, func(t *testing.T) {
			conn, remote := net.Pipe()
			defer remote.Close()
			output := make(chan struct{}, 2)
			term, err := New(Options{Conn: conn, CopyOnSelect: true, OnActivity: func() { output <- struct{}{} }})
			if err != nil {
				t.Fatal(err)
			}
			defer term.Close()
			tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill().AutoFocus() }, 320, 200)
			term.Feed([]byte("abcdefghij"))
			<-output
			if end == "remote mouse" {
				term.Feed([]byte("\x1b[?1002h\x1b[?1006h"))
			}
			tt.Frame()
			x, y := cellCenter(term, 1, 0)
			beginCharacterDrag(tt, x, y)
			x, y = cellCenter(term, 4, 0)
			tt.Move(x, y)
			if _, err := remote.Write([]byte("\rXXXXXXXXXX")); err != nil {
				t.Fatal(err)
			}
			if end != "remote mouse" {
				select {
				case <-output:
					t.Fatal("display changed during local selection")
				case <-time.After(50 * time.Millisecond):
				}
			}
			switch end {
			case "release":
				tt.Release(x, y)
				if s := tt.Clipboard(); s != "bcd" {
					t.Fatalf("copied %q", s)
				}
			case "cancel":
				tt.CancelPointer()
			case "hide":
				term.SetVisible(false)
			}
			select {
			case <-output:
			case <-time.After(2 * time.Second):
				t.Fatal("live output remained suspended")
			}
		})
	}
}

func TestSelectionStartsAtFirstDragEvent(t *testing.T) {
	for _, tc := range []struct {
		name               string
		press, first, next int
		want               string
	}{
		{"forward", 0, 3, 6, "def"},
		{"reverse", 9, 7, 4, "efg"},
	} {
		for _, bidi := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/bidi=%v", tc.name, bidi), func(t *testing.T) {
				term, tt := wordClickFixture(t, "abcdefghij", 320)
				term.mu.Lock()
				term.term.SetBidiEnabled(bidi)
				term.mu.Unlock()
				tt.Frame()
				x, y := cellCenter(term, tc.press, 0)
				tt.Press(x, y)
				x, y = cellCenter(term, tc.first, 0)
				tt.Move(x, y)
				if got := term.SearchSelection(); got != "" {
					t.Fatalf("first drag copied %q", got)
				}
				term.mu.Lock()
				active := term.term.SelectionActive()
				term.mu.Unlock()
				if !active {
					t.Fatal("first drag did not retain active empty range")
				}
				x, y = cellCenter(term, tc.next, 0)
				tt.Move(x, y)
				tt.Release(x, y)
				if got := tt.Clipboard(); got != tc.want {
					t.Fatalf("continued drag copied %q, want %q", got, tc.want)
				}
			})
		}
	}
}

func TestSelectionDragRestartsAfterSelectionCancelled(t *testing.T) {
	term, tt := wordClickFixture(t, "abcdefghij", 320)
	x, y := cellCenter(term, 0, 0)
	tt.Press(x, y)
	x, y = cellCenter(term, 1, 0)
	tt.Move(x, y)
	x, y = cellCenter(term, 4, 0)
	tt.Move(x, y)
	if got := term.SearchSelection(); got != "bcd" {
		t.Fatal("initial selection", got)
	}
	term.Feed([]byte("\x1b[1;3r\x1b[S\x1b[Habcdefghij"))
	tt.Frame()
	x, y = cellCenter(term, 5, 0)
	tt.Move(x, y)
	if got := term.SearchSelection(); got != "" {
		t.Fatal("restart selected before extension", got)
	}
	x, y = cellCenter(term, 8, 0)
	tt.Move(x, y)
	tt.Release(x, y)
	if got := tt.Clipboard(); got != "fgh" {
		t.Fatal("selection did not restart at next drag", got)
	}
}

// Model a first drag motion within the initial cell before testing an extended
// range. OShell seeds on mouseDragged, not on mouseDown.
func beginCharacterDrag(tt *ui.Tester, x, y float32) {
	tt.Press(x, y)
	tt.Move(x+0.01, y)
}

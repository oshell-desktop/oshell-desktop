package terminal

import (
	"github.com/egoist/mygo/ui"
	"strings"
	"testing"
	"time"
)

func TestSelectionHostDoubleClickIntervalAndDistance(t *testing.T) {
	term, _ := timedWordFixture(t)
	v := term.v
	x, y := cellCenter(term, 8, 0)
	press := func(ms int, dx, dy float32, interval time.Duration) string {
		term.mu.Lock()
		defer term.mu.Unlock()
		ref, _ := term.term.CellAt(8, 0)
		ev := ui.InputEvent{X: x + dx, Y: y + dy, HasDoubleClickSettings: true, DoubleClickInterval: interval, DoubleClickDistanceX: 2, DoubleClickDistanceY: 4}
		s, ok := v.wordPress(ref, ev, float64(ev.X*v.scale), float64(ev.Y*v.scale), uint64(ms)*uint64(time.Millisecond))
		if ok {
			term.term.SetSelection(&s)
		} else {
			term.term.SetSelection(nil)
		}
		v.gesture.Release(term.term, &ref)
		text, _ := term.term.SelectionText()
		return text
	}
	press(1000, 0, 0, 900*time.Millisecond)
	if got := press(1800, 1, 3, 900*time.Millisecond); got != "file" {
		t.Fatal("host's long interval rejected", got)
	}
	term.v.wordClicks = wordClickState{}
	press(2000, 0, 0, 0)
	if got := press(2001, 0, 0, 0); got != "" {
		t.Fatal("valid zero interval used fallback", got)
	}
	// Independent axis deltas: a rectangle, not a circle.
	term.v.wordClicks = wordClickState{}
	press(3000, 0, 0, 200*time.Millisecond)
	if got := press(3200, 2, 4, 200*time.Millisecond); got != "file" {
		t.Fatal("axis limits rejected", got)
	}
	term.v.wordClicks = wordClickState{}
	press(4000, 0, 0, 200*time.Millisecond)
	if got := press(4201, 0, 0, 200*time.Millisecond); got != "" {
		t.Fatal("short host interval ignored", got)
	}
	term.v.wordClicks = wordClickState{}
	press(5000, 0, 0, 900*time.Millisecond)
	if got := press(5100, 2.1, 0, 900*time.Millisecond); got != "" {
		t.Fatal("narrow host rectangle ignored", got)
	}
}

func TestWordSeedUsesHostContinuationWindow(t *testing.T) {
	for _, interval := range []time.Duration{100 * time.Millisecond, 900 * time.Millisecond} {
		term, _ := timedWordFixture(t)
		timedWordClick(t, term, 8, 0, 0, time.Second)
		timedWordClick(t, term, 8, 0, 0, 1100*time.Millisecond)
		s := term.v.wordClicks.seed
		term.v.wordClicks.interval = interval
		deadline := s.time + uint64(max(time.Second, 3*interval))
		if !term.v.continuesWordClick(s.x, s.y, deadline, s.buffer) || term.v.continuesWordClick(s.x, s.y, deadline+1, s.buffer) {
			t.Fatal("wrong host seed deadline", interval)
		}
	}
}

func TestPointerCancelStopsSelectionAutoscroll(t *testing.T) {
	term, tt, _ := bidiViewFixture(t)
	term.Feed([]byte(strings.Repeat("abcdefghij\r\n", 120)))
	tt.Frame()
	term.mu.Lock()
	term.term.ScrollToRow(20)
	term.mu.Unlock()
	tt.Frame()
	v := term.v
	x, y := cellCenter(term, 2, 2)
	term.opts.CopyOnSelect = true
	v.services.WriteClipboard("preserved clipboard")
	tt.Press(x, y)
	y = float32(v.oy+(v.rows+3)*v.cellH) / v.scale
	tt.Move(x, y)
	if !v.ticking {
		t.Fatal("fixture not scrolling")
	}
	term.mu.Lock()
	before := term.term.Scrollbar().Offset
	term.mu.Unlock()
	tt.CancelPointer()
	if got := tt.Clipboard(); got != "preserved clipboard" {
		t.Fatal("cancel copied selection", got)
	}
	term.mu.Lock()
	v.autoscrollAt(time.Now().Add(time.Hour))
	after := term.term.Scrollbar().Offset
	term.mu.Unlock()
	if v.selecting || v.ticking || v.linkPressed || before != after {
		t.Fatalf("cancelled drag continued: %+v %d %d", v.selecting, before, after)
	}
}

func TestPointerCancelReleasesRemoteMouse(t *testing.T) {
	loadLib(t)
	conn := newPipe()
	term, err := New(Options{Conn: conn})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill().AutoFocus() }, 400, 200)
	term.Feed([]byte("\x1b[?1002h\x1b[?1006h"))
	tt.Frame()
	x, y := cellCenter(term, 2, 3)
	tt.Press(x, y)
	if got := conn.take(9); got != "\x1b[<0;3;4M" {
		t.Fatal(got)
	}
	tt.CancelPointer()
	if got := conn.take(9); got != "\x1b[<0;3;4m" {
		t.Fatal("remote button remained pressed", got)
	}
	if term.v.reporting {
		t.Fatal("cancel retained mouse reporting drag")
	}
}

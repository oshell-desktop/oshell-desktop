package terminal

import (
	"github.com/egoist/mygo/ui"
	"strings"
	"testing"
	"time"
)

func TestOverlayScrollbarTiming(t *testing.T) {
	start := time.Now()
	s := overlayScrollbar{activity: start}
	for _, test := range []struct {
		delay time.Duration
		alpha float32
	}{{0, .45}, {800 * time.Millisecond, .45}, {900 * time.Millisecond, .225}, {time.Second, 0}, {2 * time.Second, 0}} {
		if got := s.alpha(start.Add(test.delay)); got != test.alpha {
			t.Fatalf("%v: alpha %f, want %f", test.delay, got, test.alpha)
		}
	}
	s.dragging = true
	if s.alpha(start.Add(time.Hour)) != .45 {
		t.Fatal("drag faded")
	}
	s.hide()
	if s.dragging || s.alpha(start) != 0 {
		t.Fatal("hide retained drag or visibility")
	}
}
func TestOverlayScrollbarNativeVTInteractionAndLifecycle(t *testing.T) {
	loadLib(t)
	term, err := New(Options{Conn: newPipe(), NoBlink: true})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill().AutoFocus() }, 640, 300)
	term.Feed([]byte(strings.Repeat("history\r\n", 240)))
	tt.Frame()
	v := term.v
	cols, rows := v.cols, v.rows
	if !v.overlay.activity.IsZero() {
		t.Fatal("output revealed scrollbar")
	}
	tt.Key(0, ui.KeyPageUp)
	if v.overlay.alpha(time.Now()) == 0 {
		t.Fatal("page scrolling did not show scrollbar")
	}
	activity := v.overlay.activity
	term.Feed([]byte("background output\r\n"))
	tt.Frame()
	if !v.overlay.activity.Equal(activity) {
		t.Fatal("output postponed fade")
	}
	if cols != v.cols || rows != v.rows {
		t.Fatal("overlay changed geometry")
	}
	y, h := scrollbarKnob(v.scrollbar, v.overlay.height)
	x := v.overlay.width - 4
	if !v.pointerEvent(ui.InputEvent{Kind: ui.InputPointerDown, Button: 0, X: x, Y: y + h/2}) || !v.overlay.dragging {
		t.Fatal("knob not draggable")
	}
	v.pointerEvent(ui.InputEvent{Kind: ui.InputPointerMove, X: x, Y: v.overlay.height})
	term.mu.Lock()
	bottom := term.term.AtBottom()
	term.mu.Unlock()
	if !bottom {
		t.Fatal("drag failed to reach bottom")
	}
	v.pointerEvent(ui.InputEvent{Kind: ui.InputPointerUp, Button: 0, X: x, Y: v.overlay.height})
	if v.overlay.dragging || v.overlay.alpha(time.Now()) == 0 {
		t.Fatal("release did not arm fade")
	}
	term.SetVisible(false)
	term.SetVisible(true)
	tt.Frame()
	if v.overlay.alpha(time.Now()) != 0 {
		t.Fatal("hidden terminal retained scrollbar")
	}
	tt.Key(0, ui.KeyPageUp)
	term.Feed([]byte("\x1b[?1049h"))
	tt.Frame()
	if v.overlay.alpha(time.Now()) != 0 {
		t.Fatal("alternate screen retained scrollbar")
	}
	term.Feed([]byte("\x1b[?1049l"))
	tt.Frame()
	if v.overlay.alpha(time.Now()) != 0 {
		t.Fatal("alternate exit spuriously revealed scrollbar")
	}
	if v.cols != cols || v.rows != rows {
		t.Fatal("visibility changed terminal size")
	}
}

func TestScrollbarFadeWakeStopsAfterDeadlineAndHide(t *testing.T) {
	term := &Terminal{done: make(chan struct{})}
	wake := make(chan struct{}, 64)
	draw := func() { wake <- struct{}{} }
	term.draw.Store(&draw)
	term.armScrollbarFade()
	select {
	case <-wake:
	case <-time.After(2 * time.Second):
		t.Fatal("idle window never invalidated")
	}
	time.Sleep(300 * time.Millisecond)
	term.scrollTimerMu.Lock()
	active := term.scrollTimer != nil
	term.scrollTimerMu.Unlock()
	if active {
		t.Fatal("fade left a persistent timer")
	}
	term.armScrollbarFade()
	term.SetVisible(false)
	term.scrollTimerMu.Lock()
	active = term.scrollTimer != nil
	term.scrollTimerMu.Unlock()
	if active {
		t.Fatal("hidden terminal retained timer")
	}
}

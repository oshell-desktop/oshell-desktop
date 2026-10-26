package terminal

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func sourceTimedPress(t *testing.T, term *Terminal, at time.Duration, present bool) string {
	t.Helper()
	x, y := cellCenter(term, 8, 0)
	ev := ui.InputEvent{Kind: ui.InputPointerDown, X: x, Y: y, HasTimestamp: present, Timestamp: at}
	if !term.v.pointerEvent(ev) {
		t.Fatal("press not handled")
	}
	ev.Kind = ui.InputPointerUp
	term.v.pointerEvent(ev)
	term.mu.Lock()
	defer term.mu.Unlock()
	s, _ := term.term.SelectionText()
	return s
}

func TestWordClicksUseSourceTimeDespiteDispatchDelay(t *testing.T) {
	term, _ := timedWordFixture(t)
	sourceTimedPress(t, term, 0, true)
	term.v.gestureStart = time.Now().Add(-time.Hour)
	if got := sourceTimedPress(t, term, 100*time.Millisecond, true); got != "file" {
		t.Fatal("queue delay split word click", got)
	}
	if got := sourceTimedPress(t, term, 2*time.Second, true); got != "" {
		t.Fatal("queued slow clicks became double click", got)
	}
	if got := sourceTimedPress(t, term, 2100*time.Millisecond, true); got != "file" {
		t.Fatal("expired word seed expanded on dispatch time", got)
	}
	if got := sourceTimedPress(t, term, time.Second, true); got != "" {
		t.Fatal("backward event continued word click", got)
	}
}

func TestWordSeedUsesSourceTimeAndSeparatesClockOrigins(t *testing.T) {
	term, _ := timedWordFixture(t)
	sourceTimedPress(t, term, time.Second, true)
	sourceTimedPress(t, term, 1100*time.Millisecond, true)
	term.v.gestureStart = time.Now().Add(-2 * time.Hour)
	sourceTimedPress(t, term, 2*time.Second, true)
	if got := sourceTimedPress(t, term, 2100*time.Millisecond, true); got != "/home/file.txt" {
		t.Fatal("dispatch delay expired word seed", got)
	}
	if got := sourceTimedPress(t, term, 0, false); got != "" {
		t.Fatal("source epoch leaked into fallback", got)
	}
	if got := sourceTimedPress(t, term, 0, true); got != "" {
		t.Fatal("fallback epoch leaked into source", got)
	}
	if got := sourceTimedPress(t, term, 100*time.Millisecond, true); got != "file" {
		t.Fatal("fresh zero-origin series failed", got)
	}
}

package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/internal/platform"
)

func TestPressSourceTimestampReachesHandler(t *testing.T) {
	var got InputEvent
	tt := NewTester(func(c *Context) {
		Box(c).Fill().HandleInput(func(ev InputEvent) bool {
			if ev.Kind == InputPointerDown {
				got = ev
			}
			return true
		})
	}, 200, 100)
	defer tt.rt.close()
	for _, at := range []time.Duration{0, 300 * time.Millisecond, 50 * 24 * time.Hour} {
		tt.send(platform.SurfaceEvent{Kind: platform.PointerDown, X: 30, Y: 40, HasTimestamp: true, Timestamp: at})
		if !got.HasTimestamp || got.Timestamp != at {
			t.Fatal("source timestamp lost", got)
		}
		tt.Release(30, 40)
	}
}

func TestButtonClickTimingUsesSourceEvents(t *testing.T) {
	doubles := 0
	tt := NewTester(func(c *Context) {
		if Button(c, "action").Fill().DoubleClicked() {
			doubles++
		}
	}, 200, 100)
	defer tt.rt.close()
	press := func(at time.Duration, present bool) {
		tt.send(platform.SurfaceEvent{Kind: platform.PointerDown, X: 30, Y: 40, HasTimestamp: present, Timestamp: at})
		tt.Release(30, 40)
	}
	press(0, true)
	// Processing took longer than the click interval, but the events did not.
	tt.rt.lastPress.at = time.Now().Add(-time.Hour)
	press(100*time.Millisecond, true)
	if doubles != 1 {
		t.Fatal("queue delay split double click", doubles)
	}
	// Dispatching stale events together cannot create a double click.
	press(2*time.Second, true)
	if tt.rt.lastPress.clicks != 1 {
		t.Fatal("compressed queue joined separate clicks")
	}
	press(time.Second, true)
	if tt.rt.lastPress.clicks != 1 {
		t.Fatal("backward timestamp joined clicks")
	}
	press(0, false)
	if tt.rt.lastPress.clicks != 1 {
		t.Fatal("source and fallback epochs compared")
	}
	press(0, true)
	if tt.rt.lastPress.clicks != 1 {
		t.Fatal("fallback and source epochs compared")
	}
	press(100*time.Millisecond, true)
	if doubles != 2 {
		t.Fatal("new source series failed", doubles)
	}
}

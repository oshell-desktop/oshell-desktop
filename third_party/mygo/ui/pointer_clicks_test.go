package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/internal/platform"
)

func TestPointerClickSeriesSurvivesHandlerRebuild(t *testing.T) {
	var down, up []int
	generation := 0
	tt := NewTester(func(c *Context) {
		Box(c.Key(generation)).Fill().HandleInput(func(ev InputEvent) bool {
			switch ev.Kind {
			case InputPointerDown:
				down = append(down, ev.Clicks)
			case InputPointerUp:
				up = append(up, ev.Clicks)
			}
			return true
		})
	}, 200, 100)
	defer tt.rt.close()
	for i := 1; i <= 8; i++ {
		tt.send(platform.SurfaceEvent{Kind: platform.PointerDown, X: 30, Y: 40, HasTimestamp: true, Timestamp: time.Duration(i) * 100 * time.Millisecond})
		tt.Release(30, 40)
		generation++
		tt.Frame()
		if len(down) != i || len(up) != i || down[i-1] != i || up[i-1] != i {
			t.Fatalf("rebuilt handler lost series: down=%v up=%v", down, up)
		}
	}
}

func TestPointerClickSeriesSharedWithDecliningHandlers(t *testing.T) {
	var counts []int
	doubles := 0
	tt := NewTester(func(c *Context) {
		e := Button(c, "action").Fill().HandleInput(func(ev InputEvent) bool {
			if ev.Kind == InputPointerDown {
				counts = append(counts, ev.Clicks)
			}
			return false
		})
		if e.DoubleClicked() {
			doubles++
		}
	}, 200, 100)
	defer tt.rt.close()
	for i := 1; i <= 3; i++ {
		tt.ClickAt(30, 40)
		if len(counts) != i || counts[i-1] != i {
			t.Fatal(counts)
		}
	}
	if doubles != 1 {
		t.Fatal("handler and control counted separately", doubles)
	}
	// A native count is authoritative, even when the local estimate differs.
	tt.send(platform.SurfaceEvent{Kind: platform.PointerDown, X: 30, Y: 40, Clicks: 7})
	tt.Release(30, 40)
	if counts[len(counts)-1] != 7 {
		t.Fatal("native count replaced", counts)
	}
}

func TestPointerClickSeriesButtonDistanceAndCancellation(t *testing.T) {
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
	clock := time.Duration(0)
	press := func(x, y float64, button, want int) {
		clock += 100 * time.Millisecond
		tt.send(platform.SurfaceEvent{Kind: platform.PointerDown, X: x, Y: y, Button: button, HasTimestamp: true, Timestamp: clock, HasDoubleClickSettings: true, DoubleClickInterval: time.Second, DoubleClickDistanceX: 2, DoubleClickDistanceY: 4})
		tt.send(platform.SurfaceEvent{Kind: platform.PointerUp, X: x, Y: y, Button: button})
		if got.Clicks != want {
			t.Fatalf("count=%d want=%d at %v,%v button=%d", got.Clicks, want, x, y, button)
		}
	}
	press(30, 40, 0, 1)
	press(32, 44, 0, 2)
	press(35, 44, 0, 1)
	press(35, 44, 1, 1)
	press(35, 44, 0, 1)
	press(35, 44, 0, 2)
	tt.CancelPointer()
	press(35, 44, 0, 1)
	tt.send(platform.SurfaceEvent{Kind: platform.SurfaceBlur})
	press(35, 44, 0, 1)
}

func TestPointerClickSeriesDragAndMatchingRelease(t *testing.T) {
	var down, up []int
	tt := NewTester(func(c *Context) {
		Box(c).Fill().HandleInput(func(ev InputEvent) bool {
			if ev.Kind == InputPointerDown {
				down = append(down, ev.Clicks)
			}
			if ev.Kind == InputPointerUp {
				up = append(up, ev.Clicks)
			}
			return true
		})
	}, 200, 100)
	defer tt.rt.close()
	tt.ClickAt(30, 40)
	tt.Press(30, 40)
	tt.Move(90, 40)
	tt.Move(30, 40)
	tt.send(platform.SurfaceEvent{Kind: platform.PointerUp, X: 30, Y: 40, Button: 1})
	if len(up) != 1 || tt.rt.pressed == nil {
		t.Fatal("different button released captured primary")
	}
	tt.Release(30, 40)
	if len(up) != 2 || up[1] != 2 {
		t.Fatal("drag lost matching press count", up)
	}
	tt.ClickAt(30, 40)
	if down[len(down)-1] != 1 {
		t.Fatal("returning drag retained click series", down)
	}
}

package ui

import (
	"github.com/egoist/mygo/internal/platform"
	"testing"
	"time"
)

func TestPointerCaptureCancellationIsNotARelease(t *testing.T) {
	var events []InputEvent
	tt := NewTester(func(c *Context) {
		Box(c).Fill().HandleInput(func(ev InputEvent) bool { events = append(events, ev); return true })
	}, 200, 100)
	defer tt.rt.close()
	tt.Press(30, 40)
	events = nil
	tt.send(platform.SurfaceEvent{Kind: platform.PointerCancel})
	tt.send(platform.SurfaceEvent{Kind: platform.PointerCancel})
	tt.Release(30, 40)
	n := 0
	for _, ev := range events {
		if ev.Kind == InputPointerUp {
			t.Fatal("cancelled pointer generated release")
		}
		if ev.Kind == InputPointerCancel {
			n++
			if ev.X != 30 || ev.Y != 40 {
				t.Fatal(ev)
			}
		}
	}
	if n != 1 || tt.rt.pressed != nil {
		t.Fatalf("cancel events=%d pressed=%v", n, tt.rt.pressed)
	}
	tt.Press(30, 40)
	tt.Release(30, 40)
	if events[len(events)-1].Kind != InputPointerUp {
		t.Fatal("new pointer did not release", events)
	}
}

func TestPointerCancelDoesNotClickButton(t *testing.T) {
	clicked := 0
	tt := NewTester(func(c *Context) {
		if Button(c, "action").Fill().Clicked() {
			clicked++
		}
	}, 200, 100)
	defer tt.rt.close()
	tt.Press(30, 40)
	tt.send(platform.SurfaceEvent{Kind: platform.PointerCancel})
	tt.Release(30, 40)
	if clicked != 0 {
		t.Fatal("lost capture clicked the button")
	}
	tt.ClickAt(30, 40)
	if clicked != 1 {
		t.Fatal("new click did not work", clicked)
	}
}

func TestNativeDoubleClickMetadataReachesHandler(t *testing.T) {
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
	tt.send(platform.SurfaceEvent{Kind: platform.PointerDown, X: 10, Y: 20, Clicks: 4, HasDoubleClickSettings: true, DoubleClickInterval: 850 * time.Millisecond, DoubleClickDistanceX: 2.5, DoubleClickDistanceY: 4.5})
	if !got.HasDoubleClickSettings || got.Clicks != 4 || got.DoubleClickInterval != 850*time.Millisecond || got.DoubleClickDistanceX != 2.5 || got.DoubleClickDistanceY != 4.5 {
		t.Fatal(got)
	}
}

func TestButtonsUseHostClickSettings(t *testing.T) {
	doubles := 0
	tt := NewTester(func(c *Context) {
		if Button(c, "action").Fill().DoubleClicked() {
			doubles++
		}
	}, 200, 100)
	defer tt.rt.close()
	press := platform.SurfaceEvent{Kind: platform.PointerDown, X: 30, Y: 40, DoubleClickInterval: 900 * time.Millisecond, DoubleClickDistanceX: 2, DoubleClickDistanceY: 4}
	tt.send(press)
	tt.Release(30, 40)
	tt.rt.lastPress.at = time.Now().Add(-800 * time.Millisecond)
	tt.send(press)
	tt.Release(30, 40)
	if doubles != 1 {
		t.Fatal("button ignored long host interval", doubles)
	}
	tt.rt.lastPress.at = time.Now().Add(-300 * time.Millisecond)
	press.DoubleClickInterval = 100 * time.Millisecond
	tt.send(press)
	tt.Release(30, 40)
	if tt.rt.lastPress.clicks != 1 {
		t.Fatal("short interval ignored")
	}
	tt.send(platform.SurfaceEvent{Kind: platform.PointerCancel})
	tt.send(press)
	tt.Release(30, 40)
	if tt.rt.lastPress.clicks != 1 {
		t.Fatal("cancel inherited old click series")
	}
}

func TestPointerCancelClearsPendingContextRelease(t *testing.T) {
	tt := NewTester(func(c *Context) { Button(c, "action").Fill() }, 200, 100)
	defer tt.rt.close()
	tt.Press(30, 40)
	tt.rt.menu.release, tt.rt.menu.releaseText = tt.rt.pressed.id, tt.rt.pressed.id
	tt.send(platform.SurfaceEvent{Kind: platform.PointerCancel})
	if tt.rt.menu.release != 0 || tt.rt.menu.releaseText != 0 {
		t.Fatal("cancel retained pending context menu")
	}
}

func TestWindowBlurCancelsPointerWithoutRelease(t *testing.T) {
	var events []InputEvent
	tt := NewTester(func(c *Context) {
		Box(c).Fill().HandleInput(func(ev InputEvent) bool { events = append(events, ev); return true })
	}, 200, 100)
	defer tt.rt.close()
	tt.Press(30, 40)
	events = nil
	tt.send(platform.SurfaceEvent{Kind: platform.SurfaceBlur})
	tt.send(platform.SurfaceEvent{Kind: platform.PointerCancel})
	n := 0
	for _, ev := range events {
		if ev.Kind == InputPointerUp {
			t.Fatal("blur synthesized release")
		}
		if ev.Kind == InputPointerCancel {
			n++
		}
	}
	if n != 1 {
		t.Fatal("blur cancellation count", n)
	}
}

//go:build windows && (amd64 || arm64)

package windows

import (
	"github.com/egoist/mygo/internal/platform"
	"testing"
	"time"
)

func TestSurfaceLostCaptureCancelsOnlyHeldPointers(t *testing.T) {
	h := &keypadRecorder{}
	s := &surface{w: &window{h: h}, buttons: 1}
	s.message(0, wmCaptureChanged, 0, 0)
	if s.buttons != 0 || len(h.events) != 1 || h.events[0].Kind != platform.PointerCancel {
		t.Fatalf("lost capture: %+v", h.events)
	}
	// ReleaseCapture also sends this message after a normal button-up; it must
	// not cancel that normal release or deliver duplicate cancellation.
	s.message(0, wmCaptureChanged, 0, 0)
	if len(h.events) != 1 {
		t.Fatal("normal/repeated release cancelled again", h.events)
	}
}

func TestSurfacePointerReadsHostDoubleClickSettings(t *testing.T) {
	h := &keypadRecorder{}
	s := &surface{w: &window{h: h}}
	s.send(s.pointer(platform.PointerDown, uintptr(20<<16|10), 0))
	if len(h.events) != 1 {
		t.Fatal(h.events)
	}
	ev := h.events[0]
	if !ev.HasTimestamp || ev.Timestamp < 0 {
		t.Fatalf("message timestamp missing: %+v", ev)
	}
	ms, _, _ := procDoubleClickTime.Call()
	width, _, _ := procDoubleClickMetrics.Call(36)
	height, _, _ := procDoubleClickMetrics.Call(37)
	if !ev.HasDoubleClickSettings || ev.DoubleClickInterval != time.Duration(ms)*time.Millisecond || ev.DoubleClickDistanceX != s.toDIP(int32(width))/2 || ev.DoubleClickDistanceY != s.toDIP(int32(height))/2 {
		t.Fatalf("system settings lost: %+v", ev)
	}
}

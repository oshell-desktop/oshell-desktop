//go:build windows && (amd64 || arm64)

package windows

import (
	"github.com/egoist/mygo/internal/platform"
	"runtime"
	"testing"
)

func TestFunctionKeyMessages(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for n := 1; n <= 24; n++ {
		want := platform.KeyF1 + platform.Key(n-1)
		if n > 12 {
			want = platform.KeyF13 + platform.Key(n-13)
		}
		h := &keypadRecorder{}
		s := &surface{w: &window{h: h}}
		vk := uintptr(0x6f + n)
		s.message(0, wmKeyDown, vk, 0)
		s.message(0, wmKeyDown, vk, 1<<30)
		s.message(0, wmKeyUp, vk, 1<<30)
		if len(h.events) != 3 {
			t.Fatalf("F%d: lost event %+v", n, h.events)
		}
		for i, ev := range h.events {
			kind := platform.KeyPressed
			if i == 2 {
				kind = platform.KeyReleased
			}
			if ev.Key != want || ev.Kind != kind || ev.Repeat != (i == 1) || ev.Keypad {
				t.Fatalf("F%d event %d: %+v", n, i, ev)
			}
		}
	}
	if vkKey(0x88) != platform.KeyUnknown {
		t.Fatal("reserved VK was interpreted as F25")
	}
}

package terminal

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

const fontWheelThreshold = 300 // DIPs; about 3–4 notches at Windows defaults.
const fontWheelIdle = time.Second

type fontWheelState struct {
	delta float32
	last  time.Time
}

func (s *fontWheelState) step(delta float32, now time.Time) float32 {
	if delta == 0 || math.IsNaN(float64(delta)) || math.IsInf(float64(delta), 0) {
		return 0
	}
	if now.Sub(s.last) > fontWheelIdle || delta*s.delta < 0 {
		s.delta = 0
	}
	s.last = now
	s.delta += delta
	if s.delta > -fontWheelThreshold && s.delta < fontWheelThreshold {
		return 0
	}
	step := float32(-1)
	if s.delta < 0 {
		step = 1
	}
	// A coalesced/high-resolution event must not jump many font sizes.
	s.delta = float32(math.Mod(float64(s.delta), fontWheelThreshold))
	return step
}

func (v *view) zoomFontWithWheel(ev ui.InputEvent) bool {
	if ev.Mods != ui.Ctrl {
		v.fontWheel = fontWheelState{}
		return false
	}
	v.scrolled = 0 // Zoom deltas must never spill into scrollback or remote mouse input.
	step := v.fontWheel.step(ev.DY, time.Now())
	if step != 0 {
		t := v.t
		t.mu.Lock()
		size := t.opts.Font.Size
		if size <= 0 {
			size = 13
		}
		t.opts.Font.Size = max(10, min(26, size+step))
		t.mu.Unlock()
		t.redraw()
	}
	return true
}

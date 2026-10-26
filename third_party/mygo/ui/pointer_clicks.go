package ui

import (
	"github.com/egoist/mygo/internal/platform"
	"time"
)

// Count at the window input boundary before hit testing or handlers. A handler
// may replace the rows under the pointer; that does not end the physical series.
// Native counts take precedence. Windows/GTK supply ordinary presses and their
// host timing/distance settings, so one series serves controls and custom views.
func (rt *engine) pointerClicks(x, y float32, button, count int, native platform.SurfaceEvent) int {
	now := time.Now()
	clicks := 1
	lp := &rt.lastPress
	interval, dx, dy := 500*time.Millisecond, float32(5), float32(5)
	if native.HasDoubleClickSettings || native.DoubleClickInterval > 0 {
		interval = max(0, native.DoubleClickInterval)
		dx, dy = max(0, float32(native.DoubleClickDistanceX)), max(0, float32(native.DoubleClickDistanceY))
	}
	elapsed := now.Sub(lp.at)
	if native.HasTimestamp && lp.hasTimestamp {
		elapsed = native.Timestamp - lp.timestamp
	}
	if native.HasTimestamp == lp.hasTimestamp && lp.clicks > 0 && lp.button == button && elapsed >= 0 && elapsed <= interval && abs32(x-lp.x) <= dx && abs32(y-lp.y) <= dy {
		clicks = lp.clicks
		if clicks < int(^uint(0)>>1) {
			clicks++
		}
	}
	if count > 0 {
		clicks = count
	} // AppKit supplies the actual click series.
	lp.at, lp.x, lp.y, lp.clicks, lp.button = now, x, y, clicks, button
	lp.timestamp, lp.hasTimestamp = native.Timestamp, native.HasTimestamp
	lp.distanceX, lp.distanceY = dx, dy
	return clicks
}

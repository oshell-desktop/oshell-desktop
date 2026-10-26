package terminal

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// pointerTimestamp uses when the press occurred, not when a queued press was
// dispatched. Synthetic events without a source clock keep the local fallback.
// Switching clocks starts a new series; their epochs must never be compared.
func (v *view) pointerTimestamp(ev ui.InputEvent) (ns uint64, changed bool) {
	native := ev.HasTimestamp && ev.Timestamp >= 0
	changed = v.gestureTimestampSet && native != v.gestureTimestampSource
	if changed {
		v.wordClicks = wordClickState{}
	}
	v.gestureTimestampSet, v.gestureTimestampSource = true, native
	if native {
		return uint64(ev.Timestamp), changed
	}
	return uint64(time.Since(v.gestureStart)), changed
}

package platform

import "time"

// MillisClock unfolds a surface's unsigned 32-bit event timestamps. Win32 and
// GDK timestamps wrap after about 49.7 days. Subtraction in the source width
// keeps the event interval across that boundary, independently of queue delay.
// A backward event is a large gap and therefore expires any click series.
// Like the source clock, intervals of an entire wrap cannot be distinguished.
type MillisClock struct {
	last  uint32
	value time.Duration
	seen  bool
}

func (c *MillisClock) Expand(ms uint32) time.Duration {
	if !c.seen {
		c.value = time.Duration(ms) * time.Millisecond
		c.seen = true
	} else {
		c.value += time.Duration(ms-c.last) * time.Millisecond
	}
	c.last = ms
	return c.value
}

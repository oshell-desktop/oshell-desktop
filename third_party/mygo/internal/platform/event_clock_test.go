package platform

import (
	"testing"
	"time"
)

func TestMillisClockPreservesEventIntervalsAcrossWrap(t *testing.T) {
	for _, start := range []uint32{0, 100, 0x7ffffff0, 0xfffffff0} {
		var c MillisClock
		a := c.Expand(start)
		if a != time.Duration(start)*time.Millisecond {
			t.Fatal("first timestamp", a)
		}
		for _, delta := range []uint32{0, 1, 100, 1000, 86400000} {
			start += delta
			b := c.Expand(start)
			if b-a != time.Duration(delta)*time.Millisecond {
				t.Fatalf("interval=%v want=%dms", b-a, delta)
			}
			a = b
		}
	}
}

func TestMillisClockBackwardEventExpiresSeries(t *testing.T) {
	var c MillisClock
	a := c.Expand(1000)
	b := c.Expand(999)
	if b-a < 24*time.Hour {
		t.Fatal("backward event appeared to be a quick click")
	}
	if c.Expand(1099)-b != 100*time.Millisecond {
		t.Fatal("new series did not retain source intervals")
	}
}

package terminal

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
	"golang.org/x/text/unicode/norm"
	"oshell-desktop/internal/terminal/internal/vt"
)

const wordWhitespace = " \t\r\n\v\f\u0085\u00a0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u2028\u2029\u202f\u205f\u3000"

const selectionClickInterval = 500 * time.Millisecond

// The host click series and OShell's word seed have different lifetimes.
// Output, a third click, and a short pause need not invalidate the word seed.
type wordClickState struct {
	last        uint64
	interval    time.Duration
	intervalSet bool
	x, y        float64
	clicks      int
	seed        *wordClickSeed
}

type wordClickSettings struct {
	delimiters      string
	shiftWhitespace bool
}

type wordClickSeed struct {
	x, y, width, height float32
	time                uint64
	buffer              [2]uint64
	scroll              uint64
	bounds              [4]int // start row/column, end row/column, in buffer cells
	text                string
	settings            wordClickSettings
}

func (s *wordClickState) press(x, y, distanceX, distanceY float64, interval time.Duration, ns uint64) int {
	dx, dy := x-s.x, y-s.y
	if s.clicks == 0 || math.Abs(dx) > distanceX || math.Abs(dy) > distanceY || ns < s.last || ns-s.last > uint64(interval) {
		s.clicks = 1
	} else if s.clicks < 5 {
		// All counts after four use SwiftTerm's default physical-row branch.
		s.clicks++
	}
	s.last, s.x, s.y, s.interval = ns, x, y, interval
	s.intervalSet = true
	return s.clicks
}

func (v *view) continuesWordClick(x, y float32, ns uint64, buffer [2]uint64) bool {
	s := v.wordClicks.seed
	if s == nil || ns < s.time || ns-s.time > uint64(max(time.Second, 3*v.wordClicks.clickInterval())) {
		return false
	}
	return abs32(x-s.x) <= 4 && abs32(y-s.y) <= 4 &&
		v.overlay.width == s.width && v.overlay.height == s.height &&
		buffer == s.buffer && v.t.term.Scrollbar().Offset == s.scroll
}

func abs32(n float32) float32 {
	if n < 0 {
		return -n
	}
	return n
}

// wordPress follows TerminalPane.mouseDown: only a plain continuing fourth
// click becomes a word press. Compare the fresh delimited word before widening
// it, and consume the seed after successful expansion.
// The caller holds t.mu.
func (v *view) wordPress(ref vt.GridRef, ev ui.InputEvent, px, py float64, ns uint64) (vt.Selection, bool) {
	t := v.t.term
	interval, dx, dy := selectionClickInterval, float64(5), float64(5)
	if ev.HasDoubleClickSettings || ev.DoubleClickInterval > 0 {
		interval = max(0, ev.DoubleClickInterval)
		dx, dy = max(0, float64(ev.DoubleClickDistanceX)), max(0, float64(ev.DoubleClickDistanceY))
	}
	count := v.wordClicks.press(px, py, dx*float64(v.scale), dy*float64(v.scale), interval, ns)
	// UI delivery provides the window-wide series on every platform. The local
	// counter remains only for direct/synthetic callers without a count.
	if ev.Clicks > 0 {
		count = ev.Clicks
	}
	buffer, identityOK := t.BufferIdentity()
	continuing := identityOK && v.continuesWordClick(ev.X, ev.Y, ns, buffer)
	plain := ev.Mods&(ui.Shift|ui.Super|ui.Ctrl|ui.Alt) == 0
	if count != 2 && !(count == 4 && continuing && plain) {
		if !continuing || !t.SelectionActive() || (count != 1 && count != 3) {
			v.wordClicks.seed = nil
		}
		return v.gesture.PressCountWithShift(t, ref, px, py, ns, count, ev.Mods&ui.Shift != 0)
	}
	previous := v.wordClicks.seed
	if !continuing || !plain {
		previous = nil
	}
	v.wordClicks.seed = nil
	settings := wordClickSettings{v.t.opts.WordSeparators, v.t.opts.ShiftWhitespace}
	boundaries := wordWhitespace
	if ev.Mods&ui.Shift == 0 || !settings.shiftWhitespace {
		boundaries += settings.delimiters
	}
	v.gesture.SetWordBoundaries(boundaries)
	selection, selected := v.gesture.PressCountWithShift(t, ref, px, py, ns, 2, ev.Mods&ui.Shift != 0)
	if !selected || !plain {
		return selection, selected
	}
	a, b, c, d, word := selection.WordRange()
	if !word {
		return selection, selected
	}
	t.SetSelection(&selection)
	text, _ := t.SelectionText()
	bounds := [4]int{a, b, c, d}
	if previous != nil && previous.settings.shiftWhitespace == settings.shiftWhitespace &&
		canonicalWordEqual(previous.settings.delimiters, settings.delimiters) &&
		previous.bounds == bounds && canonicalWordEqual(previous.text, text) {
		// The reference expands at selection.start, not the pointer's cell.
		if start, ok := t.BufferCellAt(b, a); ok {
			v.gesture.SetWordBoundaries(wordWhitespace)
			return v.gesture.PressCount(t, start, px, py, ns, 2)
		}
	} else if identityOK && len(text) > 0 && len(text) <= 65536 {
		v.wordClicks.seed = &wordClickSeed{ev.X, ev.Y, v.overlay.width, v.overlay.height, ns, buffer, t.Scrollbar().Offset, bounds, text, settings}
	}
	return selection, selected
}

// Swift String equality compares canonically equivalent Unicode sequences.
func canonicalWordEqual(a, b string) bool {
	return a == b || norm.NFC.String(a) == norm.NFC.String(b)
}

func (s *wordClickState) clickInterval() time.Duration {
	if s.intervalSet || s.interval > 0 {
		return s.interval
	}
	return selectionClickInterval
}

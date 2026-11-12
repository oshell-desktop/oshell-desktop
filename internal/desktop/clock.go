// SPDX-License-Identifier: GPL-3.0-only
package desktop

import (
	"math"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

type workspaceClock struct {
	mu      sync.Mutex
	members map[*App]bool
	stop    chan struct{}
	text    string
}

func (c *workspaceClock) update(members map[*App]bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.members = members
	if len(members) == 0 {
		if c.stop != nil {
			close(c.stop)
			c.stop = nil
		}
		return
	}
	if c.stop != nil {
		return
	}
	c.text = core.LocalClockText(time.Now())
	stop := make(chan struct{})
	c.stop = stop
	go c.run(stop)
}
func (c *workspaceClock) run(stop <-chan struct{}) {
	timer := time.NewTimer(time.Until(time.Now().Truncate(time.Second).Add(time.Second)))
	defer timer.Stop()
	for {
		select {
		case <-stop:
			return
		case <-timer.C:
			text := core.LocalClockText(time.Now())
			c.mu.Lock()
			c.text = text
			members := make([]*App, 0, len(c.members))
			for a := range c.members {
				members = append(members, a)
			}
			c.mu.Unlock()
			for _, a := range members {
				if a.clockPending.CompareAndSwap(false, true) {
					a.post(func() { a.clockPending.Store(false); a.clockText = c.current() })
				}
			}
			timer.Reset(time.Until(time.Now().Truncate(time.Second).Add(time.Second)))
		}
	}
}
func (c *workspaceClock) current() string { c.mu.Lock(); defer c.mu.Unlock(); return c.text }
func (h *applicationHub) refreshClock() {
	members := map[*App]bool{}
	if clockVisible(h.config.Settings) {
		for _, a := range h.windows {
			if a.windowClockVisible() && a.win != nil && !a.stopped.Load() && a.win.IsVisible() && !a.win.IsMinimized() {
				for _, p := range a.visiblePanes() {
					if p.term != nil && !p.closed {
						members[a] = true
						break
					}
				}
			}
		}
	}
	h.clock.update(members)
}
func (a *App) windowClockVisible() bool {
	return clockVisible(a.config.Settings) && (!a.localUI.ClockFullscreenOnly || a.focusFullscreen.fullscreen)
}

func (a *App) clockOverlay(c *ui.Context, element ui.Element) {
	if !a.windowClockVisible() {
		return
	}
	text := a.clockText
	if text == "" {
		if a.hub != nil {
			text = a.hub.clock.current()
		}
		if text == "" {
			text = core.LocalClockText(time.Now())
		}
	}
	panes := a.visiblePanes()
	drawClockOverlay(element, a.config.Settings, text, func() []ui.Rect {
		// Layout has committed by the time DrawOver runs. Read the terminal
		// controls themselves so tab strips, pane headers and docks are excluded.
		var bodies []ui.Rect
		for _, pane := range panes {
			if pane.term != nil && !pane.closed {
				if r := pane.focus.Bounds(c); r.W > 0 && r.H > 0 {
					bodies = append(bodies, r)
				}
			}
		}
		return bodies
	})
}
func clockVisible(s core.Settings) bool { return s.TerminalClockEnabled && s.TerminalClockOpacity > 0 }
func drawClockOverlay(element ui.Element, settings core.Settings, text string, terminalBodies func() []ui.Rect) {
	settings.ClampClockStyle()
	if !clockVisible(settings) {
		return
	}
	family := "monospace"
	if settings.TerminalClockFontName != "" {
		family = settings.TerminalClockFontName + ", monospace"
	}
	font := ui.Font{Family: family, Size: float32(settings.TerminalClockFontSize), Weight: 500}
	colorText := settings.TerminalClockColor
	if colorText == "" {
		colorText = settings.Scheme().Foreground
	}
	color := ui.Hex(colorText).Alpha(float32(settings.TerminalClockOpacity))
	column, row := core.ClockGrid(settings.TerminalClockPosition)
	element.DrawOver(func(p *ui.Painter, r ui.Rect) {
		var bodies []ui.Rect
		if terminalBodies != nil {
			// Arranged terminals can extend beyond the scroll viewport.
			for _, body := range terminalBodies() {
				x, y := max(r.X, body.X), max(r.Y, body.Y)
				w, h := min(r.X+r.W, body.X+body.W)-x, min(r.Y+r.H, body.Y+body.H)-y
				if w > 0 && h > 0 {
					bodies = append(bodies, ui.Rect{X: x, Y: y, W: w, H: h})
				}
			}
			if len(bodies) == 0 {
				return
			}
			r = bodies[0]
			for _, body := range bodies[1:] {
				x, y := min(r.X, body.X), min(r.Y, body.Y)
				r = ui.Rect{X: x, Y: y, W: max(r.X+r.W, body.X+body.W) - x, H: max(r.Y+r.H, body.Y+body.H) - y}
			}
		}
		fit := font
		glyphs := ui.Shape(text, fit)
		metrics := fit.Metrics()
		width := float32(0)
		for _, g := range glyphs {
			width = max(width, g.X+g.Advance)
		}
		height := metrics.Ascent + metrics.Descent
		workspace := r
		workspaceInset := clockInset(workspace)
		if len(bodies) > 1 {
			ideal := ui.Rect{X: clockCoordinate(r.X, r.W, width, workspaceInset, column), Y: clockCoordinate(r.Y, r.H, height, workspaceInset, row), W: width, H: height}
			r = clockDrawingArea(workspace, bodies, ideal)
		}
		inset := clockInset(r)
		scale := min(float32(1), min(max(float32(1), r.W-2*inset)/max(width, 1), max(float32(1), r.H-2*inset)/max(height, 1)))
		if scale < 1 {
			fit.Size = max(float32(1), font.Size*scale)
			glyphs = ui.Shape(text, fit)
			metrics = fit.Metrics()
			width = 0
			for _, g := range glyphs {
				width = max(width, g.X+g.Advance)
			}
			height = metrics.Ascent + metrics.Descent
		}
		x := clockCoordinate(workspace.X, workspace.W, width, workspaceInset, column)
		y := clockCoordinate(workspace.Y, workspace.H, height, workspaceInset, row)
		x = clampClockOrigin(x, r.X, r.W, width, inset)
		y = clampClockOrigin(y, r.Y, r.H, height, inset)
		p.Clip(r, 0, func() { p.Glyphs(glyphs, x, y+metrics.Ascent, color) })
	})
}

func clockInset(r ui.Rect) float32 { return min(float32(12), min(r.W, r.H)/8) }

func clampClockOrigin(wanted, start, span, size, inset float32) float32 {
	return max(start+inset, min(wanted, start+span-size-inset))
}

func clockCoordinate(start, span, size, inset float32, position int) float32 {
	switch position {
	case 1:
		return start + (span-size)/2
	case 2:
		return start + span - size - inset
	default:
		return start + inset
	}
}

// Prefer the window-wide position, including across adjacent terminal views.
// If a group tab strip or another panel interrupts that row, use the nearest
// terminal body so the complete clock stays on terminal content.
func clockDrawingArea(workspace ui.Rect, bodies []ui.Rect, ideal ui.Rect) ui.Rect {
	covered := ideal.X
	for covered < ideal.X+ideal.W {
		next := covered
		for _, r := range bodies {
			if r.Y <= ideal.Y && ideal.Y+ideal.H <= r.Y+r.H && r.X <= covered+1 { // one DIP splitter
				next = max(next, r.X+r.W)
			}
		}
		if next == covered {
			break
		}
		covered = next
	}
	if covered >= ideal.X+ideal.W {
		return workspace
	}
	best := bodies[0]
	bestDistance := float32(math.Inf(1))
	for _, r := range bodies {
		inset := clockInset(r)
		dx := clampClockOrigin(ideal.X, r.X, r.W, ideal.W, inset) - ideal.X
		dy := clampClockOrigin(ideal.Y, r.Y, r.H, ideal.H, inset) - ideal.Y
		if distance := dx*dx + dy*dy; distance < bestDistance {
			best, bestDistance = r, distance
		}
	}
	return best
}

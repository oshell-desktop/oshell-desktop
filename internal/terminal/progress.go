package terminal

import (
	"github.com/egoist/mygo/ui"
	"time"
)

type terminalProgress struct {
	state, value int
	updated      time.Time
}

func (t *Terminal) setProgress(state, value int) {
	if state < 0 || state > 4 {
		return
	}
	t.progress = terminalProgress{state, max(0, min(100, value)), time.Now()}
}
func (t *Terminal) ProgressState(now time.Time) (state, value int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.term == nil || now.Sub(t.progress.updated) >= 15*time.Second {
		t.progress = terminalProgress{}
	}
	return t.progress.state, t.progress.value
}
func (t *Terminal) clearProgress() { t.mu.Lock(); t.progress = terminalProgress{}; t.mu.Unlock() }
func (t *Terminal) progressView(c *ui.Context, e ui.Element) {
	state, value := t.ProgressState(time.Now())
	if state == 0 {
		return
	}
	color := c.Theme().Accent
	if state == 2 {
		color = c.Theme().Danger
	} else if state == 4 {
		color = ui.Hex("#d89c22")
	}
	fraction := float32(value) / 100
	if state == 3 {
		fraction = .25 + float32(time.Now().UnixMilli()%1000)/2000
	}
	e.DrawOver(func(p *ui.Painter, r ui.Rect) {
		r.H = 4
		p.Fill(r, color.Alpha(.18), 0)
		r.W *= fraction
		p.Fill(r, color, 0)
	})
	c.After(100 * time.Millisecond)
}

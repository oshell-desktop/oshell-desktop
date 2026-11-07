package desktop

import (
	"github.com/egoist/mygo/ui"
	"time"
)

func (g *tabGroup) revealSelectedTab(row ui.Element, id string) {
	if g.lastRevealedTab != id {
		row.ScrollIntoView()
		g.lastRevealedTab = id
	}
}

func (a *App) tabDragScroll(c *ui.Context, scroll ui.Element, g *tabGroup) {
	// Register the container for drops in trailing space; individual tabs keep priority.
	if drag, dropped := ui.Drop[tabDrag](scroll); dropped {
		a.post(func() { a.receiveTabDrag(drag, g, "", false) })
	}
	drag, over := ui.DragOver[tabDrag](scroll)
	if over && !a.acceptsTabDrag(drag, g) {
		over = false
	}
	if !over && !g.dragOver {
		g.scrollTime = time.Time{}
		return
	}
	x, _, ok := scroll.PointerPosition()
	if !ok {
		return
	}
	now := time.Now()
	delta := float32(0)
	if !g.scrollTime.IsZero() {
		delta = float32(min(now.Sub(g.scrollTime).Seconds(), 0.05)) * 350
	}
	g.scrollTime = now
	if x < 28 {
		g.scroll.X = max(0, g.scroll.X-delta)
		c.AnimationFrame()
	} else if x > scroll.Bounds().W-28 {
		g.scroll.X = min(g.scroll.MaxX, g.scroll.X+delta)
		c.AnimationFrame()
	}
}

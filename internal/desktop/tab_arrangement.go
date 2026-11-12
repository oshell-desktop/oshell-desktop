package desktop

import (
	"math"
	"slices"

	"github.com/egoist/mygo/ui"
)

// OShell distributes tabs across near-square rows in tab order. A final
// short row fills its own width. Ratios include the space for child dividers.
func buildTabArrangement[T any](items []T, mode string, split func(T, T, bool, float32) T) T {
	var join func([]T, bool) T
	join = func(nodes []T, vertical bool) T {
		if len(nodes) == 1 {
			return nodes[0]
		}
		mid := len(nodes) / 2
		return split(join(nodes[:mid], vertical), join(nodes[mid:], vertical), vertical, float32(mid)/float32(len(nodes)))
	}
	if mode != "tile" {
		return join(items, mode == "vertical")
	}
	columns := int(math.Ceil(math.Sqrt(float64(len(items)))))
	rows := (len(items) + columns - 1) / columns
	lines := make([]T, 0, rows)
	offset := 0
	for i := range rows {
		count := len(items) / rows
		if i < len(items)%rows {
			count++
		}
		lines = append(lines, join(items[offset:offset+count], false))
		offset += count
	}
	return join(lines, true)
}

type viewportSize struct{ w, h float32 }

func splitMinimum(a, b viewportSize, vertical bool) viewportSize {
	if vertical {
		return viewportSize{max(a.w, b.w), a.h + 1 + b.h}
	}
	return viewportSize{a.w + 1 + b.w, max(a.h, b.h)}
}

func paneMinimum(n *node) viewportSize {
	if n == nil || n.pane != nil {
		return viewportSize{220, 140}
	}
	return splitMinimum(paneMinimum(n.first), paneMinimum(n.second), n.vertical)
}

func (t *tab) minimumViewport() viewportSize {
	if t.zoomed != nil {
		return viewportSize{220, 140}
	}
	return paneMinimum(t.root)
}

func arrangementCanvas(c *ui.Context, minimum viewportSize, draw func(viewportSize)) {
	viewport := ui.ScrollBoth(c).Fill().MinWidth(0).MinHeight(0).Label("排列滚动区")
	previous := viewport.Bounds()
	// Percentage sizes inside a scroll view resolve against unbounded content.
	// Size the document explicitly and recompute once after a viewport resize.
	size := viewportSize{max(previous.W, minimum.w), max(previous.H, minimum.h)}
	viewport.Draw(func(_ *ui.Painter, r ui.Rect) {
		if r.W != previous.W || r.H != previous.H {
			c.Invalidate()
		}
	}).Children(func() {
		ui.Box(c).Size(size.w, size.h).Shrink(0).Children(func() { draw(size) })
	})
}

// Size offscreen branches during layout too. Waiting for a clipped divider's
// paint callback would leave its first child at zero until scrolled into view.
func (a *App) arrangedSplit(c *ui.Context, n *groupNode, size, firstMin, secondMin viewportSize, first, second func(viewportSize)) {
	extent, lower, upper := size.w, firstMin.w, secondMin.w
	if n.vertical {
		extent, lower, upper = size.h, firstMin.h, secondMin.h
	}
	n.fitSplit(extent)
	n.size = max(lower, min(n.size, extent-1-upper))
	left, right := size, size
	if n.vertical {
		left.h = n.size
		right.h = extent - 1 - n.size
	} else {
		left.w = n.size
		right.w = extent - 1 - n.size
	}
	a.terminalSplit(c, &n.size, n.vertical, func() { first(left) }, func() { second(right) })
}

func (a *App) groupMinimum(n *groupNode) viewportSize {
	if n == nil {
		return viewportSize{}
	}
	if n.group != nil {
		if n.group.hidden {
			return viewportSize{}
		}
		size := viewportSize{320, 140}
		if t := a.groupTab(n.group); t != nil {
			v := t.minimumViewport()
			size.w = max(size.w, v.w)
			size.h = max(size.h, v.h)
		}
		size.h += detailBarHeight
		return size
	}
	left, right := a.groupMinimum(n.first), a.groupMinimum(n.second)
	if left.w == 0 {
		return right
	}
	if right.w == 0 {
		return left
	}
	return splitMinimum(left, right, n.vertical)
}

// A shared strip owns its own splitters. Rebuild their topology only when
// tabs or the arrangement change, so dragging and resizing retain the ratios.
type arrangedTab struct {
	tab           *tab
	first, second *arrangedTab
	split         groupNode
}
type sharedTabArrangement struct {
	mode string
	tabs []*tab
	root *arrangedTab
}

func (n *arrangedTab) minimum() viewportSize {
	if n.tab != nil {
		return n.tab.minimumViewport()
	}
	return splitMinimum(n.first.minimum(), n.second.minimum(), n.split.vertical)
}
func (a *App) drawArrangedTab(c *ui.Context, n *arrangedTab, size viewportSize) {
	if n.tab != nil {
		a.drawSingleTab(c, n.tab)
		return
	}
	a.arrangedSplit(c, &n.split, size, n.first.minimum(), n.second.minimum(), func(left viewportSize) { a.drawArrangedTab(c, n.first, left) }, func(right viewportSize) { a.drawArrangedTab(c, n.second, right) })
}
func (a *App) drawGroupTabs(c *ui.Context, mode string, tabs []*tab) {
	if len(tabs) == 1 {
		a.drawSingleTab(c, tabs[0])
		return
	}
	host := ui.Box(c).Fill().MinWidth(0).MinHeight(0)
	layout := ui.Local(host, "arrangement", func() sharedTabArrangement { return sharedTabArrangement{} })
	if mode != layout.mode || !slices.Equal(tabs, layout.tabs) {
		leaves := make([]*arrangedTab, len(tabs))
		for i, t := range tabs {
			leaves[i] = &arrangedTab{tab: t}
		}
		layout.root = buildTabArrangement(leaves, mode, func(left, right *arrangedTab, vertical bool, ratio float32) *arrangedTab {
			return &arrangedTab{first: left, second: right, split: groupNode{vertical: vertical, splitRatio: ratio}}
		})
		layout.tabs = slices.Clone(tabs)
		layout.mode = mode
	}
	host.Children(func() {
		arrangementCanvas(c, layout.root.minimum(), func(size viewportSize) { a.drawArrangedTab(c, layout.root, size) })
	})
}
func (a *App) drawSingleTab(c *ui.Context, t *tab) {
	if t.zoomed != nil {
		a.nodeView(c.Key(t.id), t, &node{id: t.zoomed.id, pane: t.zoomed})
		return
	}
	a.nodeView(c.Key(t.id), t, t.root)
}

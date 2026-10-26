// SPDX-License-Identifier: GPL-3.0-only
package desktop

import (
	"fmt"

	"github.com/egoist/mygo"
	"oshell-desktop/internal/core"
)

type tabDrag struct{ ID string }

func (a *App) moveTabBefore(source, target string) {
	a.moveTabRelative(source, target, false)
}
func (a *App) moveTabRelative(source, target string, after bool) {
	a.ensureGroups()
	if source == target {
		return
	}
	var moving, active *tab
	active = a.activeTab()
	var remaining []*tab
	for _, t := range a.tabs {
		if t.id == source {
			moving = t
		} else {
			remaining = append(remaining, t)
		}
	}
	if moving == nil {
		return
	}
	at := -1
	for i, t := range remaining {
		if t.id == target {
			at = i
			break
		}
	}
	if at < 0 {
		return
	}
	targetGroup := remaining[at].group
	if after {
		at++
	}
	remaining = append(remaining, nil)
	copy(remaining[at+1:], remaining[at:])
	moving.group = targetGroup
	remaining[at] = moving
	a.tabs = remaining
	for i, t := range a.tabs {
		if t == active {
			a.current = i
			break
		}
	}
	a.pruneGroups()
	if active != nil {
		a.setCurrentTab(active, false)
	}
}

func (a *App) ownerTab(p *pane) *tab {
	for _, t := range a.tabs {
		found := false
		a.walk(t.root, func(candidate *pane) { found = found || candidate == p })
		if found {
			return t
		}
	}
	return nil
}
func (a *App) arrangeActive(mode string) {
	t := a.activeTab()
	if t == nil {
		return
	}
	var panes []*pane
	a.walk(t.root, func(p *pane) { panes = append(panes, p) })
	w, h := float32(1200), float32(650)
	if a.win != nil {
		width, height := a.win.Size()
		w = float32(width)
		h = float32(height - 150)
	}
	if a.filesVisible {
		w -= 340
	}
	if t.root.bounds.W > 0 && t.root.bounds.H > 0 {
		w, h = t.root.bounds.W, t.root.bounds.H
	}
	var build func([]*pane, float32, float32) *node
	build = func(ps []*pane, width, height float32) *node {
		if len(ps) == 1 {
			return &node{id: core.NewID(), pane: ps[0]}
		}
		mid := len(ps) / 2
		vertical := mode == "vertical" || (mode == "tile" && height > width)
		n := &node{id: core.NewID(), vertical: vertical, splitRatio: float32(mid) / float32(len(ps))}
		if vertical {
			firstHeight := (height+1)*n.splitRatio - 1
			n.first = build(ps[:mid], width, firstHeight)
			n.second = build(ps[mid:], width, height-1-firstHeight)
		} else {
			firstWidth := (width+1)*n.splitRatio - 1
			n.first = build(ps[:mid], firstWidth, height)
			n.second = build(ps[mid:], width-1-firstWidth, height)
		}
		return n
	}
	if len(panes) > 0 {
		t.root = build(panes, w, h)
		t.active.focus.Focus()
	}
}
func (a *App) detachActive() {
	t := a.activeTab()
	if t == nil || t.root.pane != nil {
		return
	}
	p := t.active
	var remove func(*node) *node
	remove = func(n *node) *node {
		if n.pane == p {
			return nil
		}
		if n.pane != nil {
			return n
		}
		n.first = remove(n.first)
		n.second = remove(n.second)
		if n.first == nil {
			return n.second
		}
		if n.second == nil {
			return n.first
		}
		return n
	}
	t.root = remove(t.root)
	t.active = nil
	a.walk(t.root, func(q *pane) {
		if t.active == nil {
			t.active = q
		}
	})
	a.tabs = append(a.tabs, &tab{group: t.group, id: core.NewID(), name: p.profile.Name, root: &node{id: core.NewID(), pane: p}, active: p})
	a.current = len(a.tabs) - 1
	p.focus.Focus()
}
func (a *App) activeConnections() int {
	n := 0
	for _, t := range a.tabs {
		a.walk(t.root, func(p *pane) {
			if p.closed {
				return
			}
			if paneNeedsCloseConfirmation(p) {
				n++
			}
		})
	}
	return n
}
func (a *App) confirmExit(quit bool) {
	if a.features.closePending {
		return
	}
	a.features.closePending = true
	count := a.activeConnections()
	go func() {
		r, e := mygo.Dialog.Message(mygo.MessageOptions{Parent: a.win, Type: mygo.MessageQuestion, Title: "关闭 OShell", Message: fmt.Sprintf("结束 %d 个活动连接并关闭？", count), Detail: "正在进行的文件传输也会停止。", Buttons: []string{"取消", "结束连接并关闭"}, DefaultButton: 0, CancelButton: 0})
		a.post(func() {
			a.features.closePending = false
			if e == nil && r.Button == 1 {
				a.features.allowClose = true
				if quit {
					mygo.App.Quit()
				} else {
					a.win.Close()
				}
			}
		})
	}()
}

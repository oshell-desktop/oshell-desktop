// SPDX-License-Identifier: GPL-3.0-only
package desktop

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Groups own existing tabs, never connections. A hidden leaf stays alive.
type tabGroup struct {
	dragOver         bool
	scroll           ui.ScrollState
	scrollTime       time.Time
	id, name, active string
	hidden           bool
}
type groupNode struct {
	id                                    string
	group                                 *tabGroup
	first, second                         *groupNode
	vertical                              bool
	size                                  float32
	splitRatio, splitExtent, splitApplied float32
}
type groupState struct {
	root                 *groupNode
	active, recent       string
	dialog               bool
	editing, name, error string
	moveTab              string
	numberOpen           bool
	number               string
}

func groupLeaf(name string) *groupNode {
	return &groupNode{id: core.NewID(), group: &tabGroup{id: core.NewID(), name: name}}
}
func (n *groupNode) leaves() []*tabGroup {
	if n == nil {
		return nil
	}
	if n.group != nil {
		return []*tabGroup{n.group}
	}
	return append(n.first.leaves(), n.second.leaves()...)
}
func (n *groupNode) visible() bool {
	if n == nil {
		return false
	}
	if n.group != nil {
		return !n.group.hidden
	}
	return n.first.visible() || n.second.visible()
}
func (n *groupNode) replace(id string, replacement *groupNode) *groupNode {
	if n == nil {
		return nil
	}
	if n.group != nil {
		if n.group.id == id {
			return replacement
		}
		return n
	}
	n.first = n.first.replace(id, replacement)
	n.second = n.second.replace(id, replacement)
	if n.first == nil {
		return n.second
	}
	if n.second == nil {
		return n.first
	}
	return n
}
func (a *App) groupByID(id string) *tabGroup {
	for _, g := range a.groups.root.leaves() {
		if g.id == id {
			return g
		}
	}
	return nil
}
func (a *App) ensureGroups() {
	if a.groups.root == nil {
		a.groups.root = groupLeaf("")
		a.groups.active = a.groups.root.group.id
	}
	g := a.groupByID(a.groups.active)
	if g == nil {
		g = a.groups.root.leaves()[0]
		for _, candidate := range a.groups.root.leaves() {
			if !candidate.hidden {
				g = candidate
				break
			}
		}
		if !g.hidden {
			a.groups.active = g.id
		} else {
			a.groups.active = ""
		}
	}
	for _, t := range a.tabs {
		if a.groupByID(t.group) == nil {
			t.group = g.id
		}
		if owner := a.groupByID(t.group); owner.active == "" {
			owner.active = t.id
		}
	}
	if t := a.activeTab(); t != nil {
		g = a.groupByID(t.group)
		if !g.hidden {
			a.groups.active = g.id
			g.active = t.id
		}
	}
}
func (a *App) groupTabs(g *tabGroup) []*tab {
	var out []*tab
	for _, t := range a.tabs {
		if t.group == g.id {
			out = append(out, t)
		}
	}
	return out
}
func (a *App) groupTab(g *tabGroup) *tab {
	tabs := a.groupTabs(g)
	for _, t := range tabs {
		if t.id == g.active {
			return t
		}
	}
	if len(tabs) > 0 {
		g.active = tabs[0].id
		return tabs[0]
	}
	g.active = ""
	return nil
}
func (a *App) groupTitle(g *tabGroup) string {
	if g == nil {
		return "标签组"
	}
	if g.name != "" {
		return g.name
	}
	groups := a.groups.root.leaves()
	if len(groups) == 1 {
		return "默认分组"
	}
	used := map[string]bool{}
	for _, v := range groups {
		if v.name != "" {
			used[v.name] = true
		}
	}
	number := 1
	for _, v := range groups {
		if v.name != "" {
			continue
		}
		for used[fmt.Sprint("分组 ", number)] {
			number++
		}
		title := fmt.Sprint("分组 ", number)
		used[title] = true
		if v == g {
			return title
		}
		number++
	}
	return "标签组"
}
func (a *App) setCurrentTab(t *tab, focus bool) {
	if t == nil {
		a.current = -1
		return
	}
	if old := a.activeTab(); old != nil && old != t {
		a.groups.recent = old.id
	}
	for i, v := range a.tabs {
		if v == t {
			a.current = i
			break
		}
	}
	if g := a.groupByID(t.group); g != nil {
		g.hidden = false
		g.active = t.id
		a.groups.active = g.id
	}
	t.unread = false
	if focus && t.active != nil {
		t.active.focus.Focus()
	}
}
func (a *App) selectTab(t *tab) { a.ensureGroups(); a.setCurrentTab(t, true) }
func (a *App) activateGroup(g *tabGroup) {
	g.hidden = false
	a.groups.active = g.id
	if t := a.groupTab(g); t != nil {
		a.setCurrentTab(t, true)
	} else {
		a.current = -1
	}
}
func (a *App) chooseVisibleGroup() {
	for _, g := range a.groups.root.leaves() {
		if !g.hidden && g.id == a.groups.active {
			a.activateGroup(g)
			return
		}
	}
	for _, g := range a.groups.root.leaves() {
		if !g.hidden {
			a.activateGroup(g)
			return
		}
	}
	a.current = -1
	a.groups.active = ""
}
func (a *App) validateGroupName(name, except string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 80 || len(name) > 1024 || strings.ContainsFunc(name, unicode.IsControl) {
		return "", fmt.Errorf("分组名称需为 1–80 个字符，不能包含控制字符")
	}
	for _, g := range a.groups.root.leaves() {
		if g.id != except && strings.EqualFold(name, a.groupTitle(g)) {
			return "", fmt.Errorf("已存在同名标签组")
		}
	}
	return name, nil
}
func (a *App) createGroup(name string) (*tabGroup, error) {
	a.ensureGroups()
	name, e := a.validateGroupName(name, "")
	if e != nil {
		return nil, e
	}
	next := groupLeaf(name)
	a.groups.root = &groupNode{id: core.NewID(), first: a.groups.root, second: next}
	a.activateGroup(next.group)
	return next.group, nil
}
func (a *App) hideGroup(g *tabGroup, hidden bool) {
	g.hidden = hidden
	if hidden && a.groups.active == g.id {
		a.groups.active = ""
		a.chooseVisibleGroup()
	} else if !hidden && a.activeTab() == nil {
		a.activateGroup(g)
	}
}
func (a *App) showAllGroups() {
	for _, g := range a.groups.root.leaves() {
		g.hidden = false
	}
	if a.activeTab() == nil {
		a.chooseVisibleGroup()
	}
}
func (a *App) pruneGroups() {
	groups := a.groups.root.leaves()
	for _, g := range groups {
		if g.name == "" && len(a.groupTabs(g)) == 0 && len(a.groups.root.leaves()) > 1 {
			a.groups.root = a.groups.root.replace(g.id, nil)
		}
	}
	if a.groupByID(a.groups.active) == nil {
		a.groups.active = ""
	}
	for _, g := range a.groups.root.leaves() {
		a.groupTab(g)
	}
	if t := a.activeTab(); t == nil || a.groupByID(t.group) == nil || a.groupByID(t.group).hidden {
		a.chooseVisibleGroup()
	}
}
func (a *App) moveToGroup(t *tab, g *tabGroup) {
	if t == nil || g == nil || t.group == g.id {
		return
	}
	wasActive := a.activeTab() == t
	t.group = g.id
	g.active = t.id
	if !g.hidden {
		a.setCurrentTab(t, true)
	} else if wasActive {
		a.current = -1
		a.groups.active = ""
	}
	a.pruneGroups()
}
func (a *App) dissolveGroup(g *tabGroup) {
	var destination *tabGroup
	for _, v := range a.groups.root.leaves() {
		if v != g && (destination == nil || destination.hidden && !v.hidden) {
			destination = v
		}
	}
	if destination == nil {
		g.name = ""
		g.hidden = false
		return
	}
	active := a.activeTab()
	for _, t := range a.groupTabs(g) {
		t.group = destination.id
	}
	a.groups.root = a.groups.root.replace(g.id, nil)
	if active != nil && active.group == destination.id {
		a.setCurrentTab(active, true)
	} else {
		a.chooseVisibleGroup()
	}
}
func (a *App) arrangeTabs(mode string) {
	a.ensureGroups()
	active := a.activeTab()
	if mode == "tabs" {
		leaf := groupLeaf("")
		for _, t := range a.tabs {
			t.group = leaf.group.id
		}
		a.groups.root = leaf
		a.groups.active = leaf.group.id
		if active != nil {
			a.setCurrentTab(active, true)
		}
		return
	}
	var leaves []*groupNode
	groups := a.groups.root.leaves()
	preserve := false
	for _, group := range groups {
		if group.name != "" {
			preserve = true
			break
		}
	}
	if preserve {
		for _, group := range groups {
			leaves = append(leaves, &groupNode{id: core.NewID(), group: group})
		}
	} else {
		for _, t := range a.tabs {
			leaf := groupLeaf("")
			t.group = leaf.group.id
			leaf.group.active = t.id
			leaves = append(leaves, leaf)
		}
	}
	var build func([]*groupNode, int) *groupNode
	build = func(nodes []*groupNode, depth int) *groupNode {
		if len(nodes) == 1 {
			return nodes[0]
		}
		mid := len(nodes) / 2
		vertical := mode == "vertical" || mode == "tile" && depth%2 == 1
		return &groupNode{id: core.NewID(), first: build(nodes[:mid], depth+1), second: build(nodes[mid:], depth+1), vertical: vertical, splitRatio: float32(mid) / float32(len(nodes))}
	}
	if len(leaves) > 0 {
		a.groups.root = build(leaves, 0)
		a.setCurrentTab(active, true)
	}
}
func (a *App) dropTab(t *tab, target *tabGroup, position string) {
	if t == nil || target == nil {
		return
	}
	if position == "center" {
		a.moveToGroup(t, target)
		return
	}
	if t.group == target.id && len(a.groupTabs(target)) == 1 {
		return
	}
	leaf := groupLeaf("")
	old := &groupNode{id: core.NewID(), group: target}
	split := &groupNode{id: core.NewID(), first: old, second: leaf, vertical: position == "top" || position == "bottom"}
	if position == "left" || position == "top" {
		split.first, split.second = split.second, split.first
	}
	a.groups.root = a.groups.root.replace(target.id, split)
	t.group = leaf.group.id
	leaf.group.active = t.id
	a.setCurrentTab(t, true)
	a.pruneGroups()
}
func (a *App) tabByID(id string) *tab {
	for _, t := range a.tabs {
		if t.id == id {
			return t
		}
	}
	return nil
}
func (a *App) navigateTab(delta int) {
	a.ensureGroups()
	var tabs []*tab
	for _, t := range a.tabs {
		if g := a.groupByID(t.group); g != nil && !g.hidden {
			tabs = append(tabs, t)
		}
	}
	if len(tabs) == 0 {
		return
	}
	index := 0
	for i, t := range tabs {
		if t == a.activeTab() {
			index = i
			break
		}
	}
	a.selectTab(tabs[(index+delta+len(tabs))%len(tabs)])
}
func (a *App) selectTabNumber(number int) bool {
	a.ensureGroups()
	g := a.groupByID(a.groups.active)
	if g == nil {
		return false
	}
	tabs := a.groupTabs(g)
	if number < 1 || number > len(tabs) {
		return false
	}
	a.selectTab(tabs[number-1])
	return true
}
func (a *App) promptGroup(g *tabGroup) {
	a.ensureGroups()
	a.groups.dialog = true
	a.groups.editing = ""
	a.groups.moveTab = ""
	a.groups.name = ""
	a.groups.error = ""
	if g != nil {
		a.groups.editing = g.id
		a.groups.name = a.groupTitle(g)
	}
}
func (a *App) groupsDialog(c *ui.Context) {
	s := &a.groups
	ui.Modal(c, &s.dialog, func() {
		ui.Column(c).Width(440).Gap(12).Children(func() {
			ui.Text(c, "标签组名称").FontSize(18).Bold()
			ui.TextInput(c, &s.name).Label("分组名称").AutoFocus()
			if s.error != "" {
				ui.Text(c, s.error).TextColor(c.Theme().Danger)
			}
			ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
				ui.Button(c, "取消").OnClick(func() { s.dialog = false })
				ui.PrimaryButton(c, "确定").OnClick(func() {
					a.confirmGroupDialog()
				})
			})
		})
	})
	ui.Modal(c, &s.numberOpen, func() {
		ui.Column(c).Width(320).Gap(10).Children(func() {
			ui.Text(c, "跳到当前分组标签").Bold()
			input := ui.TextInput(c, &s.number).Label("标签编号").AutoFocus()
			submit := func() {
				n, e := strconv.Atoi(s.number)
				if e != nil || !a.selectTabNumber(n) {
					s.error = "编号不在当前分组范围内"
				} else {
					s.numberOpen = false
				}
			}
			if input.Submitted() {
				submit()
			}
			if s.error != "" {
				ui.Text(c, s.error).TextColor(c.Theme().Danger)
			}
			ui.Button(c, "跳转").OnClick(submit)
		})
	})
}
func (a *App) tabGroupMenu(m *ui.Menu, g *tabGroup) {
	a.tabGroupMenuFor(m, g, a.activeTab())
}
func (a *App) tabGroupMenuFor(m *ui.Menu, g *tabGroup, current *tab) {
	if m.Item("将当前标签移入此组").Disabled(current == nil || current.group == g.id).Chosen() {
		a.moveToGroup(current, g)
		return
	}
	if m.Item("重命名标签组…").Chosen() {
		a.promptGroup(g)
	}
	if m.Item("隐藏此组").Checked(g.hidden).Chosen() {
		a.hideGroup(g, !g.hidden)
	}
	if m.Item("仅显示此组").Chosen() {
		for _, v := range a.groups.root.leaves() {
			v.hidden = v != g
		}
		a.activateGroup(g)
	}
	if m.Item("解散此组（保留连接）").Chosen() {
		a.dissolveGroup(g)
	}
}
func (a *App) groupsMenu(m *ui.Menu) {
	a.ensureGroups()
	if m.Item("新建标签组…").Chosen() {
		a.promptGroup(nil)
	}
	if m.Item("显示全部标签组").Chosen() {
		a.showAllGroups()
	}
	m.Separator()
	for _, g := range a.groups.root.leaves() {
		title := a.groupMenuTitle(g)
		m.Submenu(title, func(m *ui.Menu) {
			if m.Item("切换到此组").Chosen() {
				a.activateGroup(g)
			}
			a.tabGroupMenu(m, g)
			m.Separator()
			for i, t := range a.groupTabs(g) {
				label := fmt.Sprintf("%d · %s", i+1, t.name)
				if t.unread {
					label += " ●"
				}
				if m.Item(label).Chosen() {
					a.selectTab(t)
				}
			}
		})
	}
}
func dropPosition(x, y, w, h float32) string {
	if w <= 0 || h <= 0 {
		return "center"
	}
	values := []float32{x / w, 1 - x/w, y / h, 1 - y/h}
	names := []string{"left", "right", "top", "bottom"}
	at := 0
	for i := range values {
		if values[i] < values[at] {
			at = i
		}
	}
	if values[at] < 0.27 {
		return names[at]
	}
	return "center"
}
func (a *App) groupWorkspace(c *ui.Context, n *groupNode) {
	if n == nil || !n.visible() {
		ui.Column(c).Fill().Center().Gap(12).Background(c.Theme().Background).Children(func() {
			ui.Text(c, "所有标签组已隐藏，连接继续运行。")
			ui.Button(c, "显示全部标签组").OnClick(a.showAllGroups)
		})
		return
	}
	if n.group == nil {
		if !n.first.visible() {
			a.groupWorkspace(c, n.second)
			return
		}
		if !n.second.visible() {
			a.groupWorkspace(c, n.first)
			return
		}
		a.groupSplit(c.Key(n.id), n, func() { a.groupWorkspace(c, n.first) }, func() { a.groupWorkspace(c, n.second) })
		return
	}
	g := n.group
	ui.Column(c.Key(g.id)).Fill().MinWidth(160).MinHeight(100).Children(func() {

		a.groupTabBar(c, g)
		box := ui.Column(c).Grow(1).MinHeight(0).FillWidth().Clip()
		position := "center"
		if drag, over := ui.DragOver[tabDrag](box); over && a.acceptsTabDrag(drag, g) {
			x, y, _ := box.PointerPosition()
			r := box.Bounds()
			position = dropPosition(x, y, r.W, r.H)
			if a.tabDragSource(drag.ID) != a {
				position = "center"
			}
			accent := c.Theme().Accent
			box.DrawOver(func(p *ui.Painter, r ui.Rect) {
				switch position {
				case "left":
					r.W /= 2
				case "right":
					r.X += r.W / 2
					r.W /= 2
				case "top":
					r.H /= 2
				case "bottom":
					r.Y += r.H / 2
					r.H /= 2
				}
				p.Fill(r, accent.Alpha(0.18), 0)
				p.Stroke(r, accent, 0, 2)
			})
		}
		if drag, ok := ui.Drop[tabDrag](box); ok {
			x, y, _ := box.PointerPosition()
			r := box.Bounds()
			where := dropPosition(x, y, r.W, r.H)
			a.post(func() {
				if !a.acceptsTabDrag(drag, g) {
					return
				}
				if a.tabDragSource(drag.ID) != a {
					a.receiveTabDrag(drag, g, "", false)
				} else {
					a.dropTab(a.tabByID(drag.ID), g, where)
				}
			})
		}
		box.Children(func() {
			if t := a.groupTab(g); t != nil {
				if t.zoomed != nil {
					a.nodeView(c, t, &node{id: t.zoomed.id, pane: t.zoomed})
				} else {
					a.nodeView(c, t, t.root)
				}
			} else if len(a.groups.root.leaves()) == 1 && g.name == "" && len(a.tabs) == 0 {
				a.welcome(c)
			} else {
				ui.Column(c).Fill().Center().Gap(10).Background(c.Theme().Background).Children(func() {
					ui.Text(c, "此标签组为空")
					ui.Button(c, "在此组打开本地终端").OnClick(func() { a.activateGroup(g); a.local() })
					ui.Button(c, "新建会话").OnClick(func() { a.activateGroup(g); a.edit(a.newProfile("ssh")) })
				})
			}
		})
	})
}

func (a *App) groupForNewTab() *tabGroup {
	a.ensureGroups()
	g := a.groupByID(a.groups.active)
	if g == nil {
		g = a.groups.root.leaves()[0]
		a.activateGroup(g)
	}
	g.hidden = false
	return g
}
func (a *App) visiblePanes() []*pane {
	if a.workspaceLocked() || a.stopped.Load() || a.win != nil && (!a.win.IsVisible() || a.win.IsMinimized()) {
		return nil
	}
	a.ensureGroups()
	var out []*pane
	for _, g := range a.groups.root.leaves() {
		if !g.hidden {
			if t := a.groupTab(g); t != nil {
				if t.zoomed != nil {
					out = append(out, t.zoomed)
				} else {
					a.walk(t.root, func(p *pane) { out = append(out, p) })
				}
			}
		}
	}
	return out
}
func (a *App) pruneSyncTargets() {
	valid := map[string]bool{}
	unavailable := false
	for _, p := range a.allPanes() {
		if !p.closed && a.features.syncTargets[p.id] {
			valid[p.id] = true
			if p.term == nil || !p.term.ManagedInputAvailable() {
				unavailable = true
			}
		}
	}
	if len(valid) < 2 || unavailable {
		a.features.syncEnabled = false
		valid = map[string]bool{}
	}
	a.features.syncTargets = valid
}

func (a *App) promptGroupWithTab(t *tab) {
	if t == nil || a.tabByID(t.id) != t {
		return
	}
	a.promptGroup(nil)
	a.groups.moveTab = t.id
}
func (a *App) confirmGroupDialog() {
	s := &a.groups
	var moving *tab
	if s.moveTab != "" {
		moving = a.tabByID(s.moveTab)
		if moving == nil {
			s.error = "要移动的标签已关闭"
			return
		}
	}
	if s.editing != "" {
		group := a.groupByID(s.editing)
		if group == nil {
			s.error = "标签组已不存在"
			return
		}
		name, err := a.validateGroupName(s.name, group.id)
		if err != nil {
			s.error = err.Error()
			return
		}
		group.name = name
	} else {
		group, err := a.createGroup(s.name)
		if err != nil {
			s.error = err.Error()
			return
		}
		if moving != nil {
			a.moveToGroup(moving, group)
		}
	}
	s.dialog = false
	s.moveTab = ""
	s.error = ""
}

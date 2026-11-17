package desktop

import "github.com/egoist/mygo/ui"

// Resolve the clicked tab explicitly; context menus can open on background tabs.
func (a *App) tabSessionMenu(m *ui.Menu, t *tab) bool {
	if t == nil || t.active == nil {
		return false
	}
	a.ensureGroups()
	p := t.active

	if m.Item("会话配置属性…").Disabled(p.closed || (p.profile.Kind != "ssh" && !p.terminalOnly)).Chosen() {
		a.selectTab(t)
		a.edit(p.profile)
		return true
	}
	if m.Item("当前会话属性…").Disabled(!canEditLiveKeepAlive(p)).Chosen() {
		a.selectTab(t)
		a.showLiveProperties()
		return true
	}
	if m.Item("重新连接").Disabled(p.closed).Chosen() {
		a.selectTab(t)
		a.reconnectActive()
		return true
	}
	if m.Item("交给 Agent…").Disabled(p.closed || p.agent == nil || p.term == nil || !p.term.Available() || p.term.InLocalToolsMode()).Chosen() {
		a.selectTab(t)
		a.quickAuthorizeAgent(p)
		return true
	}

	if m.Item("移动到新窗口").Disabled(a.hub == nil || a.modalActive()).Chosen() {
		a.selectTab(t)
		a.moveTabToNewWindow()
		return true
	}

	if m.Item("新建标签组并移入此标签…").Chosen() {
		a.promptGroupWithTab(t)
		return true
	}
	target := a.tabSplitTarget(t)
	if m.Item("垂直分割（左右）").Disabled(target == nil).Chosen() {
		a.dropTab(t, target, "right")
		return true
	}
	if m.Item("水平分割（上下）").Disabled(target == nil).Chosen() {
		a.dropTab(t, target, "bottom")
		return true
	}
	if group := a.groupByID(t.group); group != nil {
		m.Submenu("当前标签组", func(menu *ui.Menu) { a.tabGroupMenuFor(menu, group, t) })
	}
	if m.Item("合并为单组选项卡").Disabled(len(a.groups.root.leaves()) <= 1).Chosen() {
		a.arrangeTabs("tabs")
		return true
	}
	m.Submenu("排列", a.arrangementMenu)
	m.Separator()
	if m.Item("关闭当前分屏会话").Disabled(p.closed).Chosen() {
		a.requestSessionClose(t, p)
		return true
	}
	if m.Item("关闭整个标签").Chosen() {
		a.requestSessionClose(t, nil)
		return true
	}
	m.Separator()
	return false
}

func (a *App) tabSplitTarget(t *tab) *tabGroup {
	if t == nil || a.tabByID(t.id) != t {
		return nil
	}
	if group := a.groupByID(t.group); group != nil && len(a.groupTabs(group)) > 1 {
		return group
	}
	if selected := a.activeTab(); selected != nil && selected != t {
		return a.groupByID(selected.group)
	}
	return nil
}

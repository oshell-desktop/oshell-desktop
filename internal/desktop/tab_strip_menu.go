package desktop

import (
	"fmt"
	"github.com/egoist/mygo/ui"
)

func (a *App) tabStripMenu(m *ui.Menu, g *tabGroup, t *tab) {
	if m.Item("新建空白标签页").Chosen() {
		a.activateGroup(g)
		a.newBlank()
		return
	}
	if m.Item("新建本地终端").Chosen() {
		a.activateGroup(g)
		a.local()
		return
	}
	if m.Item("新建标签组…").Chosen() {
		a.promptGroup(nil)
		return
	}
	if m.Item("新建窗口").Disabled(a.hub == nil).Chosen() {
		a.hub.newWindow()
		return
	}
	if m.Item("会话管理…").Chosen() {
		a.activateGroup(g)
		a.openManager()
		return
	}
	m.Separator()
	if t == nil || t.active == nil || a.tabByID(t.id) != t {
		for _, label := range []string{"复制会话", "复制 SSH 渠道", "重新连接", "会话配置属性…", "当前会话属性…", "移动到新窗口", "垂直分割（左右）", "水平分割（上下）", "关闭整个标签"} {
			m.Item(label).Disabled(true)
		}
		m.Separator()
		a.tabGroupMenuFor(m, g, nil)
		return
	}
	// Existing action implementations explicitly select t when needed.
	if a.tabSessionMenu(m, t) {
		return
	}
	p := t.active
	if m.Item("复制会话").Disabled(p.closed).Chosen() {
		a.selectTab(t)
		profile := p.profile
		if profile.Kind == "blank" {
			profile = blankProfile()
		}
		a.requestSession(profile, sessionOpenOptions{afterTab: t.id}, p.terminalOnly)
		return
	}
	if m.Item("复制 SSH 渠道").Disabled(p.closed || p.conn == nil || !p.conn.Alive()).Chosen() {
		a.selectTab(t)
		a.cloneActive()
		return
	}
	if m.Item("专注全屏 / 退出").Checked(a.focusFullscreen.requested).Chosen() {
		a.selectTab(t)
		a.toggleFocusFullscreen()
		return
	}
	if m.Item("刷新主机信息").Disabled(p.closed).Chosen() {
		a.refreshHostIdentity(p)
		return
	}
	if m.Item("添加到快捷链接").Disabled(p.closed).Chosen() {
		a.selectTab(t)
		a.addCurrentLink()
		return
	}
	if m.Item("放大 / 恢复当前分屏").Chosen() {
		a.selectTab(t)
		a.togglePaneZoom()
		return
	}
	m.Submenu("移到标签组", func(menu *ui.Menu) {
		for _, destination := range a.groups.root.leaves() {
			if menu.Item(a.groupTitle(destination)).Disabled(destination == g).Chosen() {
				a.moveToGroup(t, destination)
			}
		}
	})
	m.Submenu("移动到窗口", func(menu *ui.Menu) {
		if a.hub != nil {
			for i, dest := range a.hub.windows {
				if menu.Item(fmt.Sprintf("窗口 %d", i+1)).Disabled(dest == a || !a.canMoveTabToWindow(dest)).Chosen() {
					a.transferTab(t, dest)
				}
			}
		}
	})
	tabs := a.groupTabs(g)
	index := -1
	for i, item := range tabs {
		if item == t {
			index = i
		}
	}
	if m.Item("向左移动").Disabled(index <= 0).Chosen() {
		a.moveTabBefore(t.id, tabs[index-1].id)
		a.selectTab(t)
	}
	if m.Item("向右移动").Disabled(index < 0 || index == len(tabs)-1).Chosen() {
		a.moveTabBefore(tabs[index+1].id, t.id)
		a.selectTab(t)
	}
}

package desktop

import (
	"fmt"
	"path"
	"runtime"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// The extra 48 points on restore prevent a row from flickering near the threshold.
func (a *App) toolbar(c *ui.Context) {
	width, height := c.Size()
	margin := float32(0)
	if a.toolbarMinimal {
		margin = 48
	}
	a.toolbarMinimal = width < 980+margin || height < 500+margin
	if a.toolbarMinimal {
		return
	}
	labels := width >= 1180
	text := func(value string) string {
		if labels {
			return value
		}
		return ""
	}
	p := a.activePane()
	terminal := p != nil && p.term != nil
	logging := false
	if terminal {
		_, active, err := p.term.LogState()
		logging = active
		if err != nil {
			a.status = "日志记录失败：" + err.Error()
		}
	}
	ui.Toolbar(c, func() {
		action := func(label string, icon *ui.SVG, caption, id string) ui.Element {
			return headerButton(c, label, icon, text(caption), false, false).OnClick(func() { a.performAction(id) })
		}
		menu := func(label string, icon *ui.SVG, build func(*ui.Menu)) ui.Element {
			return headerButton(c, label, icon, "", true, false).Menu(build)
		}
		divider := func() { ui.Box(c).Width(1).Height(18).Shrink(0).Background(c.Theme().Border) }
		hint := "显示会话侧栏"
		if a.sidebar.shown {
			hint = "隐藏会话侧栏"
		}
		if a.localUI.SidebarShortcut.Valid() {
			hint += "（" + a.localUI.SidebarShortcut.Display(runtime.GOOS) + "）"
		}
		headerButton(c, "会话侧栏", sidebarIcon, "", false, a.sidebar.shown).Role(ui.RoleToggleButton).Checked(a.sidebar.shown).Tooltip(hint).Disabled(a.modalActive()).OnClick(a.toggleSidebar)
		action("会话管理", catalogIcon, "会话管理", "sessionManager")
		menu("快捷连接", quickIcon, a.connectionMenu)
		action("新建会话", newSessionIcon, "新建", "newSession")
		action("本地终端", terminalIcon, "本地", "local")
		action("文件管理", folderIcon, "文件管理", "fileManager")
		divider()
		menu("分屏", splitIcon, a.splitActionsMenu).Disabled(p == nil || p.closed)
		groups := menu("标签组", groupsIcon, a.groupsMenu)
		if n := a.hiddenUnreadGroups(); n > 0 {
			groups.Background(c.Theme().Selection).Tooltip(fmt.Sprintf("%d 个隐藏标签组有新输出", n))
		}
		menu("分组排列", groupLayoutIcon, a.arrangementMenu).Disabled(len(a.tabs) == 0)
		menu("分屏排列", paneLayoutIcon, a.headerPaneMenu).Disabled(len(a.tabs) == 0)
		divider()
		action("搜索终端", searchIcon, "搜索", "find").Disabled(!terminal)
		caption, icon, hint := "记录", recordIcon, "开始记录终端日志"
		if logging {
			caption, icon, hint = "停止记录", stopIcon, "停止记录终端日志"
		}
		headerButton(c, "记录终端日志", icon, text(caption), false, logging).Role(ui.RoleToggleButton).Checked(logging).Tooltip(hint).Disabled(!terminal).OnClick(func() { a.performAction("logging") })
		menu("工具", toolsIcon, func(m *ui.Menu) { a.headerMenu(m, "工具") })
		mcpOpen := a.mcp.open || a.mcp.window != nil
		headerButton(c, "MCP 控制中心", mcpIcon, "MCP", false, mcpOpen).Role(ui.RoleToggleButton).Checked(mcpOpen).OnClick(func() {
			if a.mcp.window != nil {
				a.mcp.window.Close()
			} else if a.mcp.open {
				a.closeMCPPanel()
			} else {
				a.showMCP()
			}
		})
		if a.features.syncEnabled {
			headerButton(c, "停止同步输入", stopIcon, "", false, true).Tooltip(fmt.Sprintf("正在向 %d 个终端同步输入，点击停止", len(a.features.syncTargets))).OnClick(func() { a.performAction("stopSync") })
		}
		ui.Spacer(c)
		ui.Row(c.Key("properties")).Gap(0).Shrink(0).Children(func() {
			headerButton(c, "当前会话属性", propertiesIcon, "", false, false).Tooltip("编辑当前会话属性").Disabled(!a.canEditCurrentProfile()).OnClick(func() { a.performAction("currentProperties") })
			headerButton(c, "选择会话属性", chevronIcon, "", false, false).Width(28).Menu(a.propertiesMenu)
		})
		a.proxyToolbar(c)
		a.syncToolbar(c)
		a.updateToolbar(c)
		action("设置", gearIcon, "", "settings")
	}).Height(40).Padding(0, 8).Gap(8).AlignItems(ui.Center).Background(c.Theme().Background).BorderWidth(0, 0, 1, 0).BorderColor(c.Theme().Border).Label("主工具栏")
}

func headerButton(c *ui.Context, label string, icon *ui.SVG, text string, arrow, selected bool) ui.Element {
	b := ui.Button(c.Key(label), "").Label(label).Tooltip(label).Height(28).MinWidth(28).Shrink(0).Padding(2, 6).Gap(4).Radius(4).BorderWidth(0)
	switch {
	case b.Pressed():
		b.Background(c.Theme().SurfacePressed)
	case b.Hovered() || b.FocusVisible():
		b.Background(c.Theme().SurfaceHover)
	case selected:
		b.Background(c.Theme().Selection)
	default:
		b.Background(ui.Transparent)
	}
	return b.Children(func() {
		ui.Icon(c, icon).Size(14, 14)
		if text != "" {
			ui.Text(c, text).FontSize(12).SingleLine()
		}
		if arrow {
			ui.Icon(c, chevronIcon).Size(10, 10)
		}
	})
}

func (a *App) canEditCurrentProfile() bool {
	p := a.activePane()
	return !a.passwordLocked() && p != nil && !p.closed && (p.profile.Kind == "ssh" || p.terminalOnly)
}

func (a *App) propertiesMenu(m *ui.Menu) {
	if m.Item("当前会话").Disabled(!a.canEditCurrentProfile()).Chosen() {
		a.performAction("currentProperties")
	}
	if m.Item("默认会话").Disabled(a.passwordLocked()).Chosen() {
		a.performAction("defaults")
	}
	if a.passwordLocked() {
		return
	}
	m.Separator()
	entries, folders := sidebarEntries(a.presentationConfig(), "")
	type item struct {
		folder string
		entry  sidebarEntry
		order  sessionSortKey
	}
	children := map[string][]item{}
	for folder := range folders {
		if folder != "" {
			parent := managerParent(folder)
			children[parent] = append(children[parent], item{folder: folder, order: sessionSortKey{name: path.Base(folder), folder: true, pinned: parent == "" && a.localUI.Pins["d:"+folder]}})
		}
	}
	for _, e := range entries {
		children[e.group] = append(children[e.group], item{entry: e, order: sessionSortKey{name: e.name, pinned: e.group == "" && a.localUI.Pins["p:"+strings.ToLower(e.id)]}})
	}
	dark := interfaceDark(a.config.Settings)
	var draw func(*ui.Menu, string)
	draw = func(menu *ui.Menu, parent string) {
		rows := children[parent]
		if len(rows) == 0 {
			menu.Item("暂无会话").Disabled(true)
			return
		}
		sortSessionList(rows, func(row item) sessionSortKey { return row.order })
		for _, row := range rows {
			if row.folder != "" {
				menu.Submenu(row.order.name, func(sub *ui.Menu) { draw(sub, row.folder) }).Icon(sessionIconFor("", row.folder == "Links", true, false).menuIcon(dark))
				continue
			}
			e := row.entry
			if menu.ItemWithID(e.id, e.name).Icon(sidebarSessionStyle(e).menuIcon(dark)).Disabled(e.profile.Kind == "local").Chosen() {
				if p, ok := a.profileByID(e.profile.ID); ok && p.Kind != "local" {
					a.edit(p)
				}
			}
		}
	}
	draw(m, "")
}

func (a *App) splitActionsMenu(m *ui.Menu) {
	for _, choice := range [][2]string{{"splitVertical", "左右分屏"}, {"splitHorizontal", "上下分屏"}, {"zoomPane", "放大 / 还原当前分屏"}, {"previousPane", "上一个分屏"}, {"nextPane", "下一个分屏"}} {
		if m.Item(choice[1]).Chosen() {
			a.performAction(choice[0])
		}
	}
	m.Separator()
	for _, choice := range [][2]string{{"horizontal", "均分左右"}, {"vertical", "均分上下"}, {"tile", "瓷砖平铺"}} {
		if m.Item(choice[1]).Chosen() {
			a.arrangeActive(choice[0])
		}
	}
	if m.Item("当前分屏移到独立标签").Chosen() {
		a.detachActive()
	}
}

// Windows hosts its application menus in the workspace. Keep the launcher
// on the tab strip so all commands remain reachable with the toolbar folded.
func (a *App) workspaceMenus(c *ui.Context) {
	if runtime.GOOS == "darwin" {
		return
	}
	a.headerEditButton(c)
	headerButton(c, "更多", moreIcon, "", false, false).Tooltip("应用菜单").Menu(func(m *ui.Menu) {
		if a.toolbarMinimal {
			if m.Item("新建会话").Chosen() {
				a.performAction("newSession")
			}
			if m.Item("本地终端").Chosen() {
				a.performAction("local")
			}
			if m.Item("会话管理").Chosen() {
				a.performAction("sessionManager")
			}
			m.Submenu("分屏", a.splitActionsMenu).Disabled(a.activePane() == nil)
			m.Submenu("快捷连接", a.connectionMenu)
			m.Submenu("会话属性", a.propertiesMenu)
			m.Submenu("标签组", a.groupsMenu)
			m.Submenu("分组排列", a.arrangementMenu)
			m.Submenu("分屏排列", a.headerPaneMenu)
			if m.Item("会话侧栏").Checked(a.sidebar.shown).Chosen() {
				a.toggleSidebar()
			}
			if m.Item("文件管理").Chosen() {
				a.performAction("fileManager")
			}
			if m.Item("全局代理设置…").Chosen() {
				a.showGlobalProxy()
			}
			if m.Item("多设备同步").Chosen() {
				a.showSyncStatus()
			}
			if m.Item("MCP 控制中心").Chosen() {
				a.showMCP()
			}
			if m.Item("搜索终端").Disabled(a.activePane() == nil || a.activePane().term == nil).Chosen() {
				a.performAction("find")
			}
			m.Separator()
		}
		for _, title := range []string{"会话", "视图", "窗口", "工具", "快速命令", "OShell"} {
			m.Submenu(title, func(child *ui.Menu) { a.headerMenu(child, title) })
		}
	})
}

// Build on opening so saved sessions, commands and shortcuts are current.
// Reuse the application menu's actions, including modal and password guards.
func (a *App) headerItems(title string) []*mygo.MenuItem {
	h := a.hub
	if h == nil {
		h = &applicationHub{config: a.config, windows: []*App{a}}
	}
	for _, root := range h.menuItems() {
		if root.Label != title {
			continue
		}
		var out []*mygo.MenuItem
		for _, item := range root.Submenu {
			omit := false
			switch title {
			case "OShell":
				omit = item.ID == "newWindow" || item.ID == "newBlank" || item.ID == "local" || item.ID == "newSession"
			case "会话":
				omit = item.ID == "newWindow" || item.ID == "quickSwitch" || item.ID == "closePane" || item.ID == "closeTab"
			case "视图":
				omit = item.ID == "newGroup" || item.ID == "showGroups" || strings.HasPrefix(item.ID, "arrange")
				if item.ID == "splitVertical" {
					item.Label = "左右分屏"
				}
				if item.ID == "splitHorizontal" {
					item.Label = "上下分屏"
				}
			case "工具":
				omit = item.ID == "quickCommands" || item.ID == "appearance"
			}
			if omit || item.Type == mygo.MenuItemSeparator && (len(out) == 0 || out[len(out)-1].Type == mygo.MenuItemSeparator) {
				continue
			}
			out = append(out, item)
		}
		if len(out) > 0 && out[len(out)-1].Type == mygo.MenuItemSeparator {
			out = out[:len(out)-1]
		}
		return out
	}
	return nil
}

func (a *App) headerMenu(m *ui.Menu, title string) {
	if title == "快捷连接" {
		a.connectionMenu(m)
		return
	}
	a.headerMenuItems(m, a.headerItems(title))
	switch title {
	case "会话":
		m.Separator()
		for _, format := range []string{"SecureCRT", "Xshell"} {
			m.Submenu("导入 "+format, func(child *ui.Menu) {
				if child.Item("选择会话文件…").Chosen() {
					a.importThirdParty(format, false)
				}
				if child.Item("选择会话目录…").Chosen() {
					a.importThirdParty(format, true)
				}
			})
		}
		if m.Item("复制 SSH 渠道（免认证）").Disabled(a.activePane() == nil || a.activePane().conn == nil).Chosen() {
			a.cloneActive()
		}
	case "窗口":
		m.Separator()
		if m.Item("移动当前标签到新窗口").Disabled(a.activeTab() == nil).Chosen() {
			a.moveTabToNewWindow()
		}
	case "工具":
		m.Separator()
		if m.Item("rsync 目录传输…").Disabled(a.activePane() == nil || a.activePane().conn == nil).Chosen() {
			a.openStandaloneRsync(a.activePane())
		}
		if m.Item("保存终端历史文本…").Disabled(a.activePane() == nil || a.activePane().term == nil).Chosen() {
			a.exportScreen()
		}
	}
}

func (a *App) headerMenuItems(m *ui.Menu, items []*mygo.MenuItem) {
	for _, item := range items {
		if item.Hidden {
			continue
		}
		if item.Type == mygo.MenuItemSeparator {
			m.Separator()
			continue
		}
		if len(item.Submenu) > 0 {
			m.Submenu(item.Label, func(child *ui.Menu) { a.headerMenuItems(child, item.Submenu) }).Disabled(item.Disabled)
			continue
		}
		entry := m.Item(item.Label).Disabled(item.Disabled)
		switch item.ID {
		case "links":
			entry.Checked(a.config.Settings.LinksVisible)
		case "quickSend":
			entry.Checked(a.config.Settings.QuickSendVisible)
		case "composer":
			entry.Checked(a.features.operators.composerOpen)
		case "syncInput":
			entry.Checked(a.features.syncEnabled)
		case "focusFullscreen":
			entry.Checked(a.focusFullscreen.requested)
		case "logging":
			p := a.activePane()
			entry.Disabled(p == nil || p.term == nil)
			if p != nil && p.term != nil {
				_, active, _ := p.term.LogState()
				entry.Checked(active)
			}
		}
		if item.Type == mygo.MenuItemCheckbox || item.Type == mygo.MenuItemRadio {
			entry.Checked(item.Checked)
		}
		if item.ID != "" {
			bindings := a.config.Settings.KeyboardShortcuts.Bindings(item.ID, runtime.GOOS)
			if len(bindings) > 0 && strings.TrimSpace(item.Accelerator) != "" {
				entry.Shortcut(shortcutMods(bindings[0]), shortcutKey(bindings[0].KeyCode))
			}
		}
		if entry.Chosen() && item.Click != nil {
			item.Click(item, a.win)
		}
	}
}

// Native Edit roles route to the focused terminal or text input. Restore that
// input after activating the toolbar button, before opening the native menu.
func (a *App) headerEditButton(c *ui.Context) {
	button := headerButton(c, "编辑", editIcon, "", false, false).Role(ui.RoleMenuButton)
	open := func() {
		a.lastInputFocus.Focus()
		if a.win == nil || !mygo.App.IsReady() {
			return
		}
		bounds := button.Bounds()
		menu, window := mygo.NewMenu(a.headerItems("编辑")), a.win
		go mygo.RunOnMain(func() {
			if !a.stopped.Load() && window.IsVisible() {
				menu.PopupAt(window, int(bounds.X), int(bounds.Y+bounds.H))
			}
		})
	}
	button.OnClick(open).HandleInput(func(ev ui.InputEvent) bool {
		if ev.Kind == ui.InputKeyDown && ev.Key == ui.KeyDown {
			open()
			return true
		}
		return false
	})
}

func (a *App) headerPaneMenu(m *ui.Menu) {
	for _, choice := range arrangementChoices {
		if m.Item(choice[1]).Chosen() {
			a.arrangeTabs("tabs")
			if choice[0] != "tabs" {
				a.groups.root.group.sharedLayout = choice[0]
			}
			a.refreshPaneVisibility()
			return
		}
	}
}

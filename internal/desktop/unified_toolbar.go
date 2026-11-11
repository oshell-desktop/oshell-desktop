package desktop

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *App) toolbar(c *ui.Context) {
	if runtime.GOOS == "darwin" {
		a.legacyToolbar(c)
		return
	}
	if p := a.activePane(); p != nil && p.term != nil {
		if _, _, err := p.term.LogState(); err != nil {
			a.status = "日志记录失败：" + err.Error()
		}
	}
	width, _ := c.Size()
	compact := width < 1200
	buttonText := func(text string) string {
		if width < 1100 {
			return ""
		}
		return text
	}
	ui.Toolbar(c, func() {
		visible := a.sidebar.shown
		hint := "显示会话侧栏"
		if visible {
			hint = "隐藏会话侧栏"
		}
		if a.localUI.SidebarShortcut.Valid() {
			hint += "（" + a.localUI.SidebarShortcut.Display(runtime.GOOS) + "）"
		}
		sidebar := toolButton(c.Key("sidebar-toggle"), "会话侧栏", sidebarIcon, "").
			Shrink(0).Role(ui.RoleToggleButton).Checked(visible).Tooltip(hint).
			Disabled(a.passwordLocked() || a.modalActive()).OnClick(a.toggleSidebar)
		if visible {
			sidebar.Background(c.Theme().Selection)
			if sidebar.Pressed() {
				sidebar.Background(c.Theme().SurfacePressed)
			} else if sidebar.Hovered() {
				sidebar.Background(c.Theme().SurfaceHover)
			}
		}
		toolButton(c, "会话管理", folderIcon, buttonText("会话管理")).Shrink(0).OnClick(func() { a.performAction("sessionManager") })
		ui.MenuButton(c, "快捷连接", a.connectionMenu).Height(28).FontSize(12).Shrink(0)
		toolButton(c, "新建会话", plusIcon, buttonText("新建")).Shrink(0).OnClick(func() { a.performAction("newSession") })
		toolButton(c, "本地终端", terminalIcon, buttonText("本地")).Shrink(0).OnClick(func() { a.performAction("local") })
		toolButton(c, "文件管理", folderIcon, buttonText("文件管理")).Shrink(0).OnClick(func() { a.performAction("fileManager") })
		ui.Box(c).Width(1).Height(18).Background(c.Theme().Border).Shrink(0)
		ui.MenuButton(c, "工具", func(m *ui.Menu) { a.headerMenu(m, "工具") }).Height(28).FontSize(12).Shrink(0)
		label := "标签组"
		if n := a.hiddenUnreadGroups(); n > 0 {
			label = fmt.Sprintf("标签组 · %d", n)
		}
		groups := ui.MenuButton(c, label, a.groupsMenu).Label("标签组").Tooltip("新建、命名和显示/隐藏标签组").Height(28).FontSize(12).Shrink(0)
		if a.hiddenUnreadGroups() > 0 {
			groups.TextColor(c.Theme().Accent)
		}
		if compact {
			ui.MenuButton(c, "排列", func(m *ui.Menu) {
				m.Submenu("分组排列", a.arrangementMenu)
				m.Submenu("分屏排列", a.headerPaneMenu)
			}).Tooltip("标签排列、创建分屏和分屏布局").Height(28).FontSize(12).Shrink(0).Disabled(len(a.tabs) == 0)
		} else {
			ui.MenuButton(c, "分组排列", a.arrangementMenu).Tooltip("排列当前组的标签").Height(28).FontSize(12).Shrink(0).Disabled(len(a.tabs) == 0)
			ui.MenuButton(c, "分屏排列", a.headerPaneMenu).Tooltip("按窗口内所有标签重新排列分屏").Height(28).FontSize(12).Shrink(0).Disabled(len(a.tabs) == 0)
		}
		a.headerEditButton(c)
		ui.MenuButton(c, "更多", func(m *ui.Menu) {
			for _, title := range []string{"会话", "视图", "窗口", "快速命令", "OShell"} {
				m.Submenu(title, func(child *ui.Menu) { a.headerMenu(child, title) })
			}
		}).Tooltip("会话属性、视图、窗口、快速命令和应用菜单").Height(28).FontSize(12).Shrink(0)
		ui.Spacer(c)
		ui.Button(c, "MCP").Label("MCP 控制中心").Height(28).FontSize(12).Shrink(0).OnClick(a.showMCP)
		a.syncToolbar(c)
		a.updateToolbar(c)
		p := a.activePane()
		hasTerminal := p != nil && p.term != nil
		toolButton(c, "搜索终端", searchIcon, buttonText("搜索")).Shrink(0).Disabled(!hasTerminal).OnClick(func() { a.performAction("find") })
		logging := false
		if hasTerminal {
			_, logging, _ = p.term.LogState()
		}
		logText, logHint := "记录", "开始记录终端日志"
		if logging {
			logText, logHint = "停止记录", "停止记录终端日志"
		}
		logButton := toolButton(c, "记录终端日志", recordIcon, buttonText(logText)).Shrink(0).Role(ui.RoleToggleButton).Checked(logging).
			Tooltip(logHint).Disabled(!hasTerminal).OnClick(func() { a.performAction("logging") })
		if logging {
			logButton.Background(c.Theme().Selection)
		}
		toolButton(c, "设置", settingsIcon, "").Shrink(0).OnClick(func() { a.performAction("settings") })
	}).Height(detailBarHeight).Padding(2, 6).Gap(3).Background(c.Theme().Background).TextColor(c.Theme().Text).BorderWidth(0, 0, 1, 0).BorderColor(c.Theme().Border).Label("主工具栏")
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
	button := ui.Button(c, "编辑").Height(28).FontSize(12).Role(ui.RoleMenuButton)
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
	a.arrangementMenu(m)
	m.Separator()
	for _, choice := range [][2]string{{"splitVertical", "左右分屏"}, {"splitHorizontal", "上下分屏"}, {"zoomPane", "放大 / 还原当前分屏"}} {
		if m.Item(choice[1]).Chosen() {
			a.performAction(choice[0])
		}
	}
	for _, choice := range [][2]string{{"horizontal", "均分左右"}, {"vertical", "均分上下"}, {"tile", "瓷砖平铺"}} {
		if m.Item(choice[1]).Chosen() {
			a.arrangeActive(choice[0])
		}
	}
	if m.Item("当前分屏移到独立标签").Chosen() {
		a.detachActive()
	}
}

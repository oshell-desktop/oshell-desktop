// SPDX-License-Identifier: GPL-3.0-only
package desktop

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"oshell-desktop/internal/terminal"
)

func (a *App) theme(c *ui.Context) { a.themeSettings(c, a.config.Settings) }

func (a *App) themeSettings(c *ui.Context, settings core.Settings) {
	t := ui.LightTheme()
	t.Background = ui.Hex("#ffffff")
	t.Surface = ui.Hex("#f5f5f7")
	t.SurfaceHover = ui.Hex("#e9ebef")
	t.SurfacePressed = ui.Hex("#e0e5ed")
	t.Border = ui.Hex("#e2e4e8")
	t.Text = ui.Hex("#303238")
	t.TextMuted = ui.Hex("#878a92")
	t.Accent = ui.Hex("#5c82c8")
	t.AccentHover = ui.Hex("#4a72b9")
	t.AccentPressed = ui.Hex("#3c64aa")
	t.Selection = ui.Hex("#e8effb")
	dark := settings.Dark
	switch settings.InterfaceTheme {
	case "dark":
		dark = true
	case "light":
		dark = false
	case "system":
		if mygo.App.IsReady() {
			dark = systemInterfaceDark(mygo.Theme.IsDark())
		}
	}
	if dark {
		t = ui.DarkTheme()
		t.Background = ui.Hex("#202124")
		t.Surface = ui.Hex("#28292d")
		t.SurfaceHover = ui.Hex("#34363b")
		t.SurfacePressed = ui.Hex("#3a3d44")
		t.Border = ui.Hex("#3b3d42")
		t.Text = ui.Hex("#dedfe3")
		t.TextMuted = ui.Hex("#9699a2")
		t.Accent = ui.Hex("#8da9dd")
		t.Selection = ui.Hex("#344257")
	}
	t.Radius = 4
	t.Spacing = 3
	t.FontSize = 12
	c.SetTheme(t)
}
func (a *App) view(c *ui.Context) {
	defer a.resumeDeferredUpdate() // Modal teardown may happen after drain during this frame.
	a.drain()
	a.observeTabDetails(c)
	c.UseSoftwareRenderer(!a.config.Settings.Metal)
	a.theme(c)
	if a.lockedWorkspace(c) {
		return
	}
	a.ensureGroups()
	if a.features.syncEnabled && !a.features.syncOpen {
		a.pruneSyncTargets()
	}
	a.refreshPaneVisibility()
	ui.Column(c).Fill().Children(func() {
		if !a.focusFullscreen.chrome {
			a.toolbar(c)
			a.masterWarning(c)
			if a.config.Settings.LinksVisible {
				a.linksBar(c)
			}
		}
		ui.Row(c.Key("workspace-content")).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			if a.localUI.SidebarVisible && !a.focusFullscreen.chrome {
				a.sidebarView(c.Key("session-sidebar"))
			}
			a.groupWorkspace(c, a.groups.root)
			if t := a.activeTab(); a.filesVisible && t != nil && t.active.term != nil {
				a.filesView(c, t.active)
			}
		})
		if !a.focusFullscreen.deep {
			a.composerView(c.Key("composer"))
		}
		if a.config.Settings.QuickSendVisible && !a.focusFullscreen.deep {
			a.sendBar(c.Key("quick-send"))
		}
		if !a.focusFullscreen.chrome {
			a.statusView(c)
		}
	})
	a.rememberInputFocus(c)
	// Register shortcuts after terminal handles and initial focus are bound.
	// Otherwise the first Control key can be claimed by the previous frame.
	a.shortcuts(c)
	a.transferOverlays(c)
	a.dialogs(c)
}
func (a *App) legacyToolbar(c *ui.Context) {
	ui.Toolbar(c, func() {
		toolButton(c, "会话管理", folderIcon, "会话管理").OnClick(a.openManager)
		ui.MenuButton(c, "快捷连接", a.connectionMenu).Height(25).FontSize(12)
		toolButton(c, "新建会话", plusIcon, "新建").OnClick(func() { a.edit(a.newProfile("ssh")) })
		toolButton(c, "本地终端", terminalIcon, "本地").OnClick(a.local)
		toolButton(c, "当前会话属性", settingsIcon, "").Disabled(a.activePane() == nil || (a.activePane().profile.Kind != "ssh" && !a.activePane().terminalOnly)).OnClick(func() { a.edit(a.activePane().profile) })
		a.updateToolbar(c)
		toolButton(c, "默认会话属性", settingsIcon, "").OnClick(a.showDefaults)
		ui.MenuButton(c, "工具", func(m *ui.Menu) {
			if m.Item("专注全屏 / 退出").Checked(a.focusFullscreen.requested).Chosen() {
				a.toggleFocusFullscreen()
			}
			m.Separator()
			if m.Item("同步键盘输入…").Checked(a.features.syncEnabled).Chosen() {
				a.showSyncInput()
			}
			if m.Item("新建窗口").Chosen() && a.hub != nil {
				a.hub.newWindow()
			}
			if m.Item("移动当前标签到新窗口").Disabled(a.activeTab() == nil).Chosen() {
				a.moveTabToNewWindow()
			}
			if m.Item("显示快速发送栏").Checked(a.config.Settings.QuickSendVisible).Chosen() {
				a.toggleQuickSend()
			}
			if m.Item("多行命令撰写…").Chosen() {
				a.openComposer(a.quick, !a.features.operators.noReturn)
			}
			if m.Item("突出显示集…").Chosen() {
				a.showHighlights()
			}
			if m.Item("本机网络工具").Chosen() {
				a.newBlank()
			}
			if m.Item("当前会话空闲保活…").Disabled(!canEditLiveKeepAlive(a.activePane())).Chosen() {
				a.showLiveProperties()
			}
			if m.Item("快速命令…").Chosen() {
				a.openCommands()
			}
			if m.Item("共享代理管理…").Chosen() {
				a.showProxyManager()
			}
			if m.Item("密码保护…").Chosen() {
				a.showVault()
			}
			m.Separator()
			if m.Item("复制 SSH 渠道（免认证）").Disabled(a.activePane() == nil || a.activePane().conn == nil).Chosen() {
				a.cloneActive()
			}
			if m.Item("保存终端历史文本…").Disabled(a.activePane() == nil || a.activePane().term == nil).Chosen() {
				a.exportScreen()
			}
			m.Separator()
			if m.Item("导入会话…").Chosen() {
				a.importProfiles()
			}
			m.Submenu("导入 SecureCRT", func(m *ui.Menu) {
				if m.Item("选择 XML / INI 文件…").Chosen() {
					a.importThirdParty("SecureCRT", false)
				}
				if m.Item("选择 Sessions 目录…").Chosen() {
					a.importThirdParty("SecureCRT", true)
				}
			})
			m.Submenu("导入 Xshell", func(m *ui.Menu) {
				if m.Item("选择 XSH / XTS / ZIP 文件…").Chosen() {
					a.importThirdParty("Xshell", false)
				}
				if m.Item("选择会话目录…").Chosen() {
					a.importThirdParty("Xshell", true)
				}
			})
			if m.Item("导出会话…").Chosen() {
				a.exportProfiles()
			}
			m.Separator()
			if m.Item("远程文件 · SFTP").Disabled(a.activePane() == nil || a.activePane().conn == nil).Chosen() {
				a.toggleFiles()
			}
		}).Height(25).FontSize(12)
		toolButton(c, "分屏", splitIcon, "").Disabled(a.activePane() == nil).Menu(func(m *ui.Menu) {
			if m.Item("左右分屏").Chosen() {
				a.splitActivePane(1)
			}
			if m.Item("放大 / 恢复当前分屏").Chosen() {
				a.togglePaneZoom()
			}
			if m.Item("上一个分屏").Chosen() {
				a.cyclePane(-1)
			}
			if m.Item("下一个分屏").Chosen() {
				a.cyclePane(1)
			}
			if m.Item("上下分屏").Chosen() {
				a.splitActivePane(2)
			}
		})
		hiddenUnread := a.hiddenUnreadGroups()
		groupLabel := "标签组"
		hint := "新建、命名和显示/隐藏标签组"
		if hiddenUnread > 0 {
			groupLabel = fmt.Sprintf("标签组 · %d", hiddenUnread)
			hint = fmt.Sprintf("%d 个隐藏标签组有新输出；连接继续运行", hiddenUnread)
		}
		groupButton := ui.MenuButton(c, groupLabel, a.groupsMenu).Label("标签组").Tooltip(hint).Height(25)
		if hiddenUnread > 0 {
			groupButton.TextColor(c.Theme().Accent)
		}
		ui.MenuButton(c, "排列", func(m *ui.Menu) {
			a.arrangementMenu(m)
			m.Separator()
			if m.Item("均分左右").Chosen() {
				a.arrangeActive("horizontal")
			}
			if m.Item("均分上下").Chosen() {
				a.arrangeActive("vertical")
			}
			if m.Item("瓷砖平铺").Chosen() {
				a.arrangeActive("tile")
			}
			m.Separator()
			if m.Item("当前分屏移到独立标签").Chosen() {
				a.detachActive()
			}
		}).Height(25).Disabled(len(a.tabs) == 0)
		ui.Spacer(c)
		toolButton(c, "搜索终端", searchIcon, "搜索").Disabled(a.activePane() == nil || a.activePane().term == nil).OnClick(a.openSearch)
		logging := false
		if p := a.activePane(); p != nil && p.term != nil {
			var logError error
			_, logging, logError = p.term.LogState()
			if logError != nil {
				a.status = "日志记录失败：" + logError.Error()
			}
		}
		logLabel := "记录"
		if logging {
			logLabel = "停止记录"
		}
		toolButton(c, "记录终端日志", nil, logLabel).Disabled(a.activePane() == nil || a.activePane().term == nil).OnClick(a.toggleLog)
		toolButton(c, "远程文件", folderIcon, "文件").Disabled(a.activePane() == nil || a.activePane().conn == nil).OnClick(a.toggleFiles)
		toolButton(c, "设置", settingsIcon, "").OnClick(func() { a.openSettings() })
		toolButton(c, "会话侧栏", sidebarIcon, "").OnClick(a.toggleSidebar)
	}).Height(38).Padding(6, 8).Gap(5).Background(c.Theme().Background).BorderWidth(0, 0, 1, 0).BorderColor(c.Theme().Border).Label("主工具栏")
}
func (a *App) toggleFiles() {
	if a.hub != nil {
		a.hub.openFilesFromPane(a.activePane())
		return
	}

	a.filesVisible = !a.filesVisible
	if a.filesVisible {
		p := a.activePane()
		if p == nil {
			return
		}
		dir := p.profile.InitialDirectory
		if dir == "" {
			dir = "."
		}
		a.loadFiles(p, dir)
	}
}
func (a *App) connectionMenu(m *ui.Menu) {
	if m.Item("快速切换 / 连接…").Chosen() {
		a.showQuickSwitch()
	}
	m.Separator()
	tree := quickConnectionTree(a.presentationConfig().Profiles)
	if len(tree) == 0 {
		m.Item("暂无快捷会话").Disabled(true)
	} else {
		a.populateQuickConnections(m, tree)
	}
	m.Separator()
	if m.Item("管理会话…").Chosen() {
		a.openManager()
	}
}

type linkRootDrag struct {
	Snapshot   *core.Config
	Generation uint64
	Key        string
	Owner      *App
}

func (a *App) moveLinkRoot(source, before string) {
	candidate := a.config.Clone()
	changed, e := candidate.ReorderLinkRoot(source, before)
	if e != nil {
		a.status = "错误：" + e.Error()
		return
	}
	if !changed {
		return
	}
	a.changeConfig(func(config *core.Config) error { *config = candidate; return nil })
}
func (a *App) linksBar(c *ui.Context) {
	ui.Row(c).Height(30).Padding(2, 8).Gap(3).BorderWidth(0, 0, 1, 0).BorderColor(c.Theme().Border).Children(func() {
		flatButton(c, "添加当前会话到链接栏", plusIcon, "").Disabled(a.activePane() == nil || a.activePane().profile.Kind == "blank").OnClick(a.addCurrentLink)
		linkDragOver := false
		scroll := ui.ScrollHorizontal(c).TrackScroll(&a.linkScroll).Grow(1).MinWidth(0).Height(26).Label("快捷链接栏")
		scroll.ContextMenu(func(menu *ui.Menu) { a.linkContextMenu(menu, nil) })
		scroll.Children(func() {
			ui.Row(c).Gap(5).Children(func() {
				items := a.config.OrderedLinkRootItems()
				for index, item := range items {
					var element ui.Element
					switch item.Kind {
					case "folder":
						openFolder := func() { a.openLinkMenu(item.Key(), item.Folder) }
						element = flatButton(c.Key(item.Key()), "快捷目录 "+item.Name, folderIcon, item.Name).OnClick(openFolder).Role(ui.RoleMenuButton)
						element.HandleInput(func(event ui.InputEvent) bool {
							if event.Kind == ui.InputKeyDown && event.Key == ui.KeyDown {
								openFolder()
								return true
							}
							return false
						})
					case "link", "session":
						profile, ok := a.profileByID(item.ProfileID)
						if !ok {
							continue
						}
						icon, label := linkIcon, "快捷链接 "
						if item.Kind == "session" {
							icon, label = terminalIcon, "快捷会话 "
						}
						element = flatButton(c.Key(item.Key()), label+item.Name, icon, item.Name).OnClick(func() { a.request(profile, 0) })
					}
					element.Drag(a.linkDrag(item.Key()))
					if item.Kind == "folder" {
						a.linkFolderPopover(c.Key(item.Key()+"-menu"), element, item.Key())
					}
					if drag, hover := ui.DragOver[linkRootDrag](element); hover && a.acceptsLinkDrag(drag) {
						linkDragOver = true
						x, _, _ := element.PointerPosition()
						rect := element.Bounds()
						_, folder := linkDropDestination(drag.Key, item, index, items, x/max(rect.W, 1))
						if folder != "" {
							element.BorderWidth(1).BorderColor(c.Theme().Accent)
						} else if x < rect.W/2 {
							element.BorderWidth(0, 0, 0, 2).BorderColor(c.Theme().Accent)
						} else {
							element.BorderWidth(0, 2, 0, 0).BorderColor(c.Theme().Accent)
						}
					}
					if drag, dropped := ui.Drop[linkRootDrag](element); dropped && a.acceptsLinkDrag(drag) {
						x, _, _ := element.PointerPosition()
						rect := element.Bounds()
						before, folder := linkDropDestination(drag.Key, item, index, items, x/max(rect.W, 1))
						a.post(func() { a.applyLinkDrop(drag, before, folder) })
					}
					element.ContextMenu(func(menu *ui.Menu) { a.linkContextMenu(menu, &item) })
				}
				tail := ui.Box(c).MinWidth(30).Height(24)
				if len(items) == 0 {
					tail.Children(func() { ui.Text(c, "快捷链接 /Links").FontSize(11).TextColor(c.Theme().TextMuted).Padding(0, 4) })
				}
				if drag, dropped := ui.Drop[linkRootDrag](tail); dropped && a.acceptsLinkDrag(drag) {
					a.post(func() { a.applyLinkDrop(drag, "", "") })
				}
			})
		})
		if drag, dropped := ui.Drop[linkRootDrag](scroll); dropped && a.acceptsLinkDrag(drag) {
			a.post(func() { a.applyLinkDrop(drag, "", "") })
		}
		a.scrollLinkDrag(c, scroll, linkDragOver)
		all := flatButton(c, "所有快捷链接", chevronIcon, "").OnClick(func() { a.openLinkMenu("all", "") })
		a.linkFolderPopover(c.Key("all-link-menu"), all, "all")
	})
}
func (a *App) profileByID(id string) (core.Profile, bool) {
	for _, p := range a.config.Profiles {
		if p.ID == id {
			return p, true
		}
	}
	return core.Profile{}, false
}
func (a *App) linksMenu(m *ui.Menu, folder string) {
	if folder == "" {
		for _, item := range a.config.OrderedLinkRootItems() {
			if item.Kind == "folder" {
				m.Submenu(item.Name, func(menu *ui.Menu) { a.linksMenu(menu, item.Folder) })
			} else if profile, ok := a.profileByID(item.ProfileID); ok {
				if m.Item(item.Name).Chosen() {
					a.request(profile, 0)
				}
			}
		}
		m.Separator()
		if m.Item("管理此快捷目录…").Chosen() {
			a.managerGroup = "Links"
			a.managerAll = false
			a.query = ""
			a.openManager()
		}
		return
	}

	full := "Links"
	if folder != "" {
		full += "/" + folder
	}
	for _, d := range a.config.AllDirectories() {
		if strings.HasPrefix(d, full+"/") {
			rest := strings.TrimPrefix(d, full+"/")
			if !strings.Contains(rest, "/") {
				next := strings.TrimPrefix(d, "Links/")
				m.Submenu(rest, func(m *ui.Menu) { a.linksMenu(m, next) })
			}
		}
	}
	for _, l := range a.config.Links {
		if l.Folder == folder {
			if p, ok := a.profileByID(l.ProfileID); ok {
				if m.Item(l.Name).Chosen() {
					a.request(p, 0)
				}
			}
		}
	}
	for _, p := range a.config.Profiles {
		if p.Group == full {
			if m.Item(p.Name).Chosen() {
				a.request(p, 0)
			}
		}
	}
	m.Separator()
	if m.Item("管理此快捷目录…").Chosen() {
		a.managerGroup = full
		a.managerAll = false
		a.query = ""
		a.openManager()
	}
}
func (a *App) newTabMenu(m *ui.Menu) {
	if m.Item("新建空白标签（网络工具）").Chosen() {
		a.newBlank()
	}
	if m.Item("新建 SSH 会话…").Chosen() {
		a.edit(a.newProfile("ssh"))
	}
	if m.Item("打开本地终端").Chosen() {
		a.local()
	}
	m.Separator()
	if m.Item("会话管理…").Chosen() {
		a.openManager()
	}
}
func (a *App) groupTabBar(c *ui.Context, g *tabGroup) {
	tabs := a.groupTabs(g)
	backgroundMenu := func(m *ui.Menu) { a.tabStripMenu(m, g, a.groupTab(g)) }
	widths := make([]float32, len(tabs))
	totalWidth := float32(0)
	for i, t := range tabs {
		t.name = a.displayTabTitle(t)
		widths[i] = preferredTabWidth(fmt.Sprintf("%d  %s", i+1, t.name))
		totalWidth += widths[i] + 1
	}
	if totalWidth > 0 {
		totalWidth--
	}
	ui.Row(c).ContextMenu(backgroundMenu).Height(36).Padding(3, 6).Gap(6).Background(c.Theme().Surface).BorderWidth(0, 0, 1, 0).BorderColor(c.Theme().Border).Children(func() {
		if len(a.groups.root.leaves()) > 1 || g.name != "" {
			title := fmt.Sprintf("%s · %d", a.groupTitle(g), len(tabs))
			color := c.Theme().Text
			if a.groups.active == g.id {
				color = c.Theme().Accent
			}
			if a.groupUnread(g) {
				title = "● " + title
				color = tabUnreadColor(c)
			}
			ui.Button(c, "").Menu(func(m *ui.Menu) {
				if m.Item("切换到此组").Chosen() {
					a.activateGroup(g)
				}
				a.tabGroupMenu(m, g)
			}).ContextMenu(backgroundMenu).Label(a.groupTitle(g)).Tooltip(title+" · 标签组操作").Height(30).MaxWidth(140).MinWidth(65).Padding(2, 4).BorderWidth(0).Background(ui.Transparent).TextColor(color).Gap(5).Children(func() {
				ui.Text(c, title).FontSize(11).Bold().SingleLine().Grow(1).MinWidth(0).TextColor(color)
				ui.Icon(c, chevronIcon).FontSize(12).TextColor(color)
			})
		}
		g.dragOver = false
		scroll := ui.ScrollHorizontal(c).TrackScroll(&g.scroll).Width(totalWidth).Shrink(1).MinWidth(0).Height(30).ContextMenu(backgroundMenu).Children(func() {
			ui.Row(c).Gap(1).Children(func() {
				for i, t := range tabs {
					row := ui.Row(c.Key(t.id)).Height(30).Gap(0).Padding(0, 4).Width(widths[i]).Shrink(0).Radius(5).Background(c.Theme().Background.Mix(c.Theme().Text, 0.02)).BorderColor(ui.Transparent)
					if t.id == g.active {
						row.Background(c.Theme().Surface.Mix(c.Theme().Text, 0.10)).DrawOver(func(p *ui.Painter, r ui.Rect) {
							p.Fill(ui.Rect{X: r.X, Y: r.Y + r.H - 2, W: r.W, H: 2}, terminalFlame, 0)
						})
					}
					if drag, over := ui.DragOver[tabDrag](row); over && a.acceptsTabDrag(drag, g) {
						g.dragOver = true
						x, _, _ := row.PointerPosition()
						if x < row.Bounds().W/2 {
							row.BorderColor(terminalFlame).BorderWidth(0, 0, 0, 2)
						} else {
							row.BorderColor(terminalFlame).BorderWidth(0, 2, 0, 0)
						}
					}
					if drag, dropped := ui.Drop[tabDrag](row); dropped {
						target := t.id
						x, _, _ := row.PointerPosition()
						after := x >= row.Bounds().W/2
						a.post(func() { a.receiveTabDrag(drag, g, target, after) })
					}
					row.ContextMenu(func(m *ui.Menu) { a.tabStripMenu(m, g, t) })
					row.Children(func() {
						connected, status := tabConnectionStatus(t)
						dot := ui.Hex("#E5534B")
						if connected {
							dot = ui.Hex("#32B85A")
						}
						ui.Text(c, "●").FontSize(8).TextColor(dot).Tooltip(status)

						title := tabTitleButton(c, t.name, fmt.Sprintf("%d  %s", i+1, t.name), t.id == g.active, t.unread).ContextMenu(func(m *ui.Menu) { a.tabStripMenu(m, g, t) }).Grow(1).MinWidth(0)
						a.tabDetailsHover(c, title, t)
						a.terminalTabDrag(title, t)
						a.tabTitleEvents(title, t)
						tabBarButton(c, "关闭 "+t.name, closeIcon).ContextMenu(func(m *ui.Menu) { a.tabStripMenu(m, g, t) }).Width(22).OnClick(func() { a.requestSessionClose(t, nil) })
					})
				}
			})
		})
		a.tabDragScroll(c, scroll, g)
		tabBarButton(c, "新建标签", plusIcon).ContextMenu(backgroundMenu).Menu(func(m *ui.Menu) { a.activateGroup(g); a.newTabMenu(m) })
		space := ui.Box(c).Grow(1).MinWidth(24).Height(30).Label("标签栏空白区域").ContextMenu(backgroundMenu)
		a.tabStripDropSpace(space, g)
		tabBarButton(c, "所有标签", chevronIcon).ContextMenu(backgroundMenu).Disabled(len(tabs) == 0).Menu(func(m *ui.Menu) {
			for _, t := range tabs {
				title := t.name
				if t.unread {
					title = "● " + title + "（新输出）"
				}
				if m.Item(title).Checked(t.id == g.active).Chosen() {
					a.selectTab(t)
				}
			}
		})
	})
}
func (a *App) welcome(c *ui.Context) {
	ui.Column(c).Grow(1).FillHeight().Center().Gap(8).Background(c.Theme().Background).Children(func() {
		ui.Image(c, brandImage()).Size(80, 80)
		ui.Text(c, "OShell").FontSize(30).Bold()
		ui.Text(c, "连接你的服务器，专注每一条命令。").FontSize(13).TextColor(c.Theme().TextMuted)
		ui.Row(c).Gap(10).Margin(3, 0).Children(func() {
			toolButton(c, "新建 SSH 会话", plusIcon, "新建 SSH 会话").OnClick(func() { a.edit(a.newProfile("ssh")) })
			toolButton(c, "打开本地终端", terminalIcon, "打开本地终端").OnClick(a.local)
		})
		ui.Text(c, "通过工具栏管理会话、创建分屏；使用底部输入栏快速发送命令。").FontSize(11).TextColor(c.Theme().TextMuted).Margin(9, 0, 0, 0)
	})
}
func (a *App) nodeView(c *ui.Context, t *tab, n *node) {
	if n.pane == nil {
		a.paneSplit(c.Key(n.id), n, func() { a.nodeView(c, t, n.first) }, func() { a.nodeView(c, t, n.second) })
		return
	}
	p := n.pane
	if (p.profile.Kind == "sftp" && p.conn != nil && !p.terminalOnly) || (p.profile.Kind == "ftp" && p.ftp != nil) {
		a.fileBrowser(c, p, true)
		return
	}
	scheme := a.config.Settings.Scheme()
	paneBackground, paneForeground := ui.Hex(scheme.Background), ui.Hex(scheme.Foreground)
	ui.Column(c.Key(p.id)).Fill().MinHeight(0).Background(paneBackground).DrawOver(func(painter *ui.Painter, r ui.Rect) {
		if a.showPaneFocusStripe(t, p) {
			painter.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W, H: 2}, terminalFlame, 0)
		}
	}).Children(func() {
		// A single terminal needs only its tab. Pane headers appear after splitting.
		if t.root.pane == nil && !a.focusFullscreen.deep {
			ui.Row(c).Height(22).Padding(0, 8).Gap(5).Background(paneBackground).TextColor(paneForeground.Alpha(.75)).Children(func() {
				terminalHeaderButton(c, "选择分屏 "+p.profile.Name, nil, a.paneDisplayTitle(p), paneForeground).Tooltip(paneConnectionDetails(p)).Height(22).Grow(1).OnClick(func() { t.active = p; p.focus.Focus() })
				label := "放大分屏"
				if t.zoomed == p {
					label = "恢复分屏"
				}
				terminalHeaderButton(c, label, splitIcon, "", paneForeground).Size(22, 22).OnClick(func() { t.active = p; a.togglePaneZoom() })
				terminalHeaderButton(c, "关闭分屏", closeIcon, "", paneForeground).Size(22, 22).OnClick(func() { a.requestSessionClose(t, p) })
			})
		}
		if p.term != nil {
			a.terminalSearchBar(c.Key("terminal-search"), t, p)
			if strings.HasPrefix(p.status, "已断开") {
				ui.Row(c).Height(27).Padding(3, 10).Gap(8).Background(c.Theme().Surface).Children(func() {
					ui.Text(c, p.status).SingleLine().Grow(1).TextColor(c.Theme().TextMuted)
					toolButton(c, "重新连接", linkIcon, "重新连接").OnClick(func() { t.active = p; a.selectTab(t); a.reconnectActive() })
				})
			}
			el := terminal.View(c.Key("terminal-view"), p.term).Bind(&p.focus).Grow(1).MinHeight(0)
			a.clockOverlay(el)
			if el.FocusWithin() {
				t.active = p
				a.setCurrentTab(t, false)
			}
		} else {
			ui.Column(c).Grow(1).FillWidth().Center().Padding(24).Gap(14).Background(c.Theme().Background).Children(func() {
				ui.Icon(c, terminalIcon).FontSize(28).TextColor(c.Theme().TextMuted)
				ui.Text(c, p.profile.Name).FontSize(17)
				ui.Text(c, p.status).TextColor(c.Theme().TextMuted).MaxWidth(650)
				if strings.Contains(p.status, "失败") {
					toolButton(c, "重新连接", linkIcon, "重新连接").OnClick(func() { t.active = p; a.selectTab(t); a.reconnectActive() })
				}
			})
		}
	})
}
func (a *App) sendBar(c *ui.Context) {
	candidates := a.sendCandidates()
	count := len(readyPanes(candidates))
	scope := a.scopeName()
	ui.Row(c).Height(35).Padding(5, 8).Gap(5).BorderWidth(1, 0, 0, 0).BorderColor(c.Theme().Border).Children(func() {
		ui.MenuButton(c, fmt.Sprintf("%s · %d", scope, count), func(m *ui.Menu) {
			for _, choice := range sendScopes {
				if m.Item(choice[1]).Checked(a.sendScope() == choice[0]).Chosen() {
					a.setSendScope(choice[0])
				}
			}
		}).Width(166).Height(25).Label("快速发送范围")
		input := ui.TextInput(c, &a.quick).Grow(1).MinWidth(0).Height(25).Label("快捷命令").Bind(&a.features.operators.quickFocus).Placeholder("输入命令，回车发送；右侧选择历史命令")
		input.HandleInput(func(ev ui.InputEvent) bool {
			if ev.Kind == ui.InputKeyDown && !input.Composing() && ev.Mods == 0 && (ev.Key == ui.KeyUp || ev.Key == ui.KeyDown) {
				a.navigateQuickHistory(ev.Key == ui.KeyUp)
				return true
			}
			return false
		})
		if input.Submitted() {
			a.prepareSend()
		}
		flatButton(c, "命令历史与快速命令", chevronIcon, "").Menu(func(m *ui.Menu) {
			a.quickCommandsMenu(m)
			m.Separator()
			if m.Item("管理快速命令…").Chosen() {
				a.openCommands()
			}
			m.Separator()
			if len(a.history) == 0 {
				m.Item("暂无历史命令").Disabled(true)
			}
			for _, s := range a.history {
				label := strings.ReplaceAll(s, "\n", " ")
				if len([]rune(label)) > 60 {
					label = string([]rune(label)[:60]) + "…"
				}
				if m.Item(label).Chosen() {
					a.quick = s
					a.features.operators.noReturn = false
				}
			}
		})
		appendReturn := !a.features.operators.noReturn
		if ui.Checkbox(c, &appendReturn, "追加回车").Changed() {
			a.features.operators.noReturn = !appendReturn
		}
		ui.Button(c, "撰写").Height(25).OnClick(func() { a.openComposer(a.quick, !a.features.operators.noReturn) })
		ui.Button(c, "发送").Height(25).Padding(3, 12).Disabled(count == 0 || strings.TrimSpace(a.quick) == "").OnClick(a.prepareSend)
	})
}
func (a *App) statusView(c *ui.Context) {
	// Errors remain visible even when the quick-send bar is hidden.
	if strings.Contains(a.status, "失败") || strings.HasPrefix(a.status, "错误：") {
		ui.Text(c, a.status).FontSize(11).TextColor(c.Theme().Danger).Padding(4, 10)
	}
}

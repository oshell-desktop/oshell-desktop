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
	c.SetTheme(professionalTheme(interfaceDark(settings)))
}

func interfaceDark(settings core.Settings) bool {
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
	return dark
}
func (a *App) view(c *ui.Context) {
	defer a.resumeDeferredUpdate() // Modal teardown may happen after drain during this frame.
	a.drain()
	if a.hub != nil && a.hub.fileWindow != nil && a.hub.fileWindow.filePresentation.owner.Load() == a {
		a.hub.fileWindow.drain()
	}
	if !c.Valid() {
		return
	}
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
	width, _ := c.Size()
	if a.mcp.open && a.mcp.window == nil && width < 1200 && !a.mcp.detaching && a.win != nil && mygo.App.IsReady() {
		// Defer native window creation until this layout pass has finished.
		a.mcp.detaching = true
		a.post(func() {
			a.mcp.detaching = false
			if a.mcp.open && !a.workspaceLocked() {
				a.mcpWindow()
			}
		})
	}
	a.mcp.sidebarShown = a.mcp.open && a.mcp.window == nil && width >= 1200 && !a.focusFullscreen.chrome
	ui.Column(c).Fill().Children(func() {
		if !a.focusFullscreen.chrome {
			a.toolbar(c)
			a.masterWarning(c)
			if a.config.Settings.LinksVisible {
				a.linksBar(c)
			}
		}
		ui.Row(c.Key("workspace-content")).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			if a.sidebar.shown && !a.focusFullscreen.chrome {
				a.sidebarView(c.Key("session-sidebar"))
			}
			a.fileDockWorkspace(c)
			if t := a.activeTab(); a.filesVisible && t != nil && t.active.term != nil {
				a.filesView(c, t.active)
			}
			if a.mcp.sidebarShown {
				ui.Column(c.Key("mcp-sidebar")).Width(480).Shrink(0).MinHeight(0).Padding(10).Gap(8).Border(1, c.Theme().Border).Children(func() { a.mcpContents(c) })
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
	tree := quickConnectionTree(a.presentationConfig().Profiles, a.localUI.Pins)
	if len(tree) == 0 {
		m.Item("暂无快捷会话").Disabled(true)
	} else {
		a.populateQuickConnections(m, tree, interfaceDark(a.config.Settings))
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
						if a.linkMenuOpen && a.linkMenuAnchor == item.Key() {
							element.Background(c.Theme().Selection)
						}
						a.expandLinkOnHover(c, element, a.linkMenuOpen && a.linkMenuAnchor == item.Key(), func() { a.openLinkMenu(item.Key(), item.Folder) })
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
		if a.linkMenuOpen && a.linkMenuAnchor == "all" {
			all.Background(c.Theme().Selection)
		}
		a.expandLinkOnHover(c, all, a.linkMenuOpen && a.linkMenuAnchor == "all", func() { a.openLinkMenu("all", "") })
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
		// Leave room for the numbered bold title, status dot and close target.
		widths[i] = min(280, max(144, preferredTabWidth(fmt.Sprintf("%d  %s", i+1, t.name))+12))
		totalWidth += widths[i] + 4
	}
	if totalWidth > 0 {
		totalWidth -= 4
	}
	ui.Row(c).ContextMenu(backgroundMenu).Height(detailBarHeight).Padding(2, 6).Gap(6).Background(c.Theme().Surface).TextColor(c.Theme().Text).BorderWidth(0, 0, 1, 0).BorderColor(c.Theme().Border).Children(func() {
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
		scroll := ui.ScrollHorizontal(c).HideScrollbars().TrackScroll(&g.scroll).Width(totalWidth).Shrink(1).MinWidth(0).Height(30).ContextMenu(backgroundMenu).Children(func() {
			ui.Row(c).Gap(4).Children(func() {
				for i, t := range tabs {
					row := ui.Row(c.Key(t.id)).Height(28).Gap(0).Padding(0, 4).Width(widths[i]).Shrink(0).Radius(2).Background(ui.Transparent).BorderColor(ui.Transparent)
					if row.Hovered() {
						row.Background(c.Theme().SurfaceHover)
					}
					if t.id == g.active {
						if len(tabs) > 1 && a.groups.active == g.id {
							g.revealSelectedTab(row, t.id)
						}
						accent := c.Theme().Accent
						row.Background(c.Theme().Selection).DrawOver(func(p *ui.Painter, r ui.Rect) {
							p.Fill(ui.Rect{X: r.X + 7, Y: r.Y + r.H - 2, W: r.W - 14, H: 2}, accent, 1)
						})
					}
					if drag, over := ui.DragOver[tabDrag](row); over && a.acceptsTabDrag(drag, g) {
						g.dragOver = true
						x, _, _ := row.PointerPosition()
						if x < row.Bounds().W/2 {
							row.BorderColor(c.Theme().Accent).BorderWidth(0, 0, 0, 2)
						} else {
							row.BorderColor(c.Theme().Accent).BorderWidth(0, 2, 0, 0)
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
		if g.id == a.groups.active && !a.focusFullscreen.chrome {
			a.workspaceMenus(c)
		}
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
		ui.Image(c, brandImage()).Size(48, 48).Label("OShell Desktop 图标")
		windowHeading(c, "OShell Desktop")
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
	body := ui.Column(c.Key(p.id)).Fill().MinHeight(0).Background(paneBackground)
	if a.groups.revealPane == p.id {
		body.ScrollIntoView()
		a.groups.revealPane = ""
	}
	body.DrawOver(func(painter *ui.Painter, r ui.Rect) {
		if a.showPaneFocusStripe(t, p) {
			painter.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W, H: 2}, terminalFlame, 0)
		}
	}).Children(func() {
		// A single terminal needs only its tab. Pane headers appear after splitting.
		if t.root.pane == nil && !a.focusFullscreen.deep {
			ui.Row(c).Height(22).Padding(0, 8).Gap(5).Background(paneBackground).TextColor(paneForeground.Alpha(.75)).Children(func() {
				terminalHeaderButton(c, "选择分屏 "+p.profile.Name, nil, a.paneDisplayTitle(p), paneForeground).Tooltip(paneConnectionDetails(p)).Height(22).Grow(1).OnClick(func() { a.activatePane(t, p) })
				label := "放大分屏"
				if t.zoomed == p {
					label = "恢复分屏"
				}
				terminalHeaderButton(c, label, splitIcon, "", paneForeground).Size(22, 22).OnClick(func() { a.activatePane(t, p); a.togglePaneZoom() })
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
			if paths := el.DroppedFiles(); len(paths) > 0 {
				a.uploadTerminalDrop(p, paths)
			}
			if el.FocusWithin() {
				t.active = p
				a.setCurrentTab(t, false)
			}
		} else {
			ui.Column(c).Grow(1).FillWidth().Center().Padding(12).Gap(8).Background(c.Theme().Background).Children(func() {
				ui.Icon(c, terminalIcon).FontSize(28).TextColor(c.Theme().TextMuted)
				windowHeading(c, p.profile.Name)
				ui.Text(c, p.status).TextColor(c.Theme().TextMuted).MaxWidth(650)
				if strings.Contains(p.status, "失败") {
					toolButton(c, "重新连接", linkIcon, "重新连接").OnClick(func() { t.active = p; a.selectTab(t); a.reconnectActive() })
				}
				a.hostKeyCleanupButton(c, p)
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
		composing := input.Composing()
		input.HandleInput(func(ev ui.InputEvent) bool {
			if ev.Kind != ui.InputKeyDown || composing {
				return false
			}
			if commandEnter(ev.Mods, ev.Key) {
				a.prepareSend()
				return true
			}
			if ev.Mods == 0 && (ev.Key == ui.KeyUp || ev.Key == ui.KeyDown) {
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

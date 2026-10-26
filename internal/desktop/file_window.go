package desktop

import (
	"context"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

// The toolbar entry opens or focuses the manager even without an SSH terminal.
// Repeated clicks must not create another connection or duplicate file tab.
func (a *App) showFileManager() {
	if a.hub == nil {
		a.showFileSessionPicker()
		return
	}
	if p := a.activePane(); p != nil && !p.closed && p.conn != nil && p.conn.Alive() {
		a.hub.openFilesFromPane(p)
		return
	}
	if existing := a.hub.fileWindow; existing != nil {
		if existing.win != nil {
			existing.win.Focus()
		}
		return
	}
	target := a.hub.externalFileWindow()
	target.win.Focus()
}

// External file launches share a dedicated window. They must never consume a
// terminal workspace tab or silently replace its active SSH session.
func (h *applicationHub) externalFileWindow() *App {
	if h.fileWindow != nil {
		return h.fileWindow
	}
	a := newApp(h.store, h.config.Clone())
	a.hub = h
	a.vault = h.vault
	a.trust = h.trust
	a.win = mygo.NewWindow(mygo.WindowOptions{Title: "文件管理", Width: 1080, Height: 720, MinWidth: 760, MinHeight: 480, Content: ui.View(a.fileWindowView)})
	h.fileWindow = a
	a.win.OnClose(a.onWorkspaceClose)
	a.win.OnClosed(func() {
		a.cancelSessionPassword()
		a.stopped.Store(true)
		if a.features.sharedSync.cancel != nil {
			a.features.sharedSync.cancel()
		}
		a.clearSharedSyncPlan()
		for _, tab := range a.tabs {
			a.walk(tab.root, a.dispose)
		}
		a.drain()
		if h.fileWindow == a {
			h.fileWindow = nil
		}
		h.scheduleSharedSync(time.Minute)
	})
	h.scheduleSharedSync(2 * time.Second)
	return a
}
func (a *App) fileWindowView(c *ui.Context) {
	defer a.resumeDeferredUpdate() // Modal teardown may happen after drain during this frame.
	a.drain()
	if !a.modalActive() && c.Shortcut(ui.Ctrl, ui.KeyW) && a.win != nil {
		a.win.Close()
		return
	}
	if !a.modalActive() && auxiliaryEscape(c, func() {
		if a.win != nil {
			a.win.Minimize()
		}
	}) {
		return
	}
	a.theme(c)
	if a.lockedWorkspace(c) {
		return
	}
	ui.Column(c).Fill().MinHeight(0).Children(func() {
		ui.Row(c).Padding(8).Gap(8).Children(func() {
			ui.MenuButton(c, "新会话…", func(m *ui.Menu) {
				if m.Item("新建 SFTP 会话…").Chosen() {
					a.edit(a.newProfile("sftp"))
				}
				if m.Item("新建 FTP 会话…").Chosen() {
					a.edit(a.newProfile("ftp"))
				}
				m.Separator()
				if m.Item("选择已保存会话…").Chosen() {
					a.showFileSessionPicker()
				}

			})
			ui.Button(c, "复制文件标签").Disabled(a.activePane() == nil).OnClick(a.duplicateFileSession)
			ui.Button(c, "重连").Disabled(a.activePane() == nil).OnClick(a.reconnectActive)
			ui.Spacer(c)
			toolButton(c, "设置", settingsIcon, "设置").OnClick(a.openSettings)
		})
		a.ensureGroups()
		g := a.groupByID(a.groups.active)
		if g == nil {
			g = a.groups.root.leaves()[0]
		}
		g.dragOver = false
		scroll := ui.ScrollHorizontal(c).TrackScroll(&g.scroll).Height(38).BorderWidth(0, 0, 1, 0).BorderColor(c.Theme().Border).Children(func() {
			ui.Row(c).Gap(4).Children(func() {
				for _, tab := range a.tabs {
					selected := tab == a.activeTab()
					title := tab.name
					if tab.active != nil && tab.active.profile.Host != "" {
						title = tab.active.profile.Host
						if tab.active.fileDisplayIP != "" {
							title = tab.active.fileDisplayIP
						}
					}
					background, underline := c.Theme().Surface, ui.Transparent
					if selected {
						background, underline = c.Theme().Selection, c.Theme().Accent
					}
					row := ui.Row(c.Key(tab.id)).Height(37).Gap(2).Padding(2, 3).Background(background).BorderWidth(0, 0, 3, 0).BorderColor(underline).Label("文件标签 " + tab.id)
					if _, over := ui.DragOver[tabDrag](row); over {
						g.dragOver = true
						x, _, _ := row.PointerPosition()
						if x < row.Bounds().W/2 {
							row.BorderWidth(0, 0, 3, 2)
						} else {
							row.BorderWidth(0, 2, 3, 0)
						}
						row.BorderColor(c.Theme().Accent)
					}
					if drag, dropped := ui.Drop[tabDrag](row); dropped {
						x, _, _ := row.PointerPosition()
						after := x >= row.Bounds().W/2
						target := tab.id
						a.post(func() { a.moveTabRelative(drag.ID, target, after) })
					}
					row.Children(func() {
						button := ui.Button(c, title).Role(ui.RoleTab).Selected(selected).Background(ui.Transparent).TextColor(c.Theme().Text).BorderWidth(0).Tooltip(fileTabDescription(tab) + "\n双击已连接的会话可新建相同文件标签").Drag(tabDrag{tab.id})
						if selected {
							button.FontWeight(600)
						}
						a.tabTitleEvents(button, tab)
						ui.Button(c, "×").Background(ui.Transparent).BorderWidth(0).Label("关闭文件会话 " + title).OnClick(func() { a.requestSessionClose(tab, nil) })
					})
				}
			})
		})
		a.tabDragScroll(c, scroll, g)
		if p := a.activePane(); p != nil {
			current := "当前会话：" + fileTabDescription(a.activeTab())
			ui.Text(c, current).SingleLine().Tooltip(current).FontWeight(600).Padding(6, 10).FillWidth().MinWidth(0)
			if (p.conn != nil && !p.conn.Alive()) || strings.HasPrefix(p.status, "连接失败") || strings.HasPrefix(p.status, "FTP 连接失败") {
				ui.Row(c).Padding(8).Gap(8).Children(func() {
					ui.Text(c, "文件连接已断开").Grow(1)
					ui.Button(c, "重新连接").OnClick(a.reconnectActive)
				})
			}
			if p.files != nil {
				a.fileBrowser(c, p, true)
			} else {
				ui.Text(c, p.status).Padding(20)
			}
		} else {
			ui.Text(c, "从会话管理或外部文件启动入口打开 FTP / SFTP 连接。").Padding(24)
		}
		if strings.Contains(a.status, "密码") || strings.Contains(a.status, "SSH 连接") {
			ui.Text(c, a.status).FontSize(11).TextColor(c.Theme().TextMuted).Padding(8)
		}
	})
	a.transferOverlays(c)
	a.dialogs(c)
}

func fileTabDescription(t *tab) string {
	if t == nil {
		return ""
	}
	if t.active == nil {
		return t.name
	}
	p := t.active.profile
	kind := strings.ToUpper(p.Kind)
	if p.Kind == "ssh" {
		kind = "SFTP"
	}
	address := p.Host
	if p.Port > 0 {
		address = net.JoinHostPort(p.Host, strconv.Itoa(p.Port))
	}
	if p.Username != "" {
		address = p.Username + "@" + address
	}
	name := p.Name
	if name == "" {
		name = t.name
	}
	return name + " · " + kind + " · " + address
}
func (h *applicationHub) openExternalFile(profile core.Profile, credentials core.Credentials) {
	a := h.externalFileWindow()
	a.open(profile, credentials, 0)
	a.win.Focus()
}

func (h *applicationHub) openFilesFromPane(source *pane) {
	if source == nil || source.closed || source.conn == nil || !source.conn.Alive() {
		return
	}
	target := h.externalFileWindow()
	defer func() {
		if target.win != nil {
			target.win.Invalidate()
			target.win.Focus()
		}
	}()
	// Match the originating connection, not profile ID or host: separate SSH
	// sessions can use the same saved profile. The association exists before
	// the asynchronous SFTP setup so a quick second click reuses its pending tab.
	for _, tab := range target.tabs {
		p := tab.active
		if p != nil && !p.closed && p.fileSource == source.conn &&
			(p.status == "正在连接…" || p.conn != nil && p.conn.Alive()) {
			target.selectTab(tab)
			return
		}
	}
	profile := source.profile
	profile.Kind = "sftp"
	if source.files != nil && source.files.dir != "" {
		profile.InitialDirectory = source.files.dir
	}
	target.openConnection(profile, core.Credentials{ExternalLaunch: source.externalLaunch}, 0, source.conn)
}

func observeFileConnection(ctx context.Context, p *pane, conn *core.Connection) {
	go func() {
		if conn.WaitClosed(ctx) != nil {
			return
		}
		postPane(p, func(a *App) {
			if p.closed || p.conn != conn {
				return
			}
			p.status = "文件连接已断开"
			if p.files != nil {
				p.files.status = "文件连接已断开"
			}
		})
	}()
}

// Keep the saved profile identity for credential lookup while opening only SFTP.
func (a *App) opensAsFileSession(profile core.Profile) bool {
	return profile.Kind == "sftp" || profile.Kind == "ssh" && a.hub != nil && a.hub.fileWindow == a
}

func (a *App) reopenSSHFileSession(p *pane, replace bool) {
	if p == nil || p.closed {
		return
	}
	if p.conn == nil || !p.conn.Alive() {
		if !replace {
			a.status = "原 SSH 连接已失效，无法复用。请先重新连接。"
			return
		}
		profile := p.profile
		if p.files != nil {
			profile.InitialDirectory = p.files.dir
		}
		split := 0
		if replace {
			a.features.reconnectTarget = p
			split = -1
		}
		a.request(profile, split)
		return
	}
	profile := p.profile
	if p.files != nil && p.files.dir != "" {
		profile.InitialDirectory = p.files.dir
	}
	split := 0
	if replace {
		split = -1
		a.features.reconnectTarget = p
	}
	a.openConnection(profile, core.Credentials{ExternalLaunch: p.externalLaunch}, split, p.conn)
	if next := a.activePane(); next != nil && next != p && p.fileSource != nil {
		// Reconnecting or explicitly duplicating the file tab keeps its original
		// SSH association; p.conn is only the intermediate file-channel wrapper.
		next.fileSource = p.fileSource
	}
}
func (a *App) duplicateFileSession() {
	p := a.activePane()
	if p == nil || p.closed {
		return
	}
	if a.opensAsFileSession(p.profile) {
		a.reopenSSHFileSession(p, false)
		return
	}
	if p.profile.Kind == "ftp" && p.ftp != nil {
		profile := p.profile
		if p.files != nil && p.files.dir != "" {
			profile.InitialDirectory = p.files.dir
		}
		a.openConnection(profile, core.Credentials{ExternalLaunch: p.externalLaunch}, 0, nil, p.ftp)
	}
}

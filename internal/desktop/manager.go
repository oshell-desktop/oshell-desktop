// SPDX-License-Identifier: GPL-3.0-only
package desktop

import (
	"fmt"
	"path"
	"runtime"
	"sort"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"
	"oshell-desktop/internal/core"
)

func (a *App) openManager() {
	if a.win == nil {
		return
	}
	if a.manager != nil {
		a.manager.Show()
		a.manager.Focus()
		return
	}
	a.refreshManagerSearchSelection()
	a.managerParentClick = false
	w := mygo.NewWindow(mygo.WindowOptions{Parent: a.win, Title: "会话管理", Width: 900, Height: 580, MinWidth: 740, MinHeight: 440, Content: ui.View(a.managerView)})
	a.manager = w
	w.OnClosed(func() { a.managerClosed(w) })
}
func (a *App) managerClosed(w *mygo.Window) {
	if a.manager == w {
		a.manager = nil
		a.managerRename = nil
	}
	// Closing for an editor must not start sync in the gap before edit()
	// sets its modal flag. Native close is deferred until that action ends.
	a.resumeDeferredSyncAfterSettings()
}
func (a *App) closeManager() {
	a.managerRename = nil
	if a.manager != nil {
		w := a.manager
		a.manager = nil
		// Close can be requested while this window is still building a frame.
		// A worker marshals teardown onto a later UI event-loop dispatch, so
		// table cells never render against a destroyed context.
		go w.Close()
	}
	if a.win != nil {
		a.win.Invalidate()
	}
}

type sessionRow struct {
	id, name, group string
	linkID          string
	folder          bool
	parent          bool
	profile         core.Profile
}

type sessionSortKey struct {
	name           string
	folder, pinned bool
}

func sortSessionList[T any](items []T, key func(T) sessionSortKey) {
	if len(items) < 2 {
		return
	}
	order := collate.New(language.Und, collate.Numeric)
	sort.SliceStable(items, func(i, j int) bool {
		left, right := key(items[i]), key(items[j])
		if left.pinned != right.pinned {
			return left.pinned
		}
		if left.folder != right.folder {
			return left.folder
		}
		return order.CompareString(left.name, right.name) < 0
	})
}

func (a *App) buildManagerRows() []sessionRow {
	cfg := a.presentationConfig()
	var folders, profiles []sessionRow
	seen := map[string]bool{}
	query := core.NewSessionQuery(a.query)
	if !query.Valid {
		return nil
	}
	terms := query.Terms
	inScope := func(group string) bool {
		return a.managerSearchScope == "all" || a.managerGroup == "" || group == a.managerGroup || strings.HasPrefix(group, a.managerGroup+"/")
	}
	if len(terms) > 0 {
		for _, d := range cfg.AllDirectories() {
			if d != a.managerGroup && inScope(d) && query.Matches(core.NormalizeSessionSearch("/"+d)) {
				folders = append(folders, sessionRow{id: "folder:" + d, name: path.Base(d), group: d, folder: true})
			}
		}
	}
	if len(terms) == 0 && !a.managerAll {
		prefix := ""
		if a.managerGroup != "" {
			prefix = a.managerGroup + "/"
		}
		for _, d := range cfg.AllDirectories() {
			if d == a.managerGroup || !strings.HasPrefix(d, prefix) {
				continue
			}
			name := strings.SplitN(strings.TrimPrefix(d, prefix), "/", 2)[0]
			full := prefix + name
			if !seen[full] {
				seen[full] = true
				folders = append(folders, sessionRow{id: "folder:" + full, name: name, group: full, folder: true})
			}
		}
	}
	add := func(r sessionRow) {
		if a.managerProtocol != "" && r.profile.Kind != a.managerProtocol {
			return
		}
		if len(terms) > 0 {
			if !inScope(r.group) {
				return
			}
			p := r.profile
			p.Name = r.name
			p.Group = r.group
			if !query.Matches(core.SessionSearchMetadata(p)) {
				return
			}

		} else if !a.managerAll && r.group != a.managerGroup {
			return
		}
		profiles = append(profiles, r)
	}
	sources := map[string]core.Profile{}
	for _, p := range cfg.Profiles {
		sources[p.ID] = p
		add(sessionRow{id: p.ID, name: p.Name, group: p.Group, profile: p})
	}
	for _, l := range cfg.Links {
		if p, ok := sources[l.ProfileID]; ok {
			g := "Links"
			if l.Folder != "" {
				g += "/" + l.Folder
			}
			add(sessionRow{id: "link:" + l.ID, linkID: l.ID, name: l.Name, group: g, profile: p})
		}
	}
	result := append(folders, profiles...)
	sortSessionList(result, func(r sessionRow) sessionSortKey {
		return sessionSortKey{name: r.name, folder: r.folder, pinned: a.rowPinned(r)}
	})
	if !a.managerAll && a.managerGroup != "" {
		parent := path.Dir(a.managerGroup)
		if parent == "." {
			parent = ""
		}
		result = append([]sessionRow{{id: "parent:" + a.managerGroup, name: "../", group: parent, folder: true, parent: true}}, result...)
	}
	return result
}
func (a *App) enterRow(row sessionRow) {
	if row.folder {
		if a.sidebar.context != nil {
			a.revealSidebar(row.id)
			return
		}
		a.navigateManager(row.group)
		return
	}
	a.selected = row.profile.ID
	a.request(row.profile, 0)
}
func (a *App) managerView(c *ui.Context) {
	defer a.resumeDeferredUpdate() // Modal teardown may happen after drain during this frame.
	a.drain()
	if !c.Valid() {
		return
	}
	if a.managerRename != nil && c.Shortcut(0, ui.KeyEscape) {
		a.managerRename = nil
		return
	}
	if a.managerRename == nil && auxiliaryEscape(c, a.closeManager) {
		return
	}
	a.theme(c)
	if a.managerProtocol != a.managerLastProtocol {
		a.refreshManagerSearchSelection()
		a.managerLastProtocol = a.managerProtocol
	}
	if a.lockedWorkspace(c) {
		return
	}
	// Ctrl+Enter belongs to command input in the workspace, so route this
	// manager binding here instead of installing a process-wide accelerator.
	if a.managerRename == nil {
		for _, binding := range a.config.Settings.KeyboardShortcuts.Bindings("connectSelected", runtime.GOOS) {
			mods, key := shortcutMods(binding), shortcutKey(binding.KeyCode)
			if commandEnter(mods, key) && c.Shortcut(mods, key) {
				a.performAction("connectSelected")
				return
			}
		}
	}
	focusResults := false
	ui.Column(c).Fill().Padding(6).Gap(4).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Toolbar(c, func() {
				toolButton(c, "新建会话", plusIcon, "新建").Disabled(a.configurationBusy()).OnClick(func() {
					kind := a.managerProtocol
					if kind != "ssh" && kind != "sftp" && kind != "ftp" {
						kind = "ssh"
					}
					p := a.newProfile(kind)
					if a.managerGroup != "" {
						p.Group = a.managerGroup
					}
					a.editManagerProfile(p)
				})
				toolButton(c, "新建目录", folderIcon, "新建目录").Disabled(a.configurationBusy()).OnClick(func() { a.openDirectory("新建目录", a.managerGroup) })
			}).Shrink(0).Padding(0)
			search := ui.TextInput(c, &a.query).Label("会话搜索").Placeholder("搜索名称、主机、用户名或端口").Grow(1).MinWidth(0).Height(28)
			for _, key := range a.config.Settings.KeyboardShortcuts.Bindings("find", runtime.GOOS) {
				if c.Shortcut(shortcutMods(key), shortcutKey(key.KeyCode)) {
					search.Focus()
				}
			}
			if search.Changed() {
				a.refreshManagerSearchSelection()
			}
			selectValue(c, "协议分类", &a.managerProtocol, [][2]string{{"", "全部协议"}, {"ssh", "SSH"}, {"sftp", "SFTP"}, {"ftp", "FTP"}, {"local", "本地"}})
			scope := a.managerSearchScope
			ui.Row(c).Disabled(len(core.NewSessionQuery(a.query).Terms) == 0).Children(func() {
				selectValue(c, "会话搜索范围", &a.managerSearchScope, [][2]string{{"", "当前目录及子目录"}, {"all", "全部目录"}})
			})
			if scope != a.managerSearchScope {
				a.refreshManagerSearchSelection()
			}
			if search.Shortcut(0, ui.KeyDown) {
				a.refreshManagerSearchSelection()
				if a.managerRow < 0 {
					for i, row := range a.managerRows() {
						if !row.parent {
							a.managerRow = i
							a.managerSelection.Add(row.id)
							break
						}
					}
				}
				focusResults = true
			}
			if search.Submitted() {
				a.refreshManagerSearchSelection()
				a.connectManagerSelection()
			}
		})
		ui.Row(c).Height(26).AlignItems(ui.Center).Padding(0, 3).Gap(4).
			Background(panelColor(c)).Border(1, c.Theme().Border).Children(func() {
			root := flatButton(c, "所有会话", folderIcon, "所有会话").OnClick(func() { a.navigateManager("") })
			a.managerDrop(root, "")
			location, destination := "", a.managerGroup
			if a.managerAll {
				location = "/ 全部目录"
				destination = ""
			} else if a.managerGroup != "" {
				location = "/ " + a.managerGroup
			}
			current := ui.Row(c).Grow(1).MinWidth(0).Height(24).AlignItems(ui.Center).Children(func() {
				ui.Text(c, location).MinWidth(0).SingleLine().Tooltip(location)
			})
			a.managerDrop(current, destination)
			parent := managerParent(destination)
			up := flatButton(c, "上一级", arrowUpIcon, "").Disabled(destination == "" && !a.managerAll).
				OnClick(func() { a.navigateManager(parent) })
			a.managerDrop(up, parent)
		})
		query := core.NewSessionQuery(a.query)
		rows := a.managerRows()
		var chosen []sessionRow
		for _, r := range rows {
			if a.managerSelection.Has(r.id) {
				chosen = append(chosen, r)
			}
		}
		if len(chosen) == 0 && a.managerRow >= 0 && a.managerRow < len(rows) {
			chosen = append(chosen, rows[a.managerRow])
		}
		a.managerList.Selected = &a.managerRow
		a.managerList.Selection = &a.managerSelection
		a.managerList.Key = func(i int) any { return rows[i].id }
		a.managerList.Label = func(i int) string { return rows[i].name }

		columns := []ui.TableColumn{
			{Title: "名称", MinWidth: 170}, {Title: "主机", Width: 200, MinWidth: 140},
			{Title: "用户名", Width: 112, MinWidth: 80}, {Title: "协议", Width: 70, MinWidth: 56},
			{Title: "端口", Width: 64, MinWidth: 56}, {Title: "操作", ID: "actions", Width: 44, Align: ui.Center, Fixed: true},
		}
		table := ui.Table(c, a.managerList, columns, len(rows), func(row, col int) {
			r := rows[row]
			p := r.profile
			if col == 5 {
				if !r.parent && !(r.folder && r.group == "Links") {
					rowIconButton(c, "编辑 "+r.name, editIcon, c.Theme().TextMuted).Size(24, 20).Disabled(a.configurationBusy()).OnClick(func() {
						if r.folder {
							a.openDirectory("移动 / 重命名目录", r.group)
						} else if r.linkID != "" {
							a.editManagerLink(r.linkID)
						} else {
							a.editManagerProfile(r.profile)
						}
					})
				}
				return
			}
			if col == 0 {
				if a.managerNameEditor(c, r) {
					return
				}
				style := sessionIconFor(p.Kind, r.linkID != "" || r.folder && r.group == "Links", r.folder, r.parent)
				tip := sessionListTooltip(p, r.linkID != "")
				if r.folder {
					tip = style.label + "\n/" + r.group
				}
				cell := ui.Row(c).FillWidth().Height(22).AlignItems(ui.Center).Gap(4).Children(func() {
					style.draw(c)
					ui.RichText(c, managerSearchSpans(r.name, query)...).SingleLine().Tooltip(tip).TextColor(c.Theme().Text).Grow(1).MinWidth(0)
				})
				cell.HandleInput(func(ev ui.InputEvent) bool { return a.managerCellInput(r, ev) })
				cell.ContextMenu(func(m *ui.Menu) { a.managerContextMenu(c, m, r.id) })
				if !r.parent && !(r.folder && r.group == "Links") {
					moving := []sessionRow{r}
					for _, selected := range chosen {
						if selected.id == r.id {
							moving = chosen
							break
						}
					}
					cell.Drag(managerDrag{owner: a, rows: moving})
				}
				if r.folder {
					a.managerDrop(cell, r.group)
				}
				return
			}
			field := ui.Row(c).FillWidth().Height(22).AlignItems(ui.Center).HandleInput(func(ev ui.InputEvent) bool { return a.managerCellInput(r, ev) }).ContextMenu(func(m *ui.Menu) { a.managerContextMenu(c, m, r.id) })
			if !r.folder {
				field.Children(func() {
					var value string
					switch col {
					case 1:
						value = p.Host
						if p.Kind == "local" {
							value = "本地终端"
						}
					case 2:
						value = p.Username
					case 3:
						value = strings.ToUpper(p.Kind)
						if p.Kind == "local" {
							value = "本地"
						}
					case 4:
						value = fmt.Sprint(p.Port)
						if p.Kind == "local" {
							value = "—"
						}
					}
					ui.RichText(c, managerSearchSpans(value, query)...).SingleLine().Tooltip(value).TextColor(c.Theme().Text).MinWidth(0)
				})
			}
		}).Grow(1).MinHeight(0).Background(panelColor(c)).Border(1, c.Theme().Border).Label("会话列表")
		a.managerClipboardShortcuts(c, table)
		if a.managerRename == nil && table.Shortcut(0, ui.KeyF2) && len(chosen) == 1 {
			a.beginManagerRename(chosen[0])
		}
		if focusResults {
			a.managerList.Focus(c)
		}
		table.ContextMenu(func(m *ui.Menu) { a.managerContextMenu(c, m, "") })
		if a.managerRow >= len(rows) {
			a.managerRow = -1
			a.managerSelection.Clear()
		}
		valid := a.managerRow >= 0 && a.managerRow < len(rows)
		if valid {
			a.selected = ""
			if !rows[a.managerRow].folder {
				a.selected = rows[a.managerRow].profile.ID
			}
		}
		if table.Submitted() && valid && a.managerRename == nil {
			a.connectManagerSelection()
		}
		// Entering a folder clears managerRow in this frame. Re-evaluate before
		// building controls, and capture the row rather than a mutable index.
		valid = a.managerRow >= 0 && a.managerRow < len(rows)
		var current sessionRow
		if valid {
			current = rows[a.managerRow]
		}
		folders, profileCount := 0, 0
		for _, r := range rows {
			if r.parent {
				continue
			}
			if !r.folder {
				profileCount++
			}
			if r.folder {
				folders++
			}
		}
		if folders == 0 && profileCount == 0 {
			message := "当前目录没有可显示的会话或目录；右键可以新建会话或目录"
			if a.query != "" {
				message = "没有匹配的会话或目录；可调整关键词、协议或搜索范围"
			}
			ui.Text(c, message).FontSize(12).TextColor(c.Theme().TextMuted)
		}
		ui.Row(c).Gap(6).Children(func() {
			statusColor := c.Theme().TextMuted
			if !query.Valid {
				statusColor = c.Theme().Danger
			}
			ui.Text(c, a.managerResultStatus(profileCount, folders)).TextColor(statusColor).FontSize(11).Grow(1).Tooltip("⌘ 单击多选，⇧ 单击连续选择；回车连接；单击 ../ 返回上级；拖到目录或 ../ 迁移；Esc 关闭")
			ui.Button(c, "编辑").Disabled(len(a.selectedManagerRows()) != 1 || !valid || current.parent || current.folder && current.group == "Links" || !current.folder && current.linkID == "" && current.profile.Kind == "local").OnClick(func() {
				if current.folder {
					a.openDirectory("移动 / 重命名目录", current.group)
				} else if current.linkID != "" {
					a.editManagerLink(current.linkID)
				} else {
					a.editManagerProfile(current.profile)
				}
			})
			ui.Button(c, "删除").Disabled((!valid && a.managerSelection.Len() == 0) || !a.managerSelectionMutable()).OnClick(a.deleteManagerSelection)
			ui.MenuButton(c, "更多", func(m *ui.Menu) {
				if a.managerClipboardMenu(c, m) {
					return
				}
				if m.Item("复制所选会话").Disabled(!valid).Chosen() {
					a.duplicateManagerSelection()
				}
				if m.Item("移动所选会话 / 引用…").Disabled(!valid || !a.managerSelectionMutable()).Chosen() {
					a.openDirectory("移动所选会话", a.managerGroup)
				}
				if m.Item("移动 / 重命名目录…").Disabled(!valid || !current.folder || current.parent || current.group == "Links").Chosen() {
					a.openDirectory("移动 / 重命名目录", current.group)
				}
				if m.Item("添加到快捷链接").Disabled(!valid || current.folder).Chosen() {
					a.addProfileLinkToRoot(current.profile)
				}
				m.Separator()
				if m.Item("导入会话…").Chosen() {
					a.importProfiles()
				}
				if m.Item("导出会话…").Chosen() {
					a.exportManagerSelection()
				}
			})
			ui.Button(c, "关闭").OnClick(a.closeManager)
			count := a.managerConnectCount()
			label := "连接"
			if count == 0 && valid && current.folder && len(a.selectedManagerRows()) == 1 {
				label = "打开"
			}
			if count > 1 {
				label = fmt.Sprintf("连接（%d）", count)
			}
			ui.PrimaryButton(c, label).Disabled(count == 0 && label != "打开").OnClick(a.connectManagerSelection)
		})
		if a.configurationBusy() {
			ui.Text(c, "配置正在保存或同步，完成后可继续编辑、复制和粘贴。").TextColor(c.Theme().TextMuted).FontSize(11)
		} else if strings.HasPrefix(a.status, "错误") || strings.Contains(a.status, "失败") || strings.HasPrefix(a.status, "已复制") || strings.HasPrefix(a.status, "已粘贴") {
			color := c.Theme().TextMuted
			if strings.HasPrefix(a.status, "错误") || strings.Contains(a.status, "失败") {
				color = c.Theme().Danger
			}
			ui.Text(c, a.status).TextColor(color).FontSize(11)
		}
	})
}

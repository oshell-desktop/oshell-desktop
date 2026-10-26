package desktop

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"strings"
)

func (a *App) selectManagerContext(id string) (sessionRow, bool) {
	if context := a.sidebar.context; context != nil {
		return context.row, context.valid && context.row.id == id
	}
	for i, r := range a.managerRows() {
		if r.id != id {
			continue
		}
		if !a.managerSelection.Has(id) {
			a.managerSelection.Clear()
			a.managerSelection.Add(id)
		}
		a.managerRow = i
		a.selected = ""
		if !r.folder {
			a.selected = r.profile.ID
		}
		return r, true
	}
	a.managerSelection.Clear()
	a.managerRow = -1
	a.selected = ""
	return sessionRow{}, false
}
func (a *App) managerContextMenu(c *ui.Context, m *ui.Menu, id string) {
	r, valid := a.selectManagerContext(id)
	group := a.sessionActionGroup()
	if valid && r.parent {
		if m.Item("上一级").Chosen() {
			a.enterRow(r)
		}
		return
	}
	if a.managerClipboardMenu(c, m) {
		return
	}
	chosen := a.selectedManagerRows()
	if a.pinsMenu(m, chosen) {
		return
	}
	mutable := a.managerSelectionMutable()
	count := a.managerConnectCount()
	if len(chosen) > 1 {
		if m.Item(fmt.Sprintf("连接（%d）", count)).Disabled(count == 0).Chosen() {
			a.connectManagerSelection()
			return
		}
		if m.Item("移动到…").Disabled(!mutable).Chosen() {
			a.openDirectory("移动所选会话", group)
			return
		}
		actual := mutable
		for _, row := range chosen {
			if row.folder || row.linkID != "" {
				actual = false
			}
		}
		if m.Item("移动实际会话到…").Disabled(!actual).Chosen() {
			a.openDirectory("移动实际会话", group)
			return
		}
		if m.Item(fmt.Sprintf("删除所选（%d）…", len(chosen))).Disabled(!mutable || a.passwordLocked()).Chosen() {
			a.deleteManagerSelection()
		}
		return
	}
	if valid {
		if r.folder {
			if m.Item("打开目录").Chosen() {
				a.enterRow(r)
				return
			}
			if m.Item("重命名…").Disabled(r.group == "Links").Chosen() {
				a.beginManagerRename(r)
				return
			}
		} else {
			if m.Item("重命名…").Chosen() {
				a.beginManagerRename(r)
				return
			}
			if m.Item("连接").Chosen() {
				a.connectManagerSelection()
				return
			}
			if r.profile.Kind == "ssh" || r.profile.Kind == "sftp" {
				if m.Item("打开 SFTP 文件").Chosen() {
					a.openSavedFileSession(r.profile)
					return
				}
			}
			if r.profile.Kind == "sftp" && m.Item("打开 SSH 终端").Chosen() {
				a.request(r.profile, 0, true)
				return
			}
			if r.linkID != "" {
				if m.Item("链接属性…").Chosen() {
					a.editManagerLink(r.linkID)
					return
				}
				if m.Item("源会话属性…").Disabled(r.profile.Kind == "local").Chosen() {
					a.editManagerProfile(r.profile)
					return
				}
			} else {
				if m.Item("属性…").Disabled(r.profile.Kind == "local").Chosen() {
					a.editManagerProfile(r.profile)
					return
				}
				if m.Item("复制会话").Disabled(r.profile.Kind == "local").Chosen() {
					a.duplicateManagerSelection()
					return
				}
				if m.Item("添加到快捷链接…").Chosen() {
					a.addProfileLinkToRoot(r.profile)
					return
				}
				if m.Item("移动实际会话到…").Chosen() {
					a.openDirectory("移动实际会话", group)
					return
				}
			}
		}
		if m.Item("移动到…").Disabled(!mutable).Chosen() {
			a.openDirectory("移动所选会话", group)
			return
		}
		title := "删除…"
		if r.linkID != "" {
			title = "删除快捷引用"
		}
		if m.Item(title).Disabled(!mutable || a.passwordLocked()).Chosen() {
			a.deleteManagerSelection()
			return
		}
		m.Separator()
	}
	if group == "Links" || strings.HasPrefix(group, "Links/") {
		if m.Item("添加已有会话链接…").Disabled(len(a.config.Profiles) == 0).Chosen() {
			a.addExistingLink(strings.TrimPrefix(strings.TrimPrefix(group, "Links"), "/"))
			return
		}
	}
	for _, entry := range [][2]string{{"ssh", "新建 SSH 会话…"}, {"sftp", "新建 SFTP 会话…"}, {"ftp", "新建 FTP 会话…"}} {
		if m.Item(entry[1]).Chosen() {
			a.editManagerProfile(a.newProfile(entry[0]))
			return
		}
	}
	if m.Item("新建目录…").Chosen() {
		a.openDirectory("新建目录", group)
		return
	}
	if m.Item("会话默认属性…").Chosen() {
		a.showDefaults()
		return
	}
	m.Separator()
	if valid {
		title := "导出此会话…"
		if r.folder {
			title = "导出此目录…"
		} else if r.linkID != "" {
			title = "导出此快捷引用…"
		}
		if m.Item(title).Chosen() {
			a.exportManagerSelection()
			return
		}
	}
	if m.Item("导出全部会话…").Chosen() {
		a.exportProfiles()
		return
	}
	if m.Item("导入会话到当前目录…").Chosen() {
		a.importProfiles()
	}
}

func (a *App) openSavedFileSession(profile core.Profile) {
	if a.passwordLocked() {
		a.showVault()
		return
	}
	if profile.Kind != "ssh" && profile.Kind != "sftp" {
		return
	}
	if a.hub == nil {
		a.status = "文件窗口尚未就绪"
		return
	}
	a.closeManager()
	target := a.hub.externalFileWindow()
	target.request(profile, 0)
	if target.win != nil {
		target.win.Focus()
	}
}

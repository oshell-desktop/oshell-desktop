package desktop

import (
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"strings"
)

type managerReturnState struct {
	group, query, protocol string
	searchScope            string
	all                    bool
	sidebar                bool
	ids                    []string
	target                 string
}

func (a *App) captureManagerReturn() *managerReturnState {
	state := &managerReturnState{group: a.managerGroup, query: a.query, protocol: a.managerProtocol, all: a.managerAll, searchScope: a.managerSearchScope}
	state.sidebar = a.sidebar.context != nil
	for _, row := range a.selectedManagerRows() {
		state.ids = append(state.ids, row.id)
	}
	return state
}
func (a *App) editManagerProfile(p core.Profile) {
	if a.sessionEditorBlocked() {
		return
	}
	state := a.captureManagerReturn()
	state.target = p.ID
	for _, row := range a.selectedManagerRows() {
		if row.linkID != "" && row.profile.ID == p.ID {
			state.target = row.id
			break
		}
	}
	a.edit(p)
	a.features.managerReturn = state
}

func (a *App) sessionEditorBlocked() bool {
	if a.passwordLocked() {
		a.status = "错误：请先解锁再编辑会话"
		return true
	}
	if a.configurationBusy() || a.features.sessionPassword.busy {
		a.status = "错误：配置正在保存或同步，完成后可编辑会话"
		return true
	}
	return false
}
func (a *App) editManagerLink(id string) {
	state := a.captureManagerReturn()
	state.target = "link:" + id
	a.closeManager()
	a.editLink(id)
	a.features.managerReturn = state
}
func (a *App) restoreManagerReturn(saved *core.Profile) {
	state := a.features.managerReturn
	if state == nil || a.features.sessionPassword.busy {
		return
	}
	a.features.managerReturn = nil
	if state.sidebar {
		a.revealSidebar(state.target)
		return
	}
	a.managerGroup = state.group
	a.managerAll = state.all
	a.managerSearchScope = state.searchScope
	a.query = state.query
	a.managerProtocol = state.protocol
	a.managerLastProtocol = state.protocol
	a.openManager()
	a.managerSelection.Clear()
	a.managerRow = -1
	ids := state.ids
	if saved != nil {
		ids = []string{state.target}
	}
	selectRows := func() {
		for i, row := range a.managerRows() {
			for _, id := range ids {
				if row.id == id {
					a.managerSelection.Add(id)
					if a.managerRow < 0 {
						a.managerRow = i
						if !row.folder {
							a.selected = row.profile.ID
						}
					}
				}
			}
		}
	}
	selectRows()
	if saved != nil && a.managerRow < 0 {
		a.query = ""
		a.managerProtocol = ""
		a.managerLastProtocol = ""
		if state.target == saved.ID {
			a.managerGroup = saved.Group
			a.managerAll = false
		}
		selectRows()
	}
	if a.managerRow < 0 && strings.HasPrefix(state.target, "link:") {
		for _, link := range a.config.Links {
			if "link:"+link.ID == state.target {
				a.managerGroup = "Links"
				if link.Folder != "" {
					a.managerGroup += "/" + link.Folder
				}
				a.managerAll = false
				a.query = ""
				a.managerProtocol = ""
				a.managerLastProtocol = ""
				ids = []string{state.target}
				selectRows()
				break
			}
		}
	}
	if a.managerRow >= 0 {
		a.managerList.ScrollTo(a.managerRow, ui.Center)
	}
	if a.manager != nil {
		a.manager.Invalidate()
	}
}

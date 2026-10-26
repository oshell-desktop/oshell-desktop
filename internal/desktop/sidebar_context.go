package desktop

import (
	"path"
	"runtime"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
)

// A context exists only while dispatching a sidebar menu or shortcut. Shared
// session actions see this target without borrowing the manager's selection.
type sidebarMenuContext struct {
	rows  []sessionRow
	row   sessionRow
	valid bool
	group string
}

func (a *App) sidebarRow(id string) (sessionRow, bool) {
	if group, folder := strings.CutPrefix(id, "folder:"); folder {
		for _, directory := range a.config.AllDirectories() {
			if directory == group || strings.HasPrefix(directory, group+"/") {
				return sessionRow{id: id, name: path.Base(group), group: group, folder: true}, true
			}
		}
		return sessionRow{}, false
	}
	if linkID, reference := strings.CutPrefix(id, "link:"); reference {
		for _, link := range a.config.Links {
			if link.ID != linkID {
				continue
			}
			for _, profile := range a.config.Profiles {
				if link.ProfileID == profile.ID {
					return sessionRow{id: id, name: link.Name, group: path.Join("Links", link.Folder), linkID: link.ID, profile: profile}, true
				}
			}
			break
		}
		return sessionRow{}, false
	}
	for _, profile := range a.config.Profiles {
		if profile.ID == id {
			return sessionRow{id: id, name: profile.Name, group: profile.Group, profile: profile}, true
		}
	}
	return sessionRow{}, false
}

func (a *App) withSidebarContext(id string, action func()) bool {
	if a.passwordLocked() || a.configurationBusy() || a.modalActive() {
		return false
	}
	row, valid := a.sidebarRow(id)
	if id != "" && !valid {
		return false // A menu can outlive a configuration refresh.
	}
	if id == "" {
		a.sidebar.selection.Clear()
	} else if !a.sidebar.selection.Has(id) {
		a.sidebar.selection.Clear()
		a.sidebar.selection.Add(id)
	}
	var rows []sessionRow
	for key := range a.sidebar.selection.All() {
		if len(a.sidebar.visible) > 0 && !slices.Contains(a.sidebar.visible, key) {
			continue
		}
		if r, ok := a.sidebarRow(key); ok {
			rows = append(rows, r)
		}
	}
	previous := a.sidebar.context
	a.sidebar.context = &sidebarMenuContext{rows: rows, row: row, valid: valid, group: row.group}
	defer func() { a.sidebar.context = previous }()
	a.sidebar.selected = id
	action()
	return true
}

func (a *App) sessionActionGroup() string {
	if context := a.sidebar.context; context != nil {
		return context.group
	}
	return a.managerGroup
}

func (a *App) sidebarContextMenu(c *ui.Context, m *ui.Menu, id string) {
	a.withSidebarContext(id, func() { a.managerContextMenu(c, m, id) })
	m.Separator()
	if m.Item("全部展开").Chosen() {
		a.setSidebarExpanded(true)
	}
	if m.Item("全部收缩").Chosen() {
		a.setSidebarExpanded(false)
	}
}

func (a *App) sidebarItemActions(c *ui.Context, item ui.Element, id string) {

	item.ContextMenu(func(m *ui.Menu) { a.sidebarContextMenu(c, m, id) })
	item.HandleInput(func(ev ui.InputEvent) bool {
		if ev.Kind == ui.InputPointerDown && ev.Button == 0 {
			a.selectSidebar(id, ev.Mods)
			return false
		}
		if ev.Kind != ui.InputKeyDown {
			return false
		}
		copyMod := ev.Mods == ui.Ctrl || runtime.GOOS == "darwin" && ev.Mods == ui.Cmd
		var action func()
		switch {
		case copyMod && ev.Key == ui.KeyC:
			action = func() { a.copyManagerConfigurations(c) }
		case copyMod && ev.Key == ui.KeyV:
			action = func() { a.pasteManagerConfigurations(c) }
		case ev.Mods == 0 && ev.Key == ui.KeyF2:
			action = func() { a.beginManagerRename(a.sidebar.context.row) }
		case ev.Mods == 0 && ev.Key == ui.KeyEnter:
			action = func() { a.enterRow(a.sidebar.context.row) }
		case ev.Mods == 0 && ev.Key == ui.KeyDelete:
			action = a.deleteManagerSelection
		default:
			return false
		}
		return a.withSidebarContext(id, action)
	})
}

func (a *App) revealSidebar(id string) {
	row, valid := a.sidebarRow(id)
	if !valid {
		return
	}
	a.sidebar.query, a.sidebar.selected = "", id
	a.sidebar.selection.Clear()
	a.sidebar.selection.Add(id)
	if a.sidebar.open == nil {
		a.sidebar.open = map[string]*bool{}
	}
	for group := row.group; group != "" && group != "."; group = path.Dir(group) {
		open := true
		a.sidebar.open[group] = &open
	}
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *App) sidebarRenameDialog(c *ui.Context) {
	state := a.sidebar.rename
	open := state != nil
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(360).Gap(12).Children(func() {
			ui.Text(c, "重命名").FontSize(18).Bold()
			input := ui.TextInput(c, &state.name).Label("新名称").AutoFocus().FillWidth()
			if !state.focused {
				input.Focus().SetTextSelection(0, len([]rune(state.name)))
				state.focused = true
			}
			apply := func() {
				if a.sidebar.rename != state {
					return
				}
				if a.renameSessionRow(state.row, state.name) {
					id := state.row.id
					if state.row.folder {
						id = "folder:" + path.Join(managerParent(state.row.group), strings.TrimSpace(state.name))
					}
					a.revealSidebar(id)
					open = false
					a.sidebar.rename = nil
				} else {
					state.error = a.status
				}
			}
			if state.error != "" {
				ui.Text(c, state.error).TextColor(c.Theme().Danger).Wrap()
			}
			ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
				ui.Button(c, "取消").OnClick(func() { a.sidebar.rename = nil })
				ui.PrimaryButton(c, "保存").OnClick(apply)
			})
			if input.Submitted() {
				apply()
			}
		})
	})
	if !open {
		a.sidebar.rename = nil
	}
}

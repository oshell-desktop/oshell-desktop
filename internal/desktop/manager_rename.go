package desktop

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"path"
	"strings"
	"unicode"
)

type managerRenameState struct {
	row     sessionRow
	name    string
	error   string
	focused bool
}

func (a *App) beginManagerRename(r sessionRow) {
	if r.parent || r.folder && r.group == "Links" || a.configurationBusy() || a.passwordLocked() {
		return
	}
	state := &managerRenameState{row: r, name: r.name}
	if a.sidebar.context != nil {
		a.sidebar.rename = state
	} else {
		a.managerRename = state
	}
}
func (a *App) commitManagerRename() {
	state := a.managerRename
	if state == nil {
		return
	}
	if a.renameSessionRow(state.row, state.name) {
		a.managerRename = nil
		a.managerSelection.Clear()
		a.managerRow = -1
	}
}

func (a *App) renameSessionRow(r sessionRow, name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || strings.ContainsAny(name, "/\\") || strings.ContainsFunc(name, unicode.IsControl) || name == "." || name == ".." {
		a.status = "错误：名称不能为空或包含路径分隔符"
		return false
	}
	if name == r.name {
		return true
	}
	pinned := a.rowPinned(r)
	ok := a.changeConfig(func(cfg *core.Config) error {
		if r.folder {
			return cfg.MoveDirectory(r.group, path.Join(managerParent(r.group), name))
		}
		if r.linkID != "" {
			for i := range cfg.Links {
				if cfg.Links[i].ID == r.linkID && cfg.Links[i].Name == r.name {
					cfg.Links[i].Name = name
					return nil
				}
			}
		} else {
			for i := range cfg.Profiles {
				if cfg.Profiles[i].ID == r.profile.ID && cfg.Profiles[i].Name == r.name {
					cfg.Profiles[i].Name = name
					return nil
				}
			}
		}
		return fmt.Errorf("项目已变化，请重新选择")
	})
	if ok && pinned && r.folder {
		p := a.localUI.clone()
		delete(p.Pins, pinKey(r))
		p.Pins["d:"+name] = true
		a.saveLocalPreferences(p)
	}
	return ok
}
func (a *App) managerNameEditor(c *ui.Context, r sessionRow) bool {
	s := a.managerRename
	if s == nil || s.row.id != r.id {
		return false
	}
	input := ui.TextInput(c.Key("rename-"+r.id), &s.name).Label("重命名 " + r.name).AutoFocus().FillWidth().TextColor(c.Theme().Text)
	if !s.focused {
		input.Focus().SetTextSelection(0, len([]rune(s.name)))
	}
	input.HandleInput(func(ev ui.InputEvent) bool {
		if ev.Kind == ui.InputKeyDown && ev.Key == ui.KeyEscape {
			a.managerRename = nil
			return true
		}
		return false
	})
	if input.Submitted() {
		a.commitManagerRename()
		return true
	}
	if input.Focused() {
		s.focused = true
	} else if s.focused {
		a.managerRename = nil
	}
	return true
}

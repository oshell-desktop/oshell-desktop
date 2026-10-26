package desktop

import (
	"github.com/egoist/mygo/ui"
	"sort"
	"strings"
)

func pinKey(r sessionRow) string {
	if r.parent || r.linkID != "" {
		return ""
	}
	if r.folder {
		if managerParent(r.group) == "" {
			return "d:" + r.group
		}
		return ""
	}
	if r.group == "" {
		return "p:" + strings.ToLower(r.profile.ID)
	}
	return ""
}
func (a *App) rowPinned(r sessionRow) bool { k := pinKey(r); return k != "" && a.localUI.Pins[k] }
func (a *App) setSessionPins(rows []sessionRow, value bool) bool {
	p := a.localUI.clone()
	if p.Pins == nil {
		p.Pins = map[string]bool{}
	}
	for _, row := range rows {
		current, ok := a.sidebarRow(row.id)
		if !ok || pinKey(current) == "" {
			a.status = "只能置顶根目录的会话或目录，请刷新后重新选择"
			return false
		}
		key := pinKey(current)
		if value {
			p.Pins[key] = true
		} else {
			delete(p.Pins, key)
		}
	}
	return a.saveLocalPreferences(p)
}
func (a *App) pinsMenu(m *ui.Menu, rows []sessionRow) bool {
	if len(rows) == 0 {
		return false
	}
	all := true
	for _, r := range rows {
		if pinKey(r) == "" {
			return false
		}
		all = all && a.rowPinned(r)
	}
	label := "置顶"
	if all {
		label = "取消置顶"
	}
	if m.Item(label).Chosen() {
		a.setSessionPins(rows, !all)
		return true
	}
	return false
}
func (a *App) reconcileSessionPins() {
	if len(a.localUI.Pins) == 0 {
		return
	}
	valid := map[string]bool{}
	for _, d := range a.config.AllDirectories() {
		if managerParent(d) == "" {
			valid["d:"+d] = true
		}
	}
	for _, p := range a.config.Profiles {
		if p.Group == "" {
			valid["p:"+strings.ToLower(p.ID)] = true
		}
	}
	next := a.localUI.clone()
	changed := false
	for key := range next.Pins {
		if !valid[key] {
			delete(next.Pins, key)
			changed = true
		}
	}
	if changed {
		a.saveLocalPreferences(next)
	}
}
func (a *App) sortPinnedRows(rows []sessionRow) {
	sort.SliceStable(rows, func(i, j int) bool { return a.rowPinned(rows[i]) && !a.rowPinned(rows[j]) })
}

func (a *App) selectSidebar(id string, mods ui.Modifiers) {
	s := &a.sidebar
	if mods&ui.Shift != 0 && s.selected != "" {
		first, last := -1, -1
		for i, k := range s.visible {
			if k == s.selected {
				first = i
			}
			if k == id {
				last = i
			}
		}
		if first >= 0 && last >= 0 {
			if first > last {
				first, last = last, first
			}
			if mods&(ui.Ctrl|ui.Cmd) == 0 {
				s.selection.Clear()
			}
			for _, k := range s.visible[first : last+1] {
				s.selection.Add(k)
			}
			return
		}
	}
	if mods&(ui.Ctrl|ui.Cmd) != 0 {
		if s.selection.Has(id) {
			s.selection.Remove(id)
		} else {
			s.selection.Add(id)
		}
	} else {
		s.selection.Clear()
		s.selection.Add(id)
	}
	s.selected = id
}

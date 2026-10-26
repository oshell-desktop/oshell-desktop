package desktop

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"path"
	"sort"
	"strings"
)

func (a *App) addExistingLink(folder string) {
	a.closeManager()
	a.features.linkEditor = linkEditorState{open: true, mode: "addExisting", folder: folder, index: -1}
}
func (a *App) linkPickerProfiles() []core.Profile {
	terms := strings.Fields(strings.ToLower(a.features.linkEditor.query))
	var result []core.Profile
	for _, p := range a.presentationConfig().Profiles {
		hay := strings.ToLower(fmt.Sprintf("%s %s %s %d %s", p.Name, p.Host, p.Username, p.Port, p.Group))
		match := true
		for _, term := range terms {
			if !strings.Contains(hay, term) {
				match = false
				break
			}
		}
		if match {
			result = append(result, p)
		}
	}
	return result
}
func (a *App) savePickedLink() {
	f := &a.features.linkEditor
	if f.id == "" {
		f.error = "请选择已有会话"
		return
	}
	if a.changeConfig(func(cfg *core.Config) error {
		for _, p := range cfg.Profiles {
			if p.ID == f.id {
				return cfg.AddLink(p, f.folder)
			}
		}
		return fmt.Errorf("原会话已不存在，请重新选择")
	}) {
		f.open = false
	} else {
		f.error = a.status
	}
}
func (a *App) linkPickerView(c *ui.Context) {
	f := &a.features.linkEditor
	ui.Text(c, "添加已有会话链接").FontSize(18).Bold()
	ui.Text(c, "目标：/Links/"+f.folder).Wrap()
	if ui.TextInput(c, &f.query).Label("搜索已有会话").Placeholder("名称、主机、用户名、端口或目录").AutoFocus().Changed() {
		f.index = -1
		f.id = ""
		f.list = ui.ListState{}
		f.searchExpanded = nil
	}
	profiles := a.linkPickerProfiles()
	if f.expanded == nil {
		f.expanded = map[string]*bool{}
	}
	if f.searchExpanded == nil {
		f.searchExpanded = map[string]*bool{}
	}
	expansion := f.expanded
	searching := strings.TrimSpace(f.query) != ""
	if searching {
		expansion = f.searchExpanded
	}
	children := map[string][]string{}
	seen := map[string]bool{}
	for _, p := range profiles {
		for group := strings.Trim(p.Group, "/"); group != "" && group != "."; group = path.Dir(group) {
			if !seen[group] {
				seen[group] = true
				parent := path.Dir(group)
				if parent == "." {
					parent = ""
				}
				children[parent] = append(children[parent], group)
			}
		}
	}
	for parent := range children {
		sort.Strings(children[parent])
	}
	ui.Scroll(c).Height(260).Border(1, c.Theme().Border).Children(func() {
		ui.Tree(c, func() {
			var draw func(string)
			draw = func(group string) {
				for _, child := range children[group] {
					open := expansion[child]
					if open == nil {
						initial := searching
						open = &initial
						expansion[child] = open
					}
					sessionTreeItem(c.Key("folder:"+child), path.Base(child), open, func() { draw(child) }, false, sessionIconFor("", false, true, false))
				}
				for _, p := range profiles {
					if strings.Trim(p.Group, "/") == group {
						item := sessionTreeItem(c.Key(p.ID), p.Name, nil, nil, f.id == p.ID, sessionIconFor(p.Kind, false, false, false)).Tooltip(core.SessionSearchMetadata(p))
						if item.Clicked() {
							f.id = p.ID
						}
						if item.DoubleClicked() || item.Shortcut(0, ui.KeyEnter) {
							f.id = p.ID
							a.savePickedLink()
						}
					}
				}
			}
			draw("")
		})
	})
	if p, exists := a.profileByID(f.id); exists {
		ui.Text(c, fmt.Sprintf("%s · %s:%d · %s\n目录：/%s", strings.ToUpper(p.Kind), p.Host, p.Port, p.Username, p.Group)).Wrap().TextColor(c.Theme().TextMuted)
	} else {
		f.id = ""
		ui.Text(c, "选择会话查看主机、端口、用户和目录").TextColor(c.Theme().TextMuted)
	}
	if f.error != "" {
		ui.Text(c, f.error).TextColor(c.Theme().Danger)
	}
	ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
		ui.Button(c, "取消").OnClick(func() { f.open = false })
		ui.PrimaryButton(c, "添加链接").Disabled(f.id == "").OnClick(a.savePickedLink)
	})
}

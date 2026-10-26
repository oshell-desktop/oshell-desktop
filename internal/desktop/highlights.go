// SPDX-License-Identifier: GPL-3.0-only
package desktop

import (
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

func (a *App) showHighlights() {
	f := &a.features
	f.highlightSets = a.config.Clone().HighlightSets
	f.highlightActive = a.config.Settings.HighlightSetID
	f.highlightEditing = f.highlightActive
	f.highlightError = ""
	f.highlightOpen = true
	if f.highlightEditing == "" && len(f.highlightSets) > 0 {
		f.highlightEditing = f.highlightSets[0].ID
	}
	for i := range f.highlightSets {
		for j := range f.highlightSets[i].Rules {
			r := &f.highlightSets[i].Rules[j]
			r.Foreground = r.TextColor()
		}
	}
}
func (a *App) editedHighlightSet() *core.HighlightSet {
	for i := range a.features.highlightSets {
		if a.features.highlightSets[i].ID == a.features.highlightEditing {
			return &a.features.highlightSets[i]
		}
	}
	return nil
}
func (a *App) newHighlightSet(copyCurrent bool) {
	set := core.HighlightSet{ID: core.NewID(), Name: "新突出显示集"}
	if current := a.editedHighlightSet(); copyCurrent && current != nil {
		set.Name = current.Name + " - 副本"
		set.Rules = append([]core.HighlightRule(nil), current.Rules...)
		for i := range set.Rules {
			set.Rules[i].ID = core.NewID()
		}
	}
	a.features.highlightSets = append(a.features.highlightSets, set)
	a.features.highlightEditing = set.ID
}
func (a *App) saveHighlights() {
	f := &a.features
	for i := range f.highlightSets {
		for j := range f.highlightSets[i].Rules {
			r := &f.highlightSets[i].Rules[j]
			r.Color = r.Foreground
		}
		if e := f.highlightSets[i].Validate(); e != nil {
			f.highlightError = e.Error()
			return
		}
	}
	if a.changeConfig(func(cfg *core.Config) error {
		cfg.HighlightSets = (core.Config{HighlightSets: f.highlightSets}).Clone().HighlightSets
		cfg.Settings.HighlightSetID = f.highlightActive
		cfg.Settings.Highlights = nil
		return nil
	}) {
		f.highlightOpen = false
		a.applyAppearance()
	} else {
		f.highlightError = a.status
	}
}
func (a *App) highlightsDialog(c *ui.Context) {
	f := &a.features
	ui.Modal(c, &f.highlightOpen, func() {
		ui.Column(c).Width(690).Gap(12).Children(func() {
			ui.Text(c, "突出显示集").FontSize(18).Bold()
			var choices [][2]string
			for _, set := range f.highlightSets {
				choices = append(choices, [2]string{set.ID, set.Name})
			}
			if len(choices) > 0 {
				selectValue(c, "编辑突出显示集", &f.highlightEditing, choices)
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Button(c, "新建集").Disabled(len(f.highlightSets) >= 128).OnClick(func() { a.newHighlightSet(false) })
				ui.Button(c, "复制当前集").Disabled(a.editedHighlightSet() == nil || len(f.highlightSets) >= 128).OnClick(func() { a.newHighlightSet(true) })
				ui.Button(c, "删除当前集").Disabled(a.editedHighlightSet() == nil).OnClick(func() {
					for i, set := range f.highlightSets {
						if set.ID == f.highlightEditing {
							if f.highlightActive == set.ID {
								f.highlightActive = ""
							}
							f.highlightSets = append(f.highlightSets[:i], f.highlightSets[i+1:]...)
							f.highlightEditing = ""
							if len(f.highlightSets) > 0 {
								f.highlightEditing = f.highlightSets[0].ID
							}
							break
						}
					}
				})
			})
			if set := a.editedHighlightSet(); set != nil {
				ui.Row(c).Gap(8).Children(func() {
					ui.TextInput(c, &set.Name).Label("突出显示集名称").Grow(1)
					enabled := f.highlightActive == set.ID
					if ui.Checkbox(c, &enabled, "启用此集").Changed() {
						if enabled {
							f.highlightActive = set.ID
						} else {
							f.highlightActive = ""
						}
					}
				})
				ui.Text(c, "每集最多 32 条规则；${hostname} 匹配当前终端主机名。").TextColor(c.Theme().TextMuted)
				ui.Scroll(c).Height(270).Gap(10).Children(func() {
					for i := range set.Rules {
						r := &set.Rules[i]
						ui.Column(c.Key(r.ID)).Padding(8).Gap(6).Border(1, c.Theme().Border).Children(func() {
							ui.Row(c).Gap(6).Children(func() {
								ui.Checkbox(c, &r.Enabled, "启用规则")
								ui.TextInput(c, &r.Pattern).Label("匹配内容").Placeholder("关键字或正则表达式").Grow(1)
								ui.Button(c, "删除规则").OnClick(func() { set.Rules = append(set.Rules[:i], set.Rules[i+1:]...) })
							})
							ui.Row(c).Gap(8).Children(func() {
								ui.Checkbox(c, &r.Regex, "正则")
								ui.Checkbox(c, &r.CaseSensitive, "区分大小写")
								ui.TextInput(c, &r.Foreground).Label("文字颜色").Placeholder("#RRGGBB").Grow(1)
							})
						})
					}
				})
				ui.Button(c, "添加规则").OnClick(func() {
					set.Rules = append(set.Rules, core.HighlightRule{ID: core.NewID(), Foreground: "#FF5F56", Enabled: true})
				})
			}
			if f.highlightError != "" {
				ui.Text(c, f.highlightError).TextColor(c.Theme().Danger)
			}
			ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
				ui.Button(c, "取消").OnClick(func() { f.highlightOpen = false })
				ui.PrimaryButton(c, "应用突出显示集").OnClick(a.saveHighlights)
			})
		})
	})
}

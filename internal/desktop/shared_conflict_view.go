package desktop

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

func (a *App) sharedConflictChoiceCount() int {
	f := &a.features.sharedSync
	if f.plan == nil {
		return 0
	}
	count := 0
	for _, conflict := range f.plan.Conflicts {
		choice := f.choices[conflict.Key]
		if choice == core.SharedKeepLocal || choice == core.SharedKeepRemote {
			count++
		}
	}
	return count
}
func (a *App) chooseSharedConflict(key string, choice core.SharedMergeChoice) {
	f := &a.features.sharedSync
	if f.plan == nil || f.busy || choice != core.SharedKeepLocal && choice != core.SharedKeepRemote {
		return
	}
	found := false
	for i, conflict := range f.plan.Conflicts {
		if conflict.Key == key {
			found = true
			f.conflictIndex = i
			break
		}
	}
	if !found {
		return
	}
	if f.choices == nil {
		f.choices = map[string]core.SharedMergeChoice{}
	}
	f.choices[key] = choice
	for i, conflict := range f.plan.Conflicts {
		selected := f.choices[conflict.Key]
		if selected != core.SharedKeepLocal && selected != core.SharedKeepRemote {
			f.conflictIndex = i
			break
		}
	}
	f.conflictList.ScrollTo(f.conflictIndex, ui.Center)
}
func (a *App) sharedConflictComparison(c *ui.Context, height float32) {
	f := &a.features.sharedSync
	conflicts := f.plan.Conflicts
	if len(conflicts) == 0 {
		return
	}
	f.conflictIndex = max(0, min(f.conflictIndex, len(conflicts)-1))
	f.conflictList.Selected = &f.conflictIndex
	label := func(i int) string {
		prefix := "[待选] "
		switch f.choices[conflicts[i].Key] {
		case core.SharedKeepLocal:
			prefix = "[本机] "
		case core.SharedKeepRemote:
			prefix = "[共享] "
		}
		return prefix + conflicts[i].Title
	}
	f.conflictList.Label = label
	f.conflictList.Key = func(i int) any { return conflicts[i].Key }
	ui.Row(c).FillWidth().Height(height).Gap(12).Children(func() {
		ui.List(c.Key("conflict-list"), &f.conflictList, len(conflicts), func(i int) {
			ui.Row(c).Height(detailRowHeight).AlignItems(ui.Center).Padding(1, 6).Children(func() { ui.Text(c, label(i)).SingleLine() })
		}).Width(200).FillHeight().Border(1, c.Theme().Border).Label("需要确认的项目")
		conflict := conflicts[f.conflictIndex]
		reset := f.conflictDetailKey != conflict.Key
		f.conflictDetailKey = conflict.Key
		for _, side := range []struct {
			title, text, button string
			choice              core.SharedMergeChoice
		}{
			{"本机待保存版本", conflict.LocalDescription, "保留本机版本", core.SharedKeepLocal},
			{"共享目录版本", conflict.RemoteDescription, "保留共享版本", core.SharedKeepRemote},
		} {
			ui.Column(c.Key(side.title)).Grow(1).MinWidth(0).FillHeight().Gap(8).Children(func() {
				ui.Text(c, side.title).Bold()
				content := side.text
				// Keying the editor by the conflict resets both panes to their first line
				// when the user selects another item. Only this item's details are laid out.
				detail := ui.TextArea(c.Key(conflict.Key), &content).ReadOnly(true).Grow(1).FillWidth().FontSize(11).Font("monospace").Label(side.title)
				first := ui.Local(detail, "first-conflict-frame", func() bool { return true })
				if *first || reset {
					detail.TextSelection()
					detail.SetTextSelection(0, 0)
					*first = false
				}
				ui.Button(c, side.button).FillWidth().OnClick(func() { a.chooseSharedConflict(conflict.Key, side.choice) })
			})
		}
	})
	ui.Text(c, fmt.Sprintf("已选择 %d / %d 项；全部选择后才能确认同步。", a.sharedConflictChoiceCount(), len(conflicts))).TextColor(c.Theme().TextMuted)
}

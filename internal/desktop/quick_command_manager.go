package desktop

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"strings"
)

func (a *App) editQuickCommand(q *core.QuickCommand) {
	f := &a.features
	f.commandError = ""
	f.commandOriginal = nil
	value := core.NewQuickCommand()
	if q != nil {
		value = *q
		original := *q
		f.commandOriginal = &original
	}
	f.commandEdit = &value
}
func (a *App) commitQuickCommand(deleting bool) {
	f := &a.features
	q := f.commandEdit
	original := f.commandOriginal
	if deleting {
		q = f.commandDelete
		original = q
	}
	if q == nil {
		return
	}
	if !deleting {
		if err := q.Validate(); err != nil {
			f.commandError = err.Error()
			return
		}
	}
	if !a.changeConfig(func(cfg *core.Config) error {
		index := -1
		for i, stored := range cfg.QuickCommands {
			if stored.ID == q.ID {
				index = i
				break
			}
		}
		if original != nil && (index < 0 || cfg.QuickCommands[index] != *original) {
			return fmt.Errorf("快速命令已变化，请重新选择")
		}
		if deleting {
			cfg.QuickCommands = append(cfg.QuickCommands[:index], cfg.QuickCommands[index+1:]...)
		} else if index >= 0 {
			cfg.QuickCommands[index] = *q
		} else {
			cfg.QuickCommands = append(cfg.QuickCommands, *q)
		}
		return nil
	}) {
		f.commandError = a.status
		return
	}
	f.commandEdit = nil
	f.commandOriginal = nil
	f.commandDelete = nil
	f.commandError = ""
	f.commandRow = -1
}
func (a *App) quickCommandDialog(c *ui.Context) {
	f := &a.features
	desktopModal(c, &f.commandsOpen, func() {
		dialogBody(c, 740).Gap(10).Children(func() {
			windowHeading(c, "快速命令管理器")
			if f.commandError != "" {
				ui.Text(c, f.commandError).TextColor(c.Theme().Danger)
			}
			if q := f.commandEdit; q != nil {
				ui.Field(c, "名称", func() { ui.TextInput(c, &q.Name).Label("命令名称").AutoFocus() })
				ui.Field(c, "分组", func() { ui.TextInput(c, &q.Group).Label("命令分组") })
				ui.TextArea(c, &q.Text).Height(210).Label("命令内容")
				ui.Checkbox(c, &q.AppendReturn, "追加回车")
				dialogActions(c).Children(func() {
					ui.Button(c, "取消编辑").OnClick(func() { f.commandEdit = nil; f.commandOriginal = nil; f.commandError = "" })
					ui.PrimaryButton(c, "保存命令").OnClick(func() { a.commitQuickCommand(false) })
				})
				return
			}
			if q := f.commandDelete; q != nil {
				ui.Text(c, "删除“"+q.Name+"”？").Wrap()
				dialogActions(c).Children(func() {
					ui.Button(c, "取消删除").OnClick(func() { f.commandDelete = nil; f.commandError = "" })
					ui.Button(c, "确认删除").OnClick(func() { a.commitQuickCommand(true) })
				})
				return
			}
			commands := a.config.QuickCommands
			f.commandList.Selected = &f.commandRow
			f.commandList.Key = func(i int) any { return commands[i].ID }
			f.commandList.Label = func(i int) string { return commands[i].Name }
			table := ui.Table(c, &f.commandList, []ui.TableColumn{{Title: "名称", Width: 180}, {Title: "分组", Width: 120}, {Title: "命令"}}, len(commands), func(i, col int) {
				q := commands[i]
				text := q.Name
				if col == 1 {
					text = q.Group
				}
				if col == 2 {
					text = strings.ReplaceAll(q.Text, "\n", " ↵ ")
				}
				ui.Text(c, text).SingleLine()
			}).Height(280).Border(1, c.Theme().Border).Background(panelColor(c))
			valid := f.commandRow >= 0 && f.commandRow < len(commands)
			var selected core.QuickCommand
			if valid {
				selected = commands[f.commandRow]
			}
			if table.Submitted() && valid {
				a.loadCommandInComposer(selected)
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Button(c, "新建命令").OnClick(func() { a.editQuickCommand(nil) })
				ui.Button(c, "编辑命令").Disabled(!valid).OnClick(func() { a.editQuickCommand(&selected) })
				ui.Button(c, "删除命令…").Disabled(!valid).OnClick(func() { value := selected; f.commandDelete = &value })
				ui.Button(c, "填入撰写窗").Disabled(!valid).OnClick(func() { a.loadCommandInComposer(selected) })
				ui.Spacer(c)
				ui.Button(c, "关闭").OnClick(func() { f.commandsOpen = false })
			})
		})
	})
}

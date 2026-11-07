package desktop

import (
	"github.com/egoist/mygo/ui"
	"strconv"
)

var historyPresets = []string{"500", "1000", "3000", "5000", "10000", "20000"}

func historyPresetIndex(value string) int {
	for i, preset := range historyPresets {
		if value == preset {
			return i
		}
	}
	return len(historyPresets)
}

func (a *App) historySettings(c *ui.Context) {
	f := &a.features
	index := historyPresetIndex(f.settingsLines)
	if f.historyCustom {
		index = len(historyPresets)
	}
	before := index
	labels := append(append([]string(nil), historyPresets...), "自定义…")
	ui.Field(c, "历史行数", func() {
		choice := labels[index]
		ui.Select(c, &choice, labels).Label("历史行数预设").Width(160)
		index = historyPresetIndex(choice)
		if index != before {
			if index == len(historyPresets) {
				f.historyCustom = true
				f.settingsLines = f.historyCustomText
			} else {
				if f.historyCustom {
					f.historyCustomText = f.settingsLines
				}
				f.historyCustom = false
				f.settingsLines = historyPresets[index]
			}
		}
		if index == len(historyPresets) {
			ui.TextInput(c, &f.settingsLines).Label("自定义历史行数").Placeholder("500–100000").Width(160)
			f.historyCustomText = f.settingsLines
		}
	})
	warning := "历史按终端分别保存在内存中；过多行数、多会话或宽窗口会增加内存占用，使搜索和窗口缩放变慢。降低上限会丢弃较旧的历史；长期留存请使用终端记录文件。"
	color := c.Theme().TextMuted
	count, err := strconv.Atoi(f.settingsLines)
	if err != nil || count < 500 || count > 100000 {
		warning = "请输入 500～100000 的整数行数。\n" + warning
		color = c.Theme().Danger
	} else if count > 20000 {
		warning = "当前超过 20000 行；严重时可能因内存不足退出。\n" + warning
		color = c.Theme().Warning
	}
	ui.Text(c, warning).Wrap().FontSize(11).TextColor(color)
}

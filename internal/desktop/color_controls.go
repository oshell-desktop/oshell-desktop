package desktop

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"strconv"
)

var ansiColorNames = []string{"黑", "红", "绿", "黄", "蓝", "紫", "青", "白", "亮黑", "亮红", "亮绿", "亮黄", "亮蓝", "亮紫", "亮青", "亮白"}

func parseSettingColor(value string) (ui.Color, bool) {
	if len(value) != 7 || value[0] != '#' {
		return ui.Color{}, false
	}
	n, err := strconv.ParseUint(value[1:], 16, 24)
	if err != nil {
		return ui.Color{}, false
	}
	return ui.RGB(uint8(n>>16), uint8(n>>8), uint8(n)), true
}

type colorFieldState struct {
	value ui.Color
	text  string
}

func colorSettingField(c *ui.Context, label string, value *string) {
	box := ui.Column(c).Gap(4)
	state := ui.Local(box, "color", func() colorFieldState {
		color, ok := parseSettingColor(*value)
		if !ok {
			color = ui.RGB(0, 0, 0)
		}
		return colorFieldState{color, *value}
	})
	if state.text != *value {
		if color, ok := parseSettingColor(*value); ok {
			state.value = color
		}
		state.text = *value
	}
	box.Children(func() {
		ui.Text(c, label).FontSize(12)
		ui.Row(c).Gap(6).Children(func() {
			if ui.ColorWell(c, &state.value).Label(label + "颜色选择器").Tooltip("点击选择" + label + "颜色").Width(30).Height(28).Changed() {
				state.value.A = 255
				*value = fmt.Sprintf("#%02X%02X%02X", state.value.R, state.value.G, state.value.B)
				state.text = *value
			}
			ui.TextInput(c, value).Label(label + "色值").Placeholder("#RRGGBB").Grow(1).MinWidth(60)
		})
		if _, ok := parseSettingColor(*value); !ok {
			ui.Text(c, "请输入 #RRGGBB").FontSize(11).TextColor(c.Theme().Danger)
		}
	})
}
func (a *App) colorSchemePreview(c *ui.Context) {
	scheme := a.features.colorDraft
	ui.Text(c, "实时预览").Bold()
	if scheme.Validate() != nil {
		ui.Text(c, "色值格式有误，修正后恢复预览。").TextColor(c.Theme().Danger)
		return
	}
	cursor, cursorText := ui.Hex(scheme.Cursor), ui.Hex(scheme.CursorText)
	ui.Column(c).Padding(12).Gap(8).Radius(5).Background(ui.Hex(scheme.Background)).Children(func() {
		ui.Text(c, "user@server:~$ ls -la").Font(terminalFamily(a.features.settingsDraft.FontFamily)).FontSize(14).TextColor(ui.Hex(scheme.Foreground))
		ui.Row(c).Gap(8).Children(func() {
			ui.Text(c, "终端文字 Aa 0123 中文").TextColor(ui.Hex(scheme.Foreground))
			ui.Text(c, "选中文字").Padding(3, 6).Background(ui.Hex(scheme.Selection)).TextColor(ui.Hex(scheme.SelectionText))
			ui.Text(c, "光标").Padding(3, 6).Background(cursor).TextColor(cursorText)
		})
		ui.Grid(c).Columns(8).Gap(4).Children(func() {
			for i, value := range scheme.ANSI {
				ui.Text(c, ansiColorNames[i]+" Aa").FontSize(12).TextColor(ui.Hex(value))
			}
		})
	})
}

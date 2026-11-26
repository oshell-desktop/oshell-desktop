package desktop

import (
	"bytes"
	"fmt"
	"image"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

func appearanceNumber(c *ui.Context, label string, value *int, low, high, step int) {
	ui.Field(c, label, func() {
		n := float64(*value)
		if ui.NumberInput(c.Key(label), &n, float64(low), float64(high), float64(step)).Label(label).Changed() {
			*value = int(n)
		}
	})
}

func (a *App) terminalAppearanceSettings(c *ui.Context) {
	s := &a.features.settingsDraft
	ui.Divider(c)
	enabled := s.TerminalAppearance != nil
	if ui.Checkbox(c, &enabled, "自定义所有终端的外观").Changed() {
		if enabled {
			defaults := core.DefaultTerminalAppearance()
			s.TerminalAppearance = &defaults
		} else {
			s.TerminalAppearance = nil
		}
	}
	style := s.TerminalAppearance
	if style == nil {
		return
	}
	ui.Text(c, "跨设备同步扩展外观时，两端都需要支持这些选项。背景图片在各设备分别选择。").Wrap()
	sectionTitle(c, "终端边距与间距")
	ui.Grid(c).Columns(2).Gap(8).Children(func() {
		appearanceNumber(c, "上边距", &style.Top, 0, 128, 1)
		appearanceNumber(c, "下边距", &style.Bottom, 0, 128, 1)
		appearanceNumber(c, "左边距", &style.Left, 0, 128, 1)
		appearanceNumber(c, "右边距", &style.Right, 0, 128, 1)
		appearanceNumber(c, "行间距", &style.LineSpacing, -5, 32, 1)
		appearanceNumber(c, "字符间距", &style.CharacterSpacing, -5, 32, 1)
	})
	sectionTitle(c, "字形与光标")
	ui.Form(c, func() {
		ui.Field(c, "主字体", func() { a.fontPicker(c) })
		ui.Field(c, "主字号", func() { ui.NumberInput(c, &s.FontSize, 10, 26, 1).Label("主字号") })
		appearanceNumber(c, "主字体字重", &style.FontWeight, 100, 900, 100)
		ui.Field(c, "东亚字体", func() { ui.TextInput(c, &style.AsianFont).Placeholder("跟随主字体").Label("东亚字体") })
		ui.Field(c, "东亚字号", func() {
			label := "跟随主字号"
			if style.AsianSize != 0 {
				label = fmt.Sprint(style.AsianSize)
			}
			ui.MenuButton(c, label, func(m *ui.Menu) {
				if m.Item("跟随主字号").Checked(style.AsianSize == 0).Chosen() {
					style.AsianSize = 0
				}
				for i := 10; i <= 26; i++ {
					if m.Item(fmt.Sprint(i)).Checked(style.AsianSize == i).Chosen() {
						style.AsianSize = i
					}
				}
			})
		})
		appearanceNumber(c, "东亚字重（0 跟随）", &style.AsianWeight, 0, 900, 100)
		ui.Checkbox(c, &style.AllowProportional, "允许使用非等宽字体")
		ui.Field(c, "字体质量", func() {
			selectValue(c, "字体质量", &style.FontQuality, [][2]string{{"system", "系统默认"}, {"grayscale", "灰度抗锯齿"}})
		})
		ui.Field(c, "粗体文本", func() {
			selectValue(c, "粗体文本", &style.BoldStyle, [][2]string{{"both", "使用亮色和粗体"}, {"font", "只使用粗体"}, {"bright", "只使用亮色"}, {"none", "普通字形和颜色"}})
		})
		ui.Field(c, "光标形状", func() {
			selectValue(c, "光标形状", &style.CursorShape, [][2]string{{"block", "块"}, {"bar", "竖线"}, {"underline", "下划线"}})
		})
		ui.Checkbox(c, &style.CursorBlink, "闪烁光标")
		if style.CursorBlink {
			appearanceNumber(c, "闪烁间隔（毫秒）", &style.BlinkInterval, 100, 2000, 50)
		}
	})
	sectionTitle(c, "标签与背景")
	customTab := style.TabColor != ""
	if ui.Checkbox(c, &customTab, "自定义标签颜色").Changed() {
		if customTab {
			style.TabColor = "#8967B5"
		} else {
			style.TabColor = ""
		}
	}
	if customTab {
		colorSettingField(c, "标签", &style.TabColor)
	}
	ui.Row(c).Gap(8).Children(func() {
		ui.Button(c, "选择背景图片…").OnClick(a.chooseTerminalBackground)
		if path := a.localUIDraft.TerminalBackgroundFile; path != "" {
			ui.Text(c, filepath.Base(path)).SingleLine()
			ui.Button(c, "清除图片").OnClick(func() { a.localUIDraft.TerminalBackgroundFile = "" })
		}
	})
	ui.Field(c, "图片位置", func() {
		selectValue(c, "图片位置", &style.ImagePosition, [][2]string{{"center", "居中"}, {"fit", "适应窗口"}, {"fill", "铺满并裁剪"}, {"stretch", "拉伸"}})
	})
}

func readTerminalBackground(path string) (*ui.Bitmap, error) {
	data, err := core.ReadBounded(path)
	if err != nil {
		return nil, err
	}
	size, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("无法识别背景图片")
	}
	if size.Width <= 0 || size.Height <= 0 || size.Width > (16<<20)/size.Height {
		return nil, fmt.Errorf("背景图片尺寸超过 1600 万像素")
	}
	return ui.DecodeBitmap(data)
}

func (a *App) chooseTerminalBackground() {
	parent := a.preferencesWindow
	go func() {
		files, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: parent, Title: "终端背景图片", Filters: []mygo.FileFilter{{Name: "图片", Extensions: []string{"png", "jpg", "jpeg", "gif", "bmp", "webp"}}}})
		if err != nil || len(files) == 0 {
			return
		}
		_, err = readTerminalBackground(files[0])
		a.post(func() {
			if !a.settingsOpen || a.preferencesWindow != parent {
				return
			}
			if err != nil {
				a.features.settingsError = err.Error()
			} else {
				a.localUIDraft.TerminalBackgroundFile = files[0]
			}
			if parent != nil {
				parent.Invalidate()
			}
		})
	}()
}

func (a *App) terminalBackground() *ui.Bitmap {
	path := a.localUI.TerminalBackgroundFile
	if path != a.backgroundFile {
		a.backgroundFile, a.backgroundBitmap = path, nil
		if path != "" {
			img, err := readTerminalBackground(path)
			if err != nil {
				a.status = "无法加载本机终端背景图片"
			} else {
				a.backgroundBitmap = img
			}
		}
	}
	if a.config.Settings.TerminalAppearance == nil {
		return nil
	}
	return a.backgroundBitmap
}

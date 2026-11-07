// SPDX-License-Identifier: GPL-3.0-only
package desktop

import (
	"context"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"strings"
	"time"
)

func (a *App) loadFontCatalog() {
	if a.features.fontLoading {
		return
	}
	a.features.fontLoading = true
	a.features.fontError = ""
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	a.features.fontCancel = cancel
	go func() {
		defer cancel()
		fonts, e := core.FontCatalog(ctx)
		a.post(func() {
			a.features.fontLoading = false
			if e != nil {
				if ctx.Err() != context.Canceled {
					a.features.fontError = e.Error()
				}
				return
			}
			a.features.fontCatalog = fonts
		})
	}()
}
func (a *App) fontPicker(c *ui.Context) {
	s := &a.features.settingsDraft
	localNames := map[string]bool{}
	fonts := []core.FontEntry{{Name: "", Monospaced: true}, {Name: "DejaVu Sans Mono", Monospaced: true}, {Name: "Cascadia Code", Monospaced: true}}
	for _, file := range a.features.fontLibrary.draft.Fonts {
		for _, face := range file.Faces {
			fonts = append(fonts, face)
			localNames[strings.ToLower(face.Name)] = true
		}
	}
	fonts = append(fonts, a.features.fontCatalog...)
	combo := ui.ComboboxBase(c, &s.FontFamily)
	combo.Input.Label("终端字体").Placeholder("系统等宽（自动），或搜索本机字体").Padding(5, 8).Border(1, c.Theme().Border).Background(c.Theme().Surface)
	combo.Popup(func(panel ui.Element) {
		ui.StylePickerPanel(panel)
		count := 0
		seen := map[string]bool{}
		for _, font := range fonts {
			key := strings.ToLower(font.Name)
			if seen[key] {
				continue
			}
			seen[key] = true
			if combo.Filtering() && !strings.Contains(strings.ToLower(font.Name), strings.ToLower(s.FontFamily)) {
				continue
			}
			if count >= 200 {
				break
			}
			count++
			label := font.Name
			if label == "" {
				label = "系统等宽（自动）"
			} else if label == "DejaVu Sans Mono" || label == "Cascadia Code" {
				label += "（内置）"
			} else if localNames[key] {
				label += "（本机导入）"
			} else if font.Monospaced {
				label += "（等宽）"
			} else {
				label += "（系统）"
			}
			item := combo.Item(font.Name)
			ui.StylePopupOption(item, font.Name == s.FontFamily)
			item.Children(func() { ui.Text(c, label).SingleLine() })
		}
	})
	if value, ok := combo.Chosen(); ok {
		s.FontFamily = value
	}
	if s.FontFamily != "" && s.FontFamily != "DejaVu Sans Mono" {
		for _, font := range fonts {
			if strings.EqualFold(font.Name, s.FontFamily) && !font.Monospaced {
				ui.Text(c, "所选字体不是等宽字体，可能影响终端字符对齐。").Wrap().FontSize(11).TextColor(c.Theme().Warning)
				break
			}
		}
	}
	if a.features.fontLibrary.error != "" {
		ui.Text(c, "本机字体库："+a.features.fontLibrary.error).Wrap().FontSize(11).TextColor(c.Theme().Danger)
	}

	if a.features.fontLoading {
		ui.Text(c, "正在读取本机字体…").FontSize(11).TextColor(c.Theme().TextMuted)
	}
	if a.features.fontError != "" {
		ui.Text(c, a.features.fontError).FontSize(11).TextColor(c.Theme().Danger)
	}
}

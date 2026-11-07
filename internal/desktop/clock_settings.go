package desktop

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"math"
	"oshell-desktop/internal/core"
	"strconv"
	"strings"
	"time"
)

func (a *App) validateClockDraft() error {
	size, err := strconv.ParseFloat(a.clockSizeDraft, 64)
	if err != nil || math.IsNaN(size) || math.IsInf(size, 0) || size < 8 || size > 96 {
		return fmt.Errorf("时钟字号须为 8–96 点")
	}
	s := &a.features.settingsDraft
	if s.TerminalClockColor != "" {
		if _, ok := parseSettingColor(s.TerminalClockColor); !ok {
			return fmt.Errorf("时钟颜色应为 #RRGGBB")
		}
	}
	s.TerminalClockFontSize = size
	s.ClampClockStyle()
	return nil
}
func (a *App) clockSettings(c *ui.Context) {
	s := &a.features.settingsDraft
	ui.Checkbox(c, &s.TerminalClockEnabled, "显示终端浮动时钟")
	ui.Form(c, func() {
		ui.Field(c, "字体", func() {
			fonts := []string{"", "monospace"}
			for _, font := range a.features.fontCatalog {
				fonts = append(fonts, font.Name)
			}
			combo := ui.ComboboxBase(c, &s.TerminalClockFontName)
			combo.Input.Label("时钟字体").Placeholder("系统等宽（默认），或搜索本机字体").Padding(5, 8).Border(1, c.Theme().Border).Background(c.Theme().Surface)
			combo.Popup(func(panel ui.Element) {
				ui.StylePickerPanel(panel)
				shown := 0
				for _, name := range fonts {
					if combo.Filtering() && !strings.Contains(strings.ToLower(name), strings.ToLower(s.TerminalClockFontName)) {
						continue
					}
					if shown >= 200 {
						break
					}
					shown++
					label := name
					if label == "" {
						label = "系统等宽（默认）"
					}
					item := combo.Item(name)
					ui.StylePopupOption(item, name == s.TerminalClockFontName)
					item.Children(func() { ui.Text(c, label).SingleLine() })
				}
			})
			if value, ok := combo.Chosen(); ok {
				s.TerminalClockFontName = value
			}
		})
		ui.Field(c, "字号（8–96）", func() { ui.TextInput(c, &a.clockSizeDraft).Label("时钟字号") })
		ui.Field(c, "位置", func() { selectValue(c, "时钟位置", &s.TerminalClockPosition, core.ClockPositions) })
	})
	if ui.Checkbox(c, &a.clockFollowsText, "跟随终端文字颜色").Changed() {
		if a.clockFollowsText {
			s.TerminalClockColor = ""
		} else {
			s.TerminalClockColor = s.Scheme().Foreground
		}
	}
	if !a.clockFollowsText {
		colorSettingField(c.Key("clock-color"), "时钟文字", &s.TerminalClockColor)
	}
	ui.Text(c, fmt.Sprintf("透明度 %.0f%%", s.TerminalClockOpacity*100))
	ui.Slider(c, &s.TerminalClockOpacity, 0, 1).Label("时钟透明度")
	ui.Text(c, "缺失字体自动回退到系统等宽字体；0% 时停止时钟刷新。").FontSize(12).TextColor(c.Theme().TextMuted)
	sectionTitle(c, "叠加预览")
	preview := *s
	if size, err := strconv.ParseFloat(a.clockSizeDraft, 64); err == nil && size >= 8 && size <= 96 {
		preview.TerminalClockFontSize = size
	}
	if _, ok := parseSettingColor(preview.TerminalClockColor); !ok {
		preview.TerminalClockColor = ""
	}
	box := ui.Column(c).Height(180).FillWidth().Padding(12).Background(ui.Hex(s.Scheme().Background)).Label("时钟叠加预览").Children(func() {
		ui.Text(c, "user@server:~$ ls -la\nDocuments  Downloads  projects\nuser@server:~$ ping example.com\n64 bytes: time=12.4 ms\n终端文字继续显示，时钟浮层不占用行列。").Font("monospace").FontSize(13).TextColor(ui.Hex(s.Scheme().Foreground))
	})
	drawClockOverlay(box, preview, core.LocalClockText(time.Now()), nil)
}

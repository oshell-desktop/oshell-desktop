package desktop

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"strings"
	"unicode"

	"github.com/clipperhouse/uax29/v2/graphemes"
)

var sessionTitleModes = [][2]string{{"sessionName", "仅显示会话名称"}, {"shellIntegration", "Shell 集成"}, {"activeProbe", "主动探测"}, {"passive", "仅被动识别"}}

func validTitleMode(mode string) bool {
	for _, v := range sessionTitleModes {
		if v[0] == mode {
			return true
		}
	}
	return false
}
func (a *App) sessionTitleMode(p core.Profile) string {
	if mode := a.localUI.TitleModes[strings.ToLower(p.ID)]; validTitleMode(mode) {
		return mode
	}
	return p.TitleMode
}
func (a *App) paneTitleMode(p *pane) string {
	if mode := a.localUI.TitleModes[strings.ToLower(p.profile.ID)]; validTitleMode(mode) {
		return mode
	}
	if validTitleMode(p.titleMode) {
		return p.titleMode
	}
	return p.profile.TitleMode
}
func (a *App) paneDisplayTitle(p *pane) string {
	if p == nil {
		return ""
	}
	if a.paneTitleMode(p) == "sessionName" && p.profile.Kind != "blank" && (p.term == nil || !p.term.InLocalToolsMode()) {
		if saved, exists := a.profileByID(p.profile.ID); exists {
			return sessionTitleName(saved.Name)
		}
		return sessionTitleName(p.profile.Name)
	}
	if p.hostname != "" {
		return p.hostname
	}
	return p.profile.Name
}

// Match the reference's Unicode scalar filter and 256-scalar name limit.
// This only changes presentation, never the stored profile name.
func sessionTitleName(name string) string {
	clean := make([]rune, 0, min(len(name), 256))
	for _, r := range name {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			continue
		}
		clean = append(clean, r)
		if len(clean) == 256 {
			break
		}
	}
	if len(clean) == 0 {
		return "未命名会话"
	}
	return string(clean)
}

// Swift String.prefix counts extended grapheme clusters, not bytes or runes.
func splitTitleLabel(name string) string {
	it := graphemes.FromString(name)
	var label strings.Builder
	for n := 0; it.Next(); n++ {
		if n == 32 {
			return label.String() + "…"
		}
		label.WriteString(it.Value())
	}
	return name
}

func (a *App) displayTabTitle(t *tab) string {
	if t == nil {
		return ""
	}
	var names []string
	panes := 0
	counts := map[string]int{}
	a.walk(t.root, func(p *pane) {
		panes++
		name := a.paneDisplayTitle(p)
		if counts[name] == 0 {
			names = append(names, name)
		}
		counts[name]++
	})
	if len(names) == 0 {
		return t.name
	}
	if panes == 1 {
		return names[0]
	}
	shown := make([]string, 0, min(2, len(names)))
	for _, name := range names[:min(2, len(names))] {
		text := splitTitleLabel(name)
		if counts[name] > 1 {
			text += fmt.Sprintf(" ×%d", counts[name])
		}
		shown = append(shown, text)
	}
	if len(names) > 2 {
		extra := 0
		for _, name := range names[2:] {
			extra += counts[name]
		}
		shown = append(shown, fmt.Sprintf("+%d", extra))
	}
	return strings.Join(shown, " · ")
}
func (a *App) refreshSessionTitles() {
	for _, t := range a.tabs {
		a.walk(t.root, func(p *pane) {
			mode := a.paneTitleMode(p)
			if mode == "sessionName" {
				mode = "shellIntegration"
			}
			if p.hostObservation.Mode != mode {
				a.cancelHostProbe(p)
				p.hostObservation.Mode = mode
				p.hostProbe.attempted = false
			}
		})
		t.name = a.displayTabTitle(t)
	}
}
func (a *App) titleSettings(c *ui.Context) {
	defer a.imagePasteSettings(c)
	ui.Text(c, "标签（仅本机）").Bold()
	selectValue(c, "新建会话默认标题", &a.localUIDraft.DefaultTitleMode, sessionTitleModes)
	ui.Checkbox(c, &a.titleOverwrite, "同时覆盖已有会话和已打开终端的标题方式")
	ui.Checkbox(c, &a.localUIDraft.ConfirmTabClose, "关闭标签前确认")
	ui.Checkbox(c, &a.localUIDraft.ReduceMotion, "减少动态效果")
	ui.Text(c, "应用后立即更新，不需重连；不会修改共享配置或导出的会话属性。").Wrap().TextColor(c.Theme().TextMuted)
}
func (a *App) applyTitleDraft() {
	p := &a.localUIDraft
	if p.TitleModes == nil {
		p.TitleModes = map[string]string{}
	}
	for _, profile := range a.config.Profiles {
		p.TitleModes[strings.ToLower(profile.ID)] = p.DefaultTitleMode
	}
	windows := []*App{a}
	if a.hub != nil {
		windows = a.hub.configurationWindows()
	}
	for _, w := range windows {
		for _, t := range w.tabs {
			w.walk(t.root, func(pane *pane) {
				if pane.profile.Kind != "blank" {
					p.TitleModes[strings.ToLower(pane.profile.ID)] = p.DefaultTitleMode
				}
			})
		}
	}
}

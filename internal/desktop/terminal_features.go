// SPDX-License-Identifier: GPL-3.0-only
package desktop

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"oshell-desktop/internal/terminal"
)

func (a *App) routeInput(source *pane, ev ui.InputEvent) {
	a.pruneSyncTargets()
	a.noteHostInput(source, ev)
	if source.term == nil || !source.term.ManagedInputAvailable() || !a.features.syncEnabled || !a.features.syncTargets[source.id] {
		return
	}
	for _, t := range a.tabs {
		a.walk(t.root, func(p *pane) {
			if p != source && p.term != nil && !p.closed && a.features.syncTargets[p.id] {
				a.noteHostInput(p, ev)
				p.term.ReplayUserInput(ev)
			}
		})
	}
}

type pasteDestination struct {
	pane       *pane
	term       *terminal.Terminal
	localTools bool
}

func validPaste(text string) bool {
	return text != "" && len(text) <= 1<<20 && !strings.ContainsRune(text, 0)
}

func (a *App) requestPaste(source *pane, text string) {
	if a.workspaceLocked() || source == nil || source.closed || source.term == nil || text == "" {
		return
	}
	if !validPaste(text) {
		a.status = "粘贴内容不能包含空字符或超过 1 MiB；较大内容请使用文件上传。"
		return
	}
	local := source.term.InLocalToolsMode() && !source.term.ManagedInputAvailable()
	targets := []pasteDestination{{source, source.term, local}}
	if !local && a.features.syncEnabled && a.features.syncTargets[source.id] {
		targets = nil
		found := map[string]bool{}
		for _, p := range a.allPanes() {
			if a.features.syncTargets[p.id] {
				targets = append(targets, pasteDestination{p, p.term, false})
				found[p.id] = true
			}
		}
		for id, selected := range a.features.syncTargets {
			if selected && !found[id] {
				targets = append(targets, pasteDestination{})
			}
		}
	}
	if a.config.Settings.Input.ConfirmMultiline && strings.ContainsAny(text, "\r\n") {
		a.features.pasteTargets = targets
		a.features.pasteText = text
		a.features.pasteNoPreview = false
		a.features.pasteOpen = true
		return
	}
	a.dispatchPaste(targets, text)
}

// Validate every captured destination before changing any terminal viewport.
// Closing/reconnecting/moving a pane while the preview is open invalidates it.
func (a *App) dispatchPaste(targets []pasteDestination, text string) bool {
	if a.workspaceLocked() || !validPaste(text) || len(targets) == 0 {
		return false
	}
	current := map[*pane]bool{}
	for _, p := range a.allPanes() {
		current[p] = true
	}
	for _, target := range targets {
		p := target.pane
		if p == nil || !current[p] || p.closed || p.term == nil || p.term != target.term ||
			(!target.localTools && !p.term.ManagedInputAvailable()) ||
			(target.localTools && (!p.term.InLocalToolsMode() || !p.term.Available())) {
			a.status = "目标会话正在登录、传输、运行交互式本机工具或标签已关闭，未粘贴。"
			return false
		}
	}
	for _, target := range targets {
		a.noteHostCommand(target.pane, text, strings.ContainsAny(text, "\r\n"))
		target.term.Paste(text)
	}
	return true
}

func (a *App) cancelPaste() {
	a.features.pasteOpen = false
	a.features.pasteText = ""
	a.features.pasteTargets = nil
}

func (a *App) confirmPaste() {
	f := &a.features
	if f.pasteText != "" && !validPaste(f.pasteText) {
		a.status = "粘贴内容不能包含空字符或超过 1 MiB。"
		return
	}
	if f.pasteNoPreview && !a.changeConfig(func(cfg *core.Config) error { cfg.Settings.Input.ConfirmMultiline = false; return nil }) {
		return
	}
	a.dispatchPaste(f.pasteTargets, f.pasteText)
	a.cancelPaste()
}

func highlightRules(rules []core.HighlightRule) ([]terminal.HighlightRule, error) {
	var out []terminal.HighlightRule
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		if r.Pattern == "" {
			return nil, fmt.Errorf("突出显示匹配内容不能为空")
		}
		if r.Regex {
			if pattern, e := core.CompilePattern(r.Pattern, 0); e != nil {
				return nil, e
			} else {
				pattern.Close()
			}
		}
		color := func(s string) (ui.Color, error) {
			if s == "" {
				return ui.Color{}, nil
			}
			s = strings.TrimPrefix(s, "#")
			if len(s) != 6 && len(s) != 8 {
				return ui.Color{}, fmt.Errorf("颜色应为 #RRGGBB 或 #RRGGBBAA")
			}
			if _, e := hex.DecodeString(s); e != nil {
				return ui.Color{}, e
			}
			return ui.Hex("#" + s), nil
		}
		fg, e := color(r.Foreground)
		if e != nil {
			return nil, e
		}
		bg, e := color(r.Background)
		if e != nil {
			return nil, e
		}
		out = append(out, terminal.HighlightRule{Pattern: r.Pattern, Regex: r.Regex, CaseSensitive: r.CaseSensitive, Bold: r.Bold, Foreground: fg, Background: bg})
	}
	return out, nil
}
func (a *App) applyTerminalPreferences(p *pane) {
	if p.term == nil {
		return
	}
	s := a.config.Settings
	p.term.SetAutomaticTransfers(s.AutoZmodem)
	p.term.SetPreferColorEmoji(s.PreferColorEmoji)
	if e := p.term.SetHistoryLimit(s.ScrollbackLines); e != nil {
		a.status = "历史设置错误：" + e.Error()
	}
	p.term.SetInputPolicy(s.Input.CopyOnSelect, s.Input.RightClickPaste, s.Input.TrimLeading, s.Input.TrimTrailing)
	p.term.SetKeyAliases(terminalKeyAliases(s.KeyboardShortcuts))
	rules, e := highlightRules(a.config.ActiveHighlightRules(p.hostname))
	if e != nil {
		a.status = "突出显示配置错误：" + e.Error()
		return
	}
	if e = p.term.SetHighlights(rules); e != nil {
		a.status = "突出显示配置错误：" + e.Error()
	}
}
func (a *App) inputDialogs(c *ui.Context) {
	f := &a.features
	ui.Modal(c, &f.syncOpen, func() {
		ui.Column(c).Width(480).Gap(12).Children(func() {
			ui.Text(c, "同步键盘输入").FontSize(18).Bold()
			ui.Text(c, "仅在勾选的终端之间同步输入；不复制协议回复或鼠标上报。").TextColor(c.Theme().TextMuted)
			ui.Checkbox(c, &f.syncDraftEnabled, "启用同步输入")
			if f.syncSelectionDraft == nil {
				f.syncSelectionDraft = map[string]bool{}
			}
			ui.Scroll(c).Height(240).Gap(6).Children(func() {
				for _, t := range a.tabs {
					a.walk(t.root, func(p *pane) {
						if p.term == nil || p.closed {
							return
						}
						value := f.syncSelectionDraft[p.id]
						if ui.Checkbox(c.Key(p.id), &value, t.name+" · "+p.profile.Name).Changed() {
							f.syncSelectionDraft[p.id] = value
						}
					})
				}
			})
			if f.syncDraftError != "" {
				ui.Text(c, f.syncDraftError).TextColor(c.Theme().Danger)
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Button(c, "取消").OnClick(func() { f.syncOpen = false })
				ui.PrimaryButton(c, "应用同步目标").OnClick(a.applySyncInputDraft)
			})
		})
	})
	ui.Modal(c, &f.pasteOpen, func() {
		ui.Column(c).Width(600).Gap(12).Children(func() {
			ui.Text(c, "多行粘贴预览").FontSize(18).Bold()
			ui.Text(c, "请确认内容；粘贴会保留原有引号、缩进和换行。").TextColor(c.Theme().TextMuted)
			ui.TextArea(c, &f.pasteText).Height(240).Font("monospace")
			ui.Checkbox(c, &f.pasteNoPreview, "下次不再使用预览")
			ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
				ui.Button(c, "取消").OnClick(a.cancelPaste)
				ui.PrimaryButton(c, "粘贴").OnClick(a.confirmPaste)
			})
		})
	})
	a.highlightsDialog(c)
}

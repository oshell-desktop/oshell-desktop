// SPDX-License-Identifier: GPL-3.0-only
package desktop

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"sort"
	"strings"
)

type operatorState struct {
	historyOffset                       int
	historyDraft, historyValue          string
	selectedGroups, groupSelectionDraft map[string]bool
	groupsOpen                          bool
	noReturn                            bool
	quickFocus, composerFocus           ui.Handle
	selected                            map[string]bool
	selectionDraft                      map[string]bool
	targetsOpen                         bool
	composerOpen                        bool
	composerText                        string
	composerReturn                      bool
	composerTargets                     map[string]bool
	pendingTargets                      []*pane
	pendingText                         string
	pendingReturn, pendingComposer      bool
	error                               string
}

var sendScopes = [][2]string{{"current", "当前会话"}, {"tab", "当前标签全部分屏"}, {"visible", "当前可见终端"}, {"all", "本窗口全部终端"}, {"selected", "所选终端"}, {"currentGroup", "当前标签组"}, {"selectedGroups", "所选标签组"}}

// The persisted scope seeds new windows, while existing windows retain their
// own runtime choice. Protected startup waits until the config is unlocked.
func (a *App) sendScope() string {
	if a.quickSendScope == "" && !a.passwordLocked() {
		a.quickSendScope = a.config.Settings.QuickSendScope
		if a.quickSendScope == "" {
			a.quickSendScope = "current"
		}
	}
	return a.quickSendScope
}

func (a *App) scopeName() string {
	for _, v := range sendScopes {
		if v[0] == a.sendScope() {
			return v[1]
		}
	}
	return "当前会话"
}
func (a *App) allPanes() []*pane {
	var out []*pane
	for _, t := range a.tabs {
		a.walk(t.root, func(p *pane) { out = append(out, p) })
	}
	return out
}
func (a *App) sendCandidates() []*pane {
	scope := a.sendScope()
	if scope == "current" || scope == "" {
		if p := a.activePane(); p != nil {
			return []*pane{p}
		}
		return nil
	}
	var out []*pane
	if scope == "visible" {
		return a.visiblePanes()
	}
	if scope == "currentGroup" || scope == "selectedGroups" {
		a.ensureGroups()
		for _, t := range a.tabs {
			if scope == "currentGroup" && t.group == a.groups.active || scope == "selectedGroups" && a.features.operators.selectedGroups[t.group] {
				a.walk(t.root, func(p *pane) { out = append(out, p) })
			}
		}
		return out
	}
	if scope == "tab" {
		if t := a.activeTab(); t != nil {
			a.walk(t.root, func(p *pane) { out = append(out, p) })
		}
		return out
	}
	for _, p := range a.allPanes() {
		if scope == "all" || a.features.operators.selected[p.id] {
			out = append(out, p)
		}
	}
	return out
}
func readyPanes(candidates []*pane) []*pane {
	var out []*pane
	seen := map[*pane]bool{}
	for _, p := range candidates {
		if p != nil && !seen[p] && !p.closed && p.term != nil && p.term.ManagedInputAvailable() {
			out = append(out, p)
			seen[p] = true
		}
	}
	return out
}
func (a *App) setSendScope(scope string) {
	if scope == "selectedGroups" {
		o := &a.features.operators
		a.ensureGroups()
		retained := map[string]bool{}
		o.groupSelectionDraft = map[string]bool{}
		for id, selected := range o.selectedGroups {
			if selected && a.groupByID(id) != nil {
				retained[id] = true
				o.groupSelectionDraft[id] = true
			}
		}
		o.selectedGroups = retained
		if len(o.groupSelectionDraft) == 0 && a.groupByID(a.groups.active) != nil {
			o.groupSelectionDraft[a.groups.active] = true
		}
		o.groupsOpen = true
		return
	}
	if scope == "selected" {
		o := &a.features.operators
		retained := map[string]bool{}
		o.selectionDraft = map[string]bool{}
		for _, p := range a.allPanes() {
			if !p.closed && o.selected[p.id] {
				retained[p.id] = true
				o.selectionDraft[p.id] = true
			}
		}
		o.selected = retained
		if len(o.selectionDraft) == 0 {
			for _, p := range readyPanes(a.sendCandidates()) {
				o.selectionDraft[p.id] = true
			}
		}
		o.targetsOpen = true
		return
	}
	if a.changeConfig(func(c *core.Config) error { c.Settings.QuickSendScope = scope; return nil }) {
		a.quickSendScope = scope
	}
}
func (a *App) loadQuickCommand(q core.QuickCommand) {
	a.quick = q.Text
	a.features.operators.noReturn = !q.AppendReturn
	if strings.ContainsAny(q.Text, "\r\n") {
		a.openComposer(q.Text, q.AppendReturn)
	} else {
		a.features.operators.quickFocus.Focus()
	}
}
func (a *App) quickCommandsMenu(m *ui.Menu) {
	groups := map[string][]core.QuickCommand{}
	var names []string
	for _, q := range a.config.QuickCommands {
		group := q.Group
		if group == "" {
			group = "未分组"
		}
		if _, ok := groups[group]; !ok {
			names = append(names, group)
		}
		groups[group] = append(groups[group], q)
	}
	sort.Strings(names)
	for _, group := range names {
		m.Submenu(group, func(m *ui.Menu) {
			for _, q := range groups[group] {
				if m.Item(q.Name).Chosen() {
					a.loadQuickCommand(q)
				}
			}
		})
	}
}
func (a *App) openComposer(text string, appendReturn bool) {
	if strings.ContainsRune(text, 0) || len(text) > 1<<20 {
		a.status = "错误：内容不能包含空字符或超过 1 MiB"
		return
	}
	o := &a.features.operators
	o.composerOpen = true
	o.composerText = text
	o.composerReturn = appendReturn
	o.error = ""
	o.composerTargets = map[string]bool{}
	for _, p := range a.sendCandidates() {
		o.composerTargets[p.id] = true
	}
	o.composerFocus.Focus()
}
func (a *App) prepareSend() {
	if strings.TrimSpace(a.quick) == "" {
		return
	}
	if len(a.quick) > 65536 || strings.ContainsRune(a.quick, 0) {
		a.status = "错误：快速命令不能包含空字符或超过 64 KiB"
		return
	}
	if strings.ContainsAny(a.quick, "\r\n") {
		a.openComposer(a.quick, !a.features.operators.noReturn)
		return
	}
	a.prepareDispatch(a.quick, !a.features.operators.noReturn, a.sendCandidates(), false)
}
func (a *App) prepareDispatch(text string, appendReturn bool, candidates []*pane, composer bool) {
	o := &a.features.operators
	o.error = ""
	if text == "" || !composer && strings.TrimSpace(text) == "" || strings.ContainsRune(text, 0) || len(text) > 1<<20 {
		o.error = "内容为空、含空字符或超过 1 MiB"
		a.status = "错误：" + o.error
		return
	}
	targets := readyPanes(candidates)
	if len(targets) == 0 || composer && len(targets) != len(candidates) {
		o.error = "所选范围包含不可输入的终端，命令未发送"
		a.status = "错误：" + o.error
		return
	}
	o.pendingText = text
	o.pendingReturn = appendReturn
	o.pendingTargets = append([]*pane(nil), targets...)
	o.pendingComposer = composer
	a.send()
}
func (a *App) send() {
	o := &a.features.operators
	targets := readyPanes(o.pendingTargets)
	// A target moved to another window is no longer part of this window's dispatch.
	current := map[*pane]bool{}
	for _, p := range a.allPanes() {
		current[p] = true
	}
	if o.pendingComposer {
		valid := len(targets) == len(o.pendingTargets) && len(targets) > 0
		for _, p := range targets {
			valid = valid && current[p]
		}
		if !valid {
			o.error = "发送目标已关闭、移走或不可输入，命令未发送"
			a.status = "错误：" + o.error
			o.pendingTargets = nil
			o.pendingText = ""
			a.confirmSend = false
			return
		}
	}
	count := 0
	for _, p := range targets {
		if !current[p] {
			continue
		}
		a.noteHostCommand(p, o.pendingText, o.pendingReturn || strings.ContainsAny(o.pendingText, "\r\n"))
		if o.pendingComposer {
			p.term.SendManagedText(o.pendingText, o.pendingReturn)
		} else {
			p.term.Send(core.ManagedInput(o.pendingText, o.pendingReturn))
		}
		count++
	}
	if count == 0 {
		o.error = "发送前目标已关闭、移走或开始传输，命令未发送"
		a.status = "错误：" + o.error
	} else {
		a.rememberQuickSend(o.pendingText)
		if !o.pendingComposer {
			a.quick = ""
			o.noReturn = false
			o.historyOffset = 0
		}
		a.status = fmt.Sprintf("已发送至 %d 个终端", count)
	}
	a.confirmSend = false
	o.pendingTargets = nil
	o.pendingText = ""
}
func (a *App) targetChoices(c *ui.Context, selected map[string]bool) {
	for _, t := range a.tabs {
		a.walk(t.root, func(p *pane) {
			if p.profile.Kind == "ftp" || p.profile.Kind == "sftp" && !p.terminalOnly {
				return
			}
			value := selected[p.id]
			label := t.name
			if t.name != p.profile.Name {
				label += " · " + p.profile.Name
			}
			if len(readyPanes([]*pane{p})) == 0 {
				label += "（暂不可输入）"
			}
			if ui.Checkbox(c.Key(p.id), &value, label).Changed() {
				selected[p.id] = value
			}
		})
	}
}
func (a *App) operatorDialogs(c *ui.Context) {
	o := &a.features.operators
	ui.Modal(c, &o.groupsOpen, func() {
		ui.Column(c).Width(440).Gap(10).Children(func() {
			ui.Text(c, "选择发送标签组").Bold()
			for _, g := range a.groups.root.leaves() {
				value := o.groupSelectionDraft[g.id]
				label := a.groupTitle(g)
				if g.hidden {
					label += "（隐藏）"
				}
				if ui.Checkbox(c.Key(g.id), &value, label).Changed() {
					o.groupSelectionDraft[g.id] = value
				}
			}
			ui.Button(c, "应用分组").OnClick(func() {
				if a.changeConfig(func(cfg *core.Config) error { cfg.Settings.QuickSendScope = "selectedGroups"; return nil }) {
					a.quickSendScope = "selectedGroups"
					o.selectedGroups = o.groupSelectionDraft
					o.groupsOpen = false
				}
			})
		})
	})
	ui.Modal(c, &o.targetsOpen, func() {
		ui.Column(c).Width(520).Gap(12).Children(func() {
			ui.Text(c, "选择快速发送目标").FontSize(18).Bold()
			ui.Scroll(c).Height(270).Gap(6).Children(func() { a.targetChoices(c, o.selectionDraft) })
			ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
				ui.Button(c, "取消").OnClick(func() { o.targetsOpen = false })
				ui.PrimaryButton(c, "应用目标").OnClick(func() {
					if a.changeConfig(func(cfg *core.Config) error { cfg.Settings.QuickSendScope = "selected"; return nil }) {
						a.quickSendScope = "selected"
						o.selected = o.selectionDraft
						o.targetsOpen = false
					}
				})
			})
		})
	})
}
func (a *App) composerView(c *ui.Context) {
	o := &a.features.operators
	if !o.composerOpen {
		return
	}
	// Use an opaque highlight so selection remains distinct from the input
	// surface, while preserving readable text in either appearance.
	selection := ui.Hex("#6099DF")
	if c.Theme().Dark {
		selection = ui.Hex("#2563EB")
	}
	ui.Column(c).Height(245).MinHeight(180).Padding(8).Gap(6).BorderWidth(1, 0, 0, 0).BorderColor(c.Theme().Border).Children(func() {
		ui.Row(c).Gap(8).Children(func() {
			ui.Text(c, "多行命令撰写").Bold()
			ui.Spacer(c)
			ui.Checkbox(c, &o.composerReturn, "末尾追加回车")
			ui.Button(c, "收起").OnClick(func() { o.composerOpen = false })
		})
		ui.Row(c).Grow(1).MinHeight(0).Gap(10).Children(func() {
			ui.TextArea(c, &o.composerText).Label("撰写命令内容").SelectionColor(selection).AutoFocus().Font(terminalFamily(a.config.Settings.FontFamily)).Bind(&o.composerFocus).Grow(1).FillHeight()
			ui.Scroll(c).Width(240).Gap(5).Children(func() { a.targetChoices(c, o.composerTargets) })
		})
		ui.Row(c).Gap(8).Children(func() {
			if o.error != "" {
				ui.Text(c, o.error).TextColor(c.Theme().Danger).Grow(1)
			} else {
				ui.Text(c, "发送前确认内容和目标，缩进与换行会保留。").TextColor(c.Theme().TextMuted).Grow(1)
			}
			ui.Button(c, "保存为快速命令").Disabled(o.composerText == "").OnClick(a.saveComposerAsCommand)
			ui.PrimaryButton(c, "发送撰写内容").OnClick(func() {
				var targets []*pane
				for _, p := range a.allPanes() {
					if o.composerTargets[p.id] {
						targets = append(targets, p)
					}
				}
				a.prepareDispatch(o.composerText, o.composerReturn, targets, true)
			})
		})
	})
}
func (a *App) toggleQuickSend() {
	a.changeConfig(func(c *core.Config) error { c.Settings.QuickSendVisible = !c.Settings.QuickSendVisible; return nil })
}
func (a *App) focusQuickSend() {
	if !a.config.Settings.QuickSendVisible {
		a.toggleQuickSend()
	}
	a.features.operators.quickFocus.Focus()
}

func (a *App) loadCommandInComposer(q core.QuickCommand) {
	if e := q.Validate(); e != nil {
		a.features.commandError = e.Error()
		return
	}
	a.features.commandsOpen = false
	a.openComposer(q.Text, q.AppendReturn)
}

func (a *App) saveComposerAsCommand() {
	o := &a.features.operators
	q := core.NewQuickCommand()
	q.Text = o.composerText
	q.AppendReturn = o.composerReturn
	if err := q.Validate(); err != nil {
		o.error = err.Error()
		return
	}
	a.openCommands()
	a.features.commandEdit = &q
	a.features.commandOriginal = nil
}

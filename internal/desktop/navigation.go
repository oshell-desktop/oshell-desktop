// SPDX-License-Identifier: GPL-3.0-only
package desktop

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"sort"
	"strings"
)

type switchEntry struct {
	Title, Detail, Search string
	Pane                  *pane
	Profile               core.Profile
	Tab                   *tab
}
type switchState struct {
	filesOnly    bool
	open         bool
	query        string
	selected     int
	list         ui.ListState
	restore      ui.Handle
	wasOpen      bool
	scope        string
	selectionKey string
}

func (entry switchEntry) key() string {
	if entry.Pane != nil {
		return "pane:" + entry.Pane.id
	}
	return "profile:" + strings.ToLower(entry.Profile.ID)
}

func (s *switchState) reconcileSelection(entries []switchEntry) {
	s.selected = 0
	for i, entry := range entries {
		if entry.key() == s.selectionKey {
			s.selected = i
			break
		}
	}
}

func (a *App) togglePaneZoom() {
	t := a.activeTab()
	if t == nil || t.root == nil || t.root.pane != nil {
		return
	}
	if t.zoomed != nil {
		t.zoomed = nil
	} else {
		t.zoomed = t.active
	}
	t.active.focus.Focus()
}
func (a *App) cyclePane(delta int) {
	t := a.activeTab()
	if t == nil {
		return
	}
	var panes []*pane
	a.walk(t.root, func(p *pane) { panes = append(panes, p) })
	if len(panes) < 2 {
		return
	}
	at := 0
	for i, p := range panes {
		if p == t.active {
			at = i
			break
		}
	}
	a.activatePane(t, panes[(at+delta+len(panes))%len(panes)])
}
func (a *App) activatePane(t *tab, p *pane) {
	t.active = p
	if t.zoomed != nil {
		t.zoomed = p
	}
	a.selectTab(t)
	p.focus.Focus()
}
func (a *App) switchEntries() []switchEntry {
	query := core.NewSessionQuery(a.features.switcher.query)
	if !query.Valid {
		return nil
	}
	a.ensureGroups()
	var result []switchEntry
	for _, t := range a.tabs {
		if a.features.switcher.filesOnly || a.features.switcher.scope == "saved" {
			break
		}
		g := a.groupByID(t.group)
		group := a.groupTitle(g)
		number := 1
		for i, v := range a.groupTabs(g) {
			if v == t {
				number = i + 1
				break
			}
		}
		paneNumber := 0
		a.walk(t.root, func(p *pane) {
			paneNumber++
			detail := fmt.Sprintf("%s · 标签 %d · 分屏 %d", group, number, paneNumber)
			if g.hidden {
				detail += " · 隐藏组"
			}
			if p.term != nil && p.term.InLocalToolsMode() {
				detail += " · 已结束 / 本机工具"
			} else {
				detail += " · 运行中"
			}
			detail += " · " + p.profile.Name
			if p.profile.Host != "" {
				detail += " · " + p.profile.Username + "@" + p.profile.Address()
			}
			title := a.paneDisplayTitle(p)
			search := title + " " + detail + " " + p.profile.Group + " " + p.profile.Kind
			result = append(result, switchEntry{title, detail, core.NormalizeSessionSearch(search + " " + core.SessionSearchMetadata(p.profile)), p, p.profile, t})
		})
	}
	saved := a.presentationConfig().Profiles
	sort.SliceStable(saved, func(i, j int) bool { return strings.ToLower(saved[i].Name) < strings.ToLower(saved[j].Name) })
	for _, p := range saved {
		if !a.features.switcher.filesOnly && a.features.switcher.scope == "opened" {
			break
		}
		if a.features.switcher.filesOnly && p.Kind != "ssh" && p.Kind != "sftp" && p.Kind != "ftp" {
			continue
		}
		detail := "新建连接 · " + p.Group + " · " + p.Kind
		if p.Host != "" {
			detail += " · " + p.Username + "@" + p.Address()
		}
		result = append(result, switchEntry{p.Name, detail, core.NormalizeSessionSearch(p.Name + " " + detail + " " + core.SessionSearchMetadata(p)), nil, p, nil})
	}
	filtered := result[:0]
	for _, entry := range result {
		if query.Matches(entry.Search) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}
func (a *App) showQuickSwitch() {
	f := &a.features.switcher
	f.filesOnly = false
	f.list = ui.ListState{}
	f.open = true
	f.wasOpen = true
	f.query = ""
	f.selected = 0
	f.scope = "all"
	f.selectionKey = ""
	f.restore = a.lastInputFocus
}
func (a *App) quickSwitchDialog(c *ui.Context) {
	f := &a.features.switcher
	ui.Modal(c, &f.open, func() {
		ui.Column(c).Width(720).Gap(10).Children(func() {
			title := "快速切换 / 连接会话"
			if f.filesOnly {
				title = "选择文件会话"
			}
			ui.Text(c, title).FontSize(18).Bold()
			input := ui.TextInput(c, &f.query).Label("快速会话搜索").Placeholder("主机、IP、账号、端口、目录、标签组；空格分隔多个关键词").AutoFocus()
			if !f.filesOnly {
				selectValue(c, "搜索范围", &f.scope, [][2]string{{"all", "全部"}, {"opened", "已打开终端"}, {"saved", "已保存"}})
			}
			entries := a.switchEntries()
			total := len(entries)
			if len(entries) > 200 {
				entries = entries[:200]
			}
			f.reconcileSelection(entries)
			open := func() {
				if f.selected < 0 || f.selected >= len(entries) {
					return
				}
				entry := entries[f.selected]
				f.open = false
				f.wasOpen = false
				if entry.Pane != nil && !entry.Pane.closed {
					a.activatePane(entry.Tab, entry.Pane)
				} else if entry.Pane == nil {
					profile, exists := a.profileByID(entry.Profile.ID)
					if !exists {
						return
					}
					if f.filesOnly && entry.Profile.Kind == "ssh" {
						a.openSavedFileSession(profile)
					} else {
						a.request(profile, 0)
					}
				}
			}
			if c.Shortcut(0, ui.KeyDown) && len(entries) > 0 {
				f.selected = min(f.selected+1, len(entries)-1)
				f.list.ScrollTo(f.selected, ui.Center)
			}
			if c.Shortcut(0, ui.KeyUp) && len(entries) > 0 {
				f.selected = max(f.selected-1, 0)
				f.list.ScrollTo(f.selected, ui.Center)
			}
			if input.Submitted() {
				open()
			}
			f.list.Selected = &f.selected
			ui.List(c, &f.list, len(entries), func(i int) {
				entry := entries[i]
				row := ui.Column(c).Padding(7, 9).Gap(3)
				row.Children(func() {
					ui.Text(c, entry.Title).Bold().SingleLine()
					ui.Text(c, entry.Detail).FontSize(11).TextColor(c.Theme().TextMuted).SingleLine()
				})
				if row.DoubleClicked() {
					f.selected = i
					open()
				}
			}).Height(300).Border(1, c.Theme().Border)
			f.selectionKey = ""
			if f.selected >= 0 && f.selected < len(entries) {
				f.selectionKey = entries[f.selected].key()
			}
			ui.Row(c).Gap(8).Children(func() {
				status := fmt.Sprintf("显示 %d / %d 个结果", len(entries), total)
				if !core.NewSessionQuery(f.query).Valid {
					status = "搜索条件过长，请缩短关键词"
				} else if total == 0 {
					status = "没有匹配的会话"
				}
				ui.Text(c, status).TextColor(c.Theme().TextMuted)
				ui.Spacer(c)
				ui.Button(c, "取消").OnClick(func() { f.open = false })
				action := "新建连接"
				if f.selected >= 0 && f.selected < len(entries) && entries[f.selected].Pane != nil {
					action = "切换到会话"
				}
				ui.PrimaryButton(c, action).Disabled(len(entries) == 0).OnClick(open)
			})
		})
	})
	if f.wasOpen && !f.open {
		f.wasOpen = false
		f.restore.Focus()
	}
}
func (a *App) refreshPaneVisibility() {
	visible := map[*pane]bool{}
	for _, p := range a.visiblePanes() {
		visible[p] = true
	}
	for _, t := range a.tabs {
		t.unread = false
		a.walk(t.root, func(p *pane) {
			shown := visible[p]
			p.visible.Store(shown)
			if shown {
				p.pendingUnread.Store(false)
			} else if p.pendingUnread.Load() {
				t.unread = true
			}
			if p.term != nil {
				p.term.SetVisible(shown)
			}
		})
	}
	if a.hub != nil {
		a.hub.refreshClock()
	}
}

func (a *App) rememberInputFocus(c *ui.Context) {
	if a.modalActive() {
		return
	}
	if a.features.operators.quickFocus.FocusWithin(c) {
		a.lastInputFocus = a.features.operators.quickFocus
		return
	}
	if a.features.operators.composerFocus.FocusWithin(c) {
		a.lastInputFocus = a.features.operators.composerFocus
		return
	}
	for _, p := range a.allPanes() {
		if p.focus.FocusWithin(c) {
			a.lastInputFocus = p.focus
			return
		}
	}
}

func (a *App) showFileSessionPicker() {
	a.showQuickSwitch()
	a.features.switcher.filesOnly = true
}

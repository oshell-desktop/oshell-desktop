package desktop

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/terminal"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// Search belongs to the pane, and travels with its terminal across windows.
type terminalSearchState struct {
	open, regex, sensitive, wholeWord, busy, focus bool
	query, err                                     string
	matches                                        []terminal.Match
	index, generation                              int
	revision                                       uint64
	due, checked                                   time.Time
}

func (a *App) hasOpenTerminalSearch() bool {
	for _, p := range a.allPanes() {
		if p.search.open {
			return true
		}
	}
	return false
}
func (a *App) openSearch() {
	p := a.activePane()
	if p == nil || p.term == nil {
		return
	}
	s := &p.search
	if text := p.term.SearchSelection(); text != "" && !strings.ContainsAny(text, "\r\n") && len(text) <= 4096 {
		if s.regex {
			text = regexp.QuoteMeta(text)
		}
		s.query = text
	}
	s.open, s.focus = true, true
	a.refreshTerminalSearch(p, 0, true)
}
func (a *App) closeTerminalSearch(p *pane) {
	s := &p.search
	s.open, s.busy = false, false
	s.generation++
	s.matches, s.index = nil, -1
	s.due = time.Time{}
	p.term.ClearSearchSelection()
	p.focus.Focus()
}
func (a *App) navigateSearch(direction int) {
	p := a.activePane()
	if p == nil || p.term == nil {
		return
	}
	if p.search.query == "" {
		a.openSearch()
		return
	}
	a.refreshTerminalSearch(p, direction, false)
}
func (a *App) refreshTerminalSearch(p *pane, direction int, reset bool) {
	s := &p.search
	reset = reset || !s.due.IsZero()
	s.generation++
	generation := s.generation
	s.due, s.checked = time.Time{}, time.Now()
	if s.query == "" {
		s.matches, s.index, s.err, s.busy = nil, -1, "", false
		p.term.ClearSearchSelection()
		return
	}
	if reset {
		s.matches, s.index = nil, -1
		p.term.ClearSearchSelection()
	}
	s.busy = true
	query, regex, sensitive, whole, term := s.query, s.regex, s.sensitive, s.wholeWord, p.term
	go func() {
		page, err := term.NavigateSearch(query, regex, sensitive, whole, direction, reset)
		owner := p.owner.Load()
		if owner == nil {
			owner = a
		}
		owner.post(func() {
			if p.closed || generation != s.generation || p.term != term {
				return
			}
			s.busy, s.err, s.revision = false, "", page.Revision
			s.matches, s.index = page.Matches, page.Index
			if err != nil {
				s.err = err.Error()
				s.open = true
				term.ClearSearchSelection()
				return
			}
			if len(page.Matches) == 0 {
				term.ClearSearchSelection()
				return
			}
			// Refreshing a live count must not pull the viewport away from output.
			if page.Current != nil && (direction != 0 || reset) {
				if err := term.Reveal(*page.Current); err != nil {
					s.due = time.Now().Add(160 * time.Millisecond)
				}
			}
		})
	}()
}
func (a *App) terminalSearchBar(c *ui.Context, tab *tab, p *pane) {
	s := &p.search
	if !s.open {
		return
	}
	composing := false
	ui.Column(c).FillWidth().Height(64).Padding(5, 8).Gap(4).Background(c.Theme().Surface).Children(func() {
		ui.Row(c).FillWidth().Height(26).Gap(5).Children(func() {
			input := ui.TextInput(c, &s.query).Label("搜索终端文本").Placeholder("搜索终端历史").Grow(1).MinWidth(0)
			if s.focus {
				input.Focus().SetTextSelection(0, utf8.RuneCountInString(s.query))
				s.focus = false
			}
			if input.Focused() {
				tab.active = p
			}
			if input.Changed() {
				s.generation++
				s.busy = false
				s.due = time.Now().Add(160 * time.Millisecond)
			}
			composing = input.Composing()
			input.HandleInput(func(ev ui.InputEvent) bool {
				if ev.Kind != ui.InputKeyDown || composing {
					return false
				}
				if ev.Key == ui.KeyEscape {
					a.closeTerminalSearch(p)
					return true
				}
				if ev.Key == ui.KeyEnter {
					dir := 1
					if ev.Mods&ui.Shift != 0 {
						dir = -1
					}
					a.refreshTerminalSearch(p, dir, false)
					return true
				}
				return false
			})
			toolButton(c, "上一个匹配", nil, "↑").Width(26).Disabled(len(s.matches) == 0).OnClick(func() { a.refreshTerminalSearch(p, -1, false) })
			toolButton(c, "下一个匹配", nil, "↓").Width(26).Disabled(len(s.matches) == 0).OnClick(func() { a.refreshTerminalSearch(p, 1, false) })
			toolButton(c, "关闭搜索", closeIcon, "").Width(26).OnClick(func() { a.closeTerminalSearch(p) })
		})
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			ui.MenuButton(c, "选项", func(m *ui.Menu) {
				for _, option := range []struct {
					label string
					value *bool
				}{{"区分大小写", &s.sensitive}, {"正则表达式", &s.regex}, {"整词匹配", &s.wholeWord}} {
					if m.Item(option.label).Checked(*option.value).Chosen() {
						*option.value = !*option.value
						a.refreshTerminalSearch(p, 0, true)
					}
				}
			}).Label("搜索选项").Height(22).Width(86)
			status := fmt.Sprintf("%d / %d", max(0, s.index+1), len(s.matches))
			if s.query == "" {
				status = ""
			} else if s.busy {
				status = "搜索中…"
			} else if s.err != "" {
				status = s.err
			} else if len(s.matches) == 0 {
				status = "无匹配"
			} else if len(s.matches) > 1000 {
				status = "1000+ 处匹配"
				if s.index >= 0 {
					status = fmt.Sprintf("%d / 1000+", s.index+1)
				}
			} else if s.index < 0 {
				status = fmt.Sprintf("共 %d 处匹配", len(s.matches))
			}
			ui.Text(c, status).SingleLine().Grow(1).MinWidth(0).FontSize(11).TextColor(c.Theme().TextMuted).Tooltip(status)
		})
	})
	if composing {
		c.After(160 * time.Millisecond)
		return
	}
	now := time.Now()
	if !s.due.IsZero() && !now.Before(s.due) {
		a.refreshTerminalSearch(p, 0, true)
	}
	if !s.busy && s.due.IsZero() && now.Sub(s.checked) >= 500*time.Millisecond {
		s.checked = now
		if s.revision != p.term.SearchRevision() {
			a.refreshTerminalSearch(p, 0, false)
		}
	}
	c.After(160 * time.Millisecond)
}

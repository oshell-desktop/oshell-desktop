package desktop

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

// These preferences are deliberately outside Config and the shared contract.
type localPreferences struct {
	Version           int                       `json:"version,omitempty"`
	DeepFocusShortcut core.KeyboardShortcut     `json:"deepFocusShortcut"`
	ImagePasteLocal   bool                      `json:"imagePasteLocal"`
	ImagePasteSSH     map[string]imagePasteRule `json:"imagePasteSSH,omitempty"`
	DefaultTitleMode  string                    `json:"defaultTitleMode"`
	TitleModes        map[string]string         `json:"titleModes,omitempty"`
	ConfirmTabClose   bool                      `json:"confirmTabClose"`
	ReduceMotion      bool                      `json:"reduceMotion"`
	Pins              map[string]bool           `json:"pins,omitempty"`
	SidebarVisible    bool                      `json:"sidebarVisible"`
	SidebarShortcut   core.KeyboardShortcut     `json:"sidebarShortcut"`
	WordSeparators    string                    `json:"wordSeparators"`
	ShiftWhitespace   bool                      `json:"shiftWhitespace"`
}

const defaultWordSeparators = " /\\()\"'-:,.;<>~!@#$%^&*|+=[]{}?"

func defaultLocalPreferences() localPreferences {
	var code, focusCode uint16
	for k, name := range core.ShortcutKeyNames {
		if name == "F" {
			focusCode = k
		}
		if name == "B" {
			code = k
		}
	}
	return localPreferences{DeepFocusShortcut: core.KeyboardShortcut{KeyCode: focusCode, Modifiers: 1 | 2 | 8}, DefaultTitleMode: "sessionName", ConfirmTabClose: true, SidebarVisible: true, SidebarShortcut: core.KeyboardShortcut{KeyCode: code, Modifiers: 1 | 8}, WordSeparators: defaultWordSeparators, ShiftWhitespace: true}
}
func (p localPreferences) clone() localPreferences {
	p.ImagePasteSSH = maps.Clone(p.ImagePasteSSH)
	p.TitleModes = maps.Clone(p.TitleModes)
	p.Pins = maps.Clone(p.Pins)
	return p
}
func loadLocalPreferences(dir string) localPreferences {
	p, err := readLocalPreferences(dir)
	if err != nil {
		return defaultLocalPreferences()
	}
	return p
}
func readLocalPreferences(dir string) (localPreferences, error) {
	p := defaultLocalPreferences()
	b, err := core.ReadBounded(filepath.Join(dir, "desktop-preferences.json"))
	if os.IsNotExist(err) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	next := p
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&next); err != nil {
		return p, err
	}
	if !json.Valid(b) || next.Version < 0 || next.Version > 1 {
		return p, fmt.Errorf("本机设置格式或版本不受支持")
	}
	if !next.DeepFocusShortcut.Valid() {
		next.DeepFocusShortcut = p.DeepFocusShortcut
	}
	if !next.SidebarShortcut.Valid() {
		next.SidebarShortcut = p.SidebarShortcut
	}
	if !validTitleMode(next.DefaultTitleMode) {
		next.DefaultTitleMode = p.DefaultTitleMode
	}
	return next, nil
}
func (a *App) saveLocalPreferences(p localPreferences) bool {
	p = p.clone()
	_, err := readLocalPreferences(a.store.Dir)
	var b []byte
	if err == nil {
		b, err = json.Marshal(p)
	}
	if err == nil {
		err = core.AtomicWrite(filepath.Join(a.store.Dir, "desktop-preferences.json"), b)
	}
	if err != nil {
		a.features.settingsError = err.Error()
		a.status = "本机设置保存失败：" + err.Error()
		return false
	}
	apply := func(target *App) {
		target.localUI = p.clone()
		target.managerCache = nil
		target.refreshSessionTitles()
		for _, tab := range target.tabs {
			target.walk(tab.root, func(pane *pane) {
				if pane.term != nil {
					pane.term.SetSelectionPolicy(p.WordSeparators, p.ShiftWhitespace)
				}
			})
		}
		if target.win != nil {
			target.win.Invalidate()
		}
	}
	apply(a)
	if a.hub != nil {
		for _, target := range a.hub.windows {
			apply(target)
		}
		if a.hub.fileWindow != nil {
			apply(a.hub.fileWindow)
		}
	}
	return true
}
func (a *App) toggleSidebar() {
	if a.passwordLocked() || a.modalActive() {
		return
	}
	p := a.localUI.clone()
	p.SidebarVisible = !p.SidebarVisible
	a.saveLocalPreferences(p)
}

type sidebarState struct {
	selection            ui.Selection[string]
	visible              []string
	capture, deepCapture bool
	query, selected      string
	open                 map[string]*bool
	filteredQuery        string
	filteredOpen         map[string]*bool
	context              *sidebarMenuContext
	rename               *managerRenameState
}

func (s *sidebarState) expansion() map[string]*bool {
	query := strings.TrimSpace(s.query)
	if s.filteredQuery != query {
		s.filteredQuery = query
		s.filteredOpen = nil
	}
	if query != "" {
		if s.filteredOpen == nil {
			s.filteredOpen = map[string]*bool{}
		}
		return s.filteredOpen
	}
	if s.open == nil {
		s.open = map[string]*bool{}
	}
	return s.open
}

func (a *App) setSidebarExpanded(expanded bool) {
	state := a.sidebar.expansion()
	_, folders := sidebarEntries(a.config, strings.TrimSpace(a.sidebar.query))
	for folder := range folders {
		if folder == "" {
			continue
		}
		if state[folder] == nil {
			state[folder] = new(bool)
		}
		*state[folder] = expanded
	}
}

func (a *App) localSelectionSettings(c *ui.Context) {
	p := &a.localUIDraft
	ui.Text(c, "选择").Bold()
	ui.TextInput(c, &p.WordSeparators).Label("双击选词分隔符").FillWidth()
	ui.Text(c, "首次双击按分隔符选词；同一位置再次双击按空白扩大选择。三击仍选整行。").TextColor(c.Theme().TextMuted).Wrap()
	ui.Button(c, "重置分隔符").OnClick(func() { p.WordSeparators = defaultWordSeparators })
	ui.Checkbox(c, &p.ShiftWhitespace, "Shift+双击按空白选取（关闭后按分隔符）")
	s := &a.features.settingsDraft.Input
	ui.Checkbox(c, &s.CopyOnSelect, "选中即复制")
	ui.Checkbox(c, &s.TrimLeading, "复制时清理头部空白")
	ui.Checkbox(c, &s.TrimTrailing, "复制时清理尾部空白")
	ui.Divider(c)
	ui.Checkbox(c, &p.SidebarVisible, "显示会话侧栏")
	ui.Button(c, "全心专注："+p.DeepFocusShortcut.Display(runtime.GOOS)).OnClick(func() { a.sidebar.capture = true; a.sidebar.deepCapture = true })
	ui.Button(c, "切换侧栏："+p.SidebarShortcut.Display(runtime.GOOS)).OnClick(func() { a.sidebar.capture = true; a.sidebar.deepCapture = false })
	if a.sidebar.capture {
		ui.Box(c).Height(45).FillWidth().Focusable().AutoFocus().Label("侧栏快捷键录入").Border(1, c.Theme().Accent).HandleInput(func(ev ui.InputEvent) bool {
			if ev.Kind != ui.InputKeyDown {
				return false
			}
			if ev.Key == ui.KeyEscape {
				a.sidebar.capture = false
				return true
			}
			key, ok := shortcutEvent(ev)
			if !ok || !key.Valid() {
				a.features.settingsError = "请使用带修饰键的快捷键"
				return true
			}
			if other := a.features.settingsDraft.KeyboardShortcuts.Conflict(key, "", runtime.GOOS); other != "" {
				a.features.settingsError = "快捷键与 " + other + " 冲突"
				return true
			}
			other := p.DeepFocusShortcut
			if a.sidebar.deepCapture {
				other = p.SidebarShortcut
			}
			if shortcutMods(key) == shortcutMods(other) && key.KeyCode == other.KeyCode {
				a.features.settingsError = "快捷键与另一个本机操作冲突"
				return true
			}
			if a.sidebar.deepCapture {
				p.DeepFocusShortcut = key
			} else {
				p.SidebarShortcut = key
			}
			a.sidebar.capture = false
			a.features.settingsError = ""
			return true
		}).Children(func() { ui.Text(c, "按下快捷键；Esc 取消") })
	}
	ui.Text(c, "侧栏和选词设置仅保存在本机，不参与共享同步。").TextColor(c.Theme().TextMuted)
}

type sidebarEntry struct {
	id, name, group string
	profile         core.Profile
}

func sidebarEntries(cfg core.Config, query string) ([]sidebarEntry, map[string]bool) {
	q := core.NewSessionQuery(query)
	entries := []sidebarEntry{}
	folders := map[string]bool{"": true}
	parent := func(group string) {
		for group != "" && group != "." {
			folders[group] = true
			group = path.Dir(group)
		}
	}
	if !q.Valid {
		return entries, folders
	}
	for _, d := range cfg.AllDirectories() {
		if query == "" || q.Matches(core.NormalizeSessionSearch(d)) {
			parent(d)
		}
	}
	profiles := map[string]core.Profile{}
	add := func(id, name, group string, p core.Profile) {
		match := p
		match.Name, match.Group = name, group
		if query != "" && !q.Matches(core.SessionSearchMetadata(match)+" "+core.NormalizeSessionSearch(p.Name)) {
			return
		}
		entries = append(entries, sidebarEntry{id, name, group, p})
		parent(group)
	}
	for _, p := range cfg.Profiles {
		profiles[p.ID] = p
		add(p.ID, p.Name, p.Group, p)
	}
	for _, l := range cfg.Links {
		if p, ok := profiles[l.ProfileID]; ok {
			add("link:"+l.ID, l.Name, path.Join("Links", l.Folder), p)
		}
	}
	sort.SliceStable(entries, func(i, j int) bool { return strings.ToLower(entries[i].name) < strings.ToLower(entries[j].name) })
	return entries, folders
}
func (a *App) sidebarView(c *ui.Context) {
	s := &a.sidebar
	if s.open == nil {
		s.open = map[string]*bool{}
	}
	ui.Column(c).Width(235).MinWidth(150).MinHeight(0).BorderWidth(0, 1, 0, 0).BorderColor(c.Theme().Border).Children(func() {
		ui.Row(c).Padding(6, 8).Gap(2).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "会话").FontSize(13).Bold().Grow(1)
			rowIconButton(c, "管理会话", settingsIcon, c.Theme().Accent).Tooltip("打开会话管理").OnClick(a.openManager)
			rowIconButton(c, "隐藏会话侧栏", collapseSidebarIcon, c.Theme().TextMuted).Tooltip("收起会话侧栏").OnClick(a.toggleSidebar)
		})
		ui.TextInput(c, &s.query).Label("搜索会话侧栏").Placeholder("名称 / 主机 / 用户 / 端口 / 目录").FillWidth()
		ui.Row(c).Padding(4, 8).Gap(4).Children(func() {
			ui.Button(c, "全部展开").Height(24).FontSize(11).Padding(2, 6).BorderWidth(0).Tooltip("展开所有目录；搜索时仅操作当前结果").OnClick(func() { a.setSidebarExpanded(true) })
			ui.Button(c, "全部收缩").Height(24).FontSize(11).Padding(2, 6).BorderWidth(0).Tooltip("收缩所有目录；搜索时仅操作当前结果").OnClick(func() { a.setSidebarExpanded(false) })
		})
		if !core.NewSessionQuery(s.query).Valid {
			ui.Text(c, "搜索内容不能超过 4096 UTF-8 字节或 32 个关键词").Wrap().TextColor(c.Theme().Danger)
			return
		}
		expansion := s.expansion()
		entries, folders := sidebarEntries(a.presentationConfig(), strings.TrimSpace(s.query))
		children := map[string][]string{}
		for group := range folders {
			if group != "" {
				p := path.Dir(group)
				if p == "." {
					p = ""
				}
				children[p] = append(children[p], group)
			}
		}
		for p := range children {
			sort.SliceStable(children[p], func(i, j int) bool {
				l, r := children[p][i], children[p][j]
				lp, rp := a.localUI.Pins["d:"+l], a.localUI.Pins["d:"+r]
				if lp != rp {
					return lp
				}
				return l < r
			})
		}
		ui.Scroll(c).Label("会话侧栏列表").Grow(1).MinHeight(0).ContextMenu(func(m *ui.Menu) {
			a.sidebarContextMenu(c, m, "")
		}).Children(func() {
			s.visible = nil
			ui.Tree(c, func() {
				var draw func(string)
				draw = func(group string) {
					for phase := 0; phase < 2; phase++ {
						for _, child := range children[group] {
							pinned := group == "" && a.localUI.Pins["d:"+child]
							if pinned != (phase == 0) {
								continue
							}
							open := expansion[child]
							if open == nil {
								v := strings.TrimSpace(s.query) != ""
								open = &v
								expansion[child] = open
							}
							id := "folder:" + child
							s.visible = append(s.visible, id)
							name := path.Base(child)
							if pinned {
								name = "↑ " + name
							}
							item := sessionTreeItem(c.Key("dir:"+child), name, open, func() { draw(child) }, s.selection.Has(id), sessionIconFor("", child == "Links", true, false)).Tooltip("/" + child)
							a.sidebarItemActions(c, item, id)
						}
						for _, entry := range entries {
							if entry.group != group {
								continue
							}
							pinned := group == "" && a.localUI.Pins["p:"+strings.ToLower(entry.id)]
							if pinned != (phase == 0) {
								continue
							}
							e := entry
							s.visible = append(s.visible, e.id)
							name := e.name
							if pinned {
								name = "↑ " + name
							}
							item := sessionTreeItem(c.Key(e.id), name, nil, nil, s.selection.Has(e.id), sidebarSessionStyle(e)).Tooltip(sessionListTooltip(e.profile, strings.HasPrefix(e.id, "link:")))
							a.sidebarItemActions(c, item, e.id)
							if item.DoubleClicked() {
								a.request(e.profile, 0)
							}
						}
					}
				}
				draw("")
			})
		})
	})
}

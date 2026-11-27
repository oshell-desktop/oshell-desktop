// SPDX-License-Identifier: GPL-3.0-only
package core

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"unicode"
)

func (c Config) Validate() error {
	if a := c.Settings.TerminalAppearance; a != nil {
		raw, err := json.Marshal(a)
		if err != nil {
			return err
		}
		if _, err := canonicalSharedJSON(raw, "terminalAppearance"); err != nil {
			return err
		}
	}
	if e := ValidateProxyCatalog(c.Proxies, c.Profiles); e != nil {
		return e
	}
	if e := c.Settings.KeyboardShortcuts.ValidateStored(); e != nil {
		return e
	}
	if len(c.Profiles) > 2000 || len(c.Directories) > 4000 || len(c.Links) > 4000 || len(c.QuickCommands) > 1000 {
		return fmt.Errorf("配置项目过多")
	}
	ids := map[string]bool{}
	for _, p := range c.Profiles {
		if p.ID == "" || ids[p.ID] {
			return fmt.Errorf("会话 ID 缺失或重复")
		}
		ids[p.ID] = true
		if e := p.Validate(); e != nil {
			return e
		}
		if _, e := NormalizeDirectory(p.Group); e != nil {
			return e
		}
	}
	linkIDs := map[string]bool{}
	for _, l := range c.Links {
		if l.ID == "" || linkIDs[l.ID] || !ids[l.ProfileID] || strings.TrimSpace(l.Name) == "" {
			return fmt.Errorf("快捷引用无效或源会话不存在")
		}
		linkIDs[l.ID] = true
		if _, e := NormalizeDirectory(l.Folder); e != nil {
			return e
		}
	}
	for _, d := range c.Directories {
		if _, e := NormalizeDirectory(d); e != nil {
			return e
		}
	}
	commands := map[string]bool{}
	for _, q := range c.QuickCommands {
		if q.ID == "" || commands[q.ID] || strings.TrimSpace(q.Name) == "" || len(q.Name) > 256 || len(q.Text) > 1<<20 {
			return fmt.Errorf("快速命令无效或过长")
		}
		commands[q.ID] = true
		if e := q.Validate(); e != nil {
			return e
		}
	}
	sets := map[string]bool{}
	if len(c.HighlightSets) > 128 {
		return fmt.Errorf("突出显示集过多")
	}
	for _, h := range c.HighlightSets {
		if sets[h.ID] {
			return fmt.Errorf("突出显示集 ID 重复")
		}
		sets[h.ID] = true
		if e := h.Validate(); e != nil {
			return e
		}
	}
	if c.Settings.HighlightSetID != "" && !sets[c.Settings.HighlightSetID] {
		return fmt.Errorf("选中的突出显示集不存在")
	}
	switch c.Settings.QuickSendScope {
	case "", "current", "tab", "all", "visible", "selected", "currentGroup", "selectedGroups":
	default:
		return fmt.Errorf("快速发送范围无效")
	}
	return nil
}

func NormalizeDirectory(s string) (string, error) {
	if len(s) > 2048 || strings.Contains(s, "\\") || strings.ContainsFunc(s, unicode.IsControl) {
		return "", fmt.Errorf("目录应使用 / 分隔，不能包含控制字符")
	}
	var parts []string
	for _, part := range strings.Split(strings.TrimSpace(s), "/") {
		part = strings.TrimSpace(part)
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			if len(parts) > 0 {
				parts = parts[:len(parts)-1]
			}
			continue
		}
		parts = append(parts, part)
	}
	if len(parts) > 32 {
		return "", fmt.Errorf("目录层级过深")
	}
	return strings.Join(parts, "/"), nil
}
func Within(s, parent string) bool {
	return parent == "" || s == parent || strings.HasPrefix(s, parent+"/")
}
func (c Config) Clone() Config {
	out := c
	out.Proxies = slices.Clone(c.Proxies)
	if c.SharedPreferences != nil {
		out.SharedPreferences = map[string]json.RawMessage{}
		for k, v := range c.SharedPreferences {
			out.SharedPreferences[k] = slices.Clone(v)
		}
	}
	out.LinkRootOrder = slices.Clone(c.LinkRootOrder)
	out.Settings.KeyboardShortcuts = c.Settings.KeyboardShortcuts.Clone()
	out.HighlightSets = slices.Clone(c.HighlightSets)
	for i := range out.HighlightSets {
		out.HighlightSets[i].Rules = slices.Clone(out.HighlightSets[i].Rules)
	}
	out.Settings.CustomSchemes = slices.Clone(c.Settings.CustomSchemes)
	if c.Settings.TerminalAppearance != nil {
		appearance := *c.Settings.TerminalAppearance
		out.Settings.TerminalAppearance = &appearance
	}
	for i := range out.Settings.CustomSchemes {
		out.Settings.CustomSchemes[i].ANSI = slices.Clone(c.Settings.CustomSchemes[i].ANSI)
	}
	out.Settings.Highlights = slices.Clone(c.Settings.Highlights)
	out.Profiles = slices.Clone(c.Profiles)
	for i := range out.Profiles {
		out.Profiles[i].SSHAuthenticationMethods = slices.Clone(out.Profiles[i].SSHAuthenticationMethods)
		out.Profiles[i].Tunnels = slices.Clone(out.Profiles[i].Tunnels)
		out.Profiles[i].RuntimeProxyRoute = slices.Clone(out.Profiles[i].RuntimeProxyRoute)
		out.Profiles[i].resolvedIdentityFiles = slices.Clone(out.Profiles[i].resolvedIdentityFiles)
	}
	out.Directories = slices.Clone(c.Directories)
	out.Links = slices.Clone(c.Links)
	out.QuickCommands = slices.Clone(c.QuickCommands)
	return out
}
func (c Config) AllDirectories() []string {
	seen := map[string]bool{"Links": true}
	add := func(s string) {
		parts := strings.Split(s, "/")
		for i := range parts {
			v := strings.Join(parts[:i+1], "/")
			if v != "" {
				seen[v] = true
			}
		}
	}
	for _, d := range c.Directories {
		add(d)
	}
	for _, p := range c.Profiles {
		add(p.Group)
	}
	for _, l := range c.Links {
		if l.Folder != "" {
			add("Links/" + l.Folder)
		}
	}
	out := make([]string, 0, len(seen))
	for d := range seen {
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}
func (c *Config) AddDirectory(name string) error {
	d, e := NormalizeDirectory(name)
	if e != nil {
		return e
	}
	if d == "" {
		return fmt.Errorf("请填写目录名称")
	}
	for _, v := range c.AllDirectories() {
		if v == d {
			return fmt.Errorf("同名目录已存在")
		}
	}
	c.Directories = append(c.Directories, d)
	return nil
}
func (c *Config) MoveDirectory(from, to string) error {
	var e error
	from, e = NormalizeDirectory(from)
	if e != nil {
		return e
	}
	to, e = NormalizeDirectory(to)
	if e != nil {
		return e
	}
	if from == "" || from == "Links" || to == "" || Within(to, from) {
		return fmt.Errorf("不能移动根目录、Links 根目录或移入自身子目录")
	}
	if Within(from, "Links") != Within(to, "Links") {
		return fmt.Errorf("快捷引用目录只能在 Links 内移动")
	}
	paths := c.AllDirectories()
	existing := map[string]bool{}
	for _, d := range paths {
		existing[d] = true
	}
	if !existing[from] {
		return fmt.Errorf("源目录已不存在，请重新选择")
	}
	if slash := strings.LastIndexByte(to, '/'); slash >= 0 && !existing[to[:slash]] {
		return fmt.Errorf("目标父目录已不存在，请重新选择")
	}
	for _, d := range c.AllDirectories() {
		if d == to {
			return fmt.Errorf("目标目录已存在，不自动合并")
		}
	}
	move := func(s string) string {
		if Within(s, from) {
			return to + strings.TrimPrefix(s, from)
		}
		return s
	}
	c.Directories = nil
	for _, directory := range paths {
		if directory != "Links" {
			c.Directories = append(c.Directories, move(directory))
		}
	}
	for i := range c.Profiles {
		c.Profiles[i].Group = move(c.Profiles[i].Group)
	}
	for i := range c.Links {
		full := "Links"
		if c.Links[i].Folder != "" {
			full += "/" + c.Links[i].Folder
		}
		c.Links[i].Folder = strings.TrimPrefix(strings.TrimPrefix(move(full), "Links"), "/")
	}
	for i, key := range c.LinkRootOrder {
		if folder, ok := strings.CutPrefix(key, "folder:"); ok {
			moved := move("Links/" + folder)
			c.LinkRootOrder[i] = "folder:" + strings.TrimPrefix(moved, "Links/")
		}
	}
	c.NormalizeLinkRootOrder()
	return nil
}
func (c *Config) RemoveProfiles(ids map[string]bool) {
	profiles := c.Profiles[:0]
	for _, p := range c.Profiles {
		if !ids[p.ID] {
			profiles = append(profiles, p)
		}
	}
	c.Profiles = profiles
	links := c.Links[:0]
	for _, l := range c.Links {
		if !ids[l.ProfileID] {
			links = append(links, l)
		}
	}
	c.Links = links
	c.NormalizeLinkRootOrder()
}
func (c *Config) DeleteDirectory(dir string) error {
	if dir == "" || dir == "Links" {
		return fmt.Errorf("不能删除根目录或 Links 根目录")
	}
	paths := c.AllDirectories()
	found := false
	for _, path := range paths {
		found = found || path == dir
	}
	if !found {
		return fmt.Errorf("目录已不存在，请重新选择")
	}
	ids := map[string]bool{}
	for _, p := range c.Profiles {
		if Within(p.Group, dir) {
			ids[p.ID] = true
		}
	}
	rootOrder := append([]string(nil), c.LinkRootOrder...)
	c.RemoveProfiles(ids)
	c.Directories = nil
	for _, path := range paths {
		if path != "Links" && !Within(path, dir) {
			c.Directories = append(c.Directories, path)
		}
	}
	links := c.Links[:0]
	for _, l := range c.Links {
		if !Within("Links/"+l.Folder, dir) {
			links = append(links, l)
		}
	}
	c.Links = links
	c.LinkRootOrder = rootOrder
	c.NormalizeLinkRootOrder()
	return nil
}
func (c *Config) AddLink(p Profile, folder string) error {
	d, e := NormalizeDirectory(folder)
	if e != nil {
		return e
	}
	for _, l := range c.Links {
		if l.ProfileID == p.ID && l.Folder == d {
			return fmt.Errorf("该目录已存在此会话的快捷引用")
		}
	}
	found := false
	for _, v := range c.Profiles {
		found = found || v.ID == p.ID
	}
	if !found {
		return fmt.Errorf("请先保存会话")
	}
	c.rememberLinkFolder(d)
	c.Links = append(c.Links, SessionLink{ID: NewID(), ProfileID: p.ID, Name: p.Name, Folder: d})
	c.Settings.LinksVisible = true
	c.NormalizeLinkRootOrder()
	return nil
}

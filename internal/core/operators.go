// SPDX-License-Identifier: GPL-3.0-only
package core

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

func NewQuickCommand() QuickCommand {
	return QuickCommand{ID: NewID(), Name: "新命令", Group: "常用", AppendReturn: true}
}
func (q *QuickCommand) UnmarshalJSON(data []byte) error {
	type plain QuickCommand
	value := plain(NewQuickCommand())
	value.ID = ""
	if e := json.Unmarshal(data, &value); e != nil {
		return e
	}
	*q = QuickCommand(value)
	return nil
}
func (q QuickCommand) Validate() error {
	if strings.TrimSpace(q.Name) == "" || len(q.Name) > 1024 || q.Text == "" || len(q.Text) > 1<<20 || strings.ContainsRune(q.Text, 0) {
		return fmt.Errorf("请填写命令名称和内容，内容不能包含空字符或超过 1 MiB")
	}
	if len(q.Group) > 1024 || strings.ContainsFunc(q.Group, unicode.IsControl) {
		return fmt.Errorf("命令分组名称无效")
	}
	return nil
}
func NormalizeInput(text string) string {
	return strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
}

// ManagedInput is for explicit command dispatch, independent of keyboard sync.
func ManagedInput(text string, appendReturn bool) []byte {
	text = strings.ReplaceAll(NormalizeInput(text), "\x1b", "")
	suffix := ""
	if appendReturn && !strings.HasSuffix(text, "\n") {
		suffix = "\r"
	}
	return []byte(strings.ReplaceAll(text, "\n", "\r") + suffix)
}

const StandardHighlightSetID = "F6C9AE44-0C05-450B-AB54-28F8AC88B314"

type HighlightSet struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Rules []HighlightRule `json:"rules"`
}

func StandardHighlightSet() HighlightSet {
	return HighlightSet{ID: StandardHighlightSetID, Name: "错误、警告与主机名", Rules: []HighlightRule{
		{ID: "F6C9AE44-0C05-450B-AB54-28F8AC88B315", Pattern: `\b(error|fatal|failed|panic)\b`, Regex: true, Color: "#FF5F56", Enabled: true},
		{ID: "F6C9AE44-0C05-450B-AB54-28F8AC88B316", Pattern: `\b(warn|warning)\b`, Regex: true, Color: "#FFAA33", Enabled: true},
		{ID: "F6C9AE44-0C05-450B-AB54-28F8AC88B317", Pattern: "${hostname}", Color: "#F5D547", Enabled: true},
	}}
}
func (r HighlightRule) TextColor() string {
	if r.Foreground != "" {
		return r.Foreground
	}
	return r.Color
}
func (r HighlightRule) ForHostname(host string) HighlightRule {
	if r.Regex {
		host = regexp.QuoteMeta(host)
	}
	r.Pattern = strings.ReplaceAll(r.Pattern, "${hostname}", host)
	r.Foreground = r.TextColor()
	return r
}

var highlightColor = regexp.MustCompile(`^#[0-9A-Fa-f]{6}([0-9A-Fa-f]{2})?$`)

func (h HighlightSet) Validate() error {
	if h.ID == "" || strings.TrimSpace(h.Name) == "" || len(h.Name) > 1024 || strings.ContainsFunc(h.Name, unicode.IsControl) {
		return fmt.Errorf("突出显示集名称无效")
	}
	ids := map[string]bool{}
	for _, r := range h.Rules {
		if r.ID == "" || ids[r.ID] || r.Pattern == "" || len(r.Pattern) > 512 {
			return fmt.Errorf("突出显示规则无效：需要唯一 ID 和最多 512 字节的匹配内容")
		}
		ids[r.ID] = true
		color := r.TextColor()
		if !highlightColor.MatchString(color) {
			return fmt.Errorf("文字颜色应为 #RRGGBB")
		}
		if r.Regex {
			if pattern, e := CompilePattern(r.ForHostname("example").Pattern, 0); e != nil {
				return fmt.Errorf("正则表达式无效：%w", e)
			} else {
				pattern.Close()
			}
		}
	}
	return nil
}
func (c Config) ActiveHighlightRules(host string) []HighlightRule {
	for _, set := range c.HighlightSets {
		if set.ID == c.Settings.HighlightSetID {
			result := make([]HighlightRule, 0, len(set.Rules))
			for _, r := range set.Rules[:min(len(set.Rules), 32)] {
				if r.Enabled {
					r = r.ForHostname(host)
					if r.Pattern != "" {
						result = append(result, r)
					}
				}
			}
			return result
		}
	}
	return nil
}

// Preserve the former desktop flat rule list once, without overwriting choices.
func (c *Config) UnmarshalJSON(data []byte) error {
	type plain Config
	value := plain(DefaultConfig())
	if e := json.Unmarshal(data, &value); e != nil {
		return e
	}
	var fields map[string]json.RawMessage
	if e := json.Unmarshal(data, &fields); e != nil {
		return e
	}
	if _, exists := fields["highlightSets"]; !exists && len(value.Settings.Highlights) > 0 {
		rules := value.Settings.Highlights
		for i := range rules {
			if rules[i].ID == "" {
				rules[i].ID = NewID()
			}
			rules[i].Color = rules[i].TextColor()
		}
		set := HighlightSet{ID: NewID(), Name: "原有突出显示规则", Rules: rules}
		value.HighlightSets = append(value.HighlightSets, set)
		value.Settings.HighlightSetID = set.ID
	}
	value.Settings.Highlights = nil
	*c = Config(value)
	return c.MigrateProxyCatalog()
}

var hostnameTitle = regexp.MustCompile(`^\s*\[?[^\s@\[\]:]{1,64}@([A-Za-z0-9][A-Za-z0-9_.-]{0,252})(?:[:\s\]]|$)`)

// Display-only: never used for authentication, connections, or file paths.
func HostnameFromTitle(title string) string {
	if len(title) > 2048 || strings.ContainsFunc(title, unicode.IsControl) {
		return ""
	}
	m := hostnameTitle.FindStringSubmatch(title)
	if len(m) < 2 || m[1] == "." || m[1] == ".." {
		return ""
	}
	return m[1]
}

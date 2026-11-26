// SPDX-License-Identifier: GPL-3.0-only
package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// SharedMerge mirrors OShell 0.2.67: a session is an indivisible unit.
// Simultaneous edits to different fields of one session still need a choice.
// Other configuration sections are also atomic, preventing an accidental
// mixture of credentials and connection identities from different revisions.
type SharedMergeChoice string

const (
	SharedKeepLocal  SharedMergeChoice = "local"
	SharedKeepRemote SharedMergeChoice = "remote"
)

type SharedMergeConflict struct {
	Key               string `json:"key"`
	Title             string `json:"title"`
	LocalDescription  string `json:"localDescription"`
	RemoteDescription string `json:"remoteDescription"`
}
type SharedMerge struct {
	Conflicts                               []SharedMergeConflict
	merged, local, remote                   map[string]json.RawMessage
	profiles, localProfiles, remoteProfiles map[string]json.RawMessage
	order                                   []string
}

// SameStoredProperties compares editable profile data across a sync round trip.
func (p Profile) SameStoredProperties(other Profile) bool {
	left, e1 := json.Marshal(PortableProfile{Definition: p})
	right, e2 := json.Marshal(PortableProfile{Definition: other})
	return e1 == nil && e2 == nil && equalSharedJSON(left, right)
}

func equalSharedJSON(a, b json.RawMessage) bool {
	var x, y any
	if len(a) > 0 && json.Unmarshal(a, &x) != nil {
		return false
	}
	if len(b) > 0 && json.Unmarshal(b, &y) != nil {
		return false
	}
	normalizeSharedIDs(x)
	normalizeSharedIDs(y)
	left, _ := json.Marshal(x)
	right, _ := json.Marshal(y)
	return bytes.Equal(left, right)
}

// Swift UUID encoding normalizes case. Match that semantic equality without
// changing free-form names, text, or encrypted data.
func normalizeSharedIDs(value any) {
	switch v := value.(type) {
	case map[string]any:
		for key, item := range v {
			switch key {
			case "id", "profileID", "proxyID", "upstreamID", "highlightSetID":
				if text, ok := item.(string); ok && uuidText.MatchString(text) {
					v[key] = strings.ToUpper(text)
				}
			}
			normalizeSharedIDs(item)
		}
	case []any:
		for _, item := range v {
			normalizeSharedIDs(item)
		}
	}
}

func describeSharedJSON(value json.RawMessage) string {
	if len(value) == 0 || string(value) == "null" {
		return "已删除 / 未设置"
	}
	var decoded any
	if json.Unmarshal(value, &decoded) != nil {
		return "值已改变"
	}
	var redact func(any) any
	redact = func(v any) any {
		switch x := v.(type) {
		case map[string]any:
			for k, v := range x {
				key := strings.ToLower(k)
				private := false
				for _, token := range []string{"password", "ciphertext", "salt", "secret", "token"} {
					if strings.Contains(key, token) {
						private = true
						break
					}
				}
				if private {
					x[k] = "已设置（内容不展示）"
				} else {
					x[k] = redact(v)
				}
			}
		case []any:
			for i := range x {
				x[i] = redact(x[i])
			}
		}
		return v
	}
	out, e := json.MarshalIndent(redact(decoded), "", "  ")
	if e != nil {
		return "值已改变"
	}
	return string(out)
}
func sharedFields(d SharedDocument) (map[string]json.RawMessage, error) {
	normalized, err := d.canonicalProtocol()
	if err != nil {
		return nil, err
	}
	d = normalized
	data, e := json.Marshal(d)
	if e != nil {
		return nil, e
	}
	var fields map[string]json.RawMessage
	e = json.Unmarshal(data, &fields)
	return fields, e
}
func sharedProfiles(d SharedDocument) (map[string]json.RawMessage, []string, error) {
	values := map[string]json.RawMessage{}
	order := []string{}
	for _, profile := range d.Profiles {
		id := strings.ToUpper(profile.Definition.ID)
		if !uuidText.MatchString(id) {
			return nil, nil, fmt.Errorf("配置含有无效会话 ID")
		}
		if _, exists := values[id]; exists {
			return nil, nil, fmt.Errorf("配置含有重复会话 ID，无法安全合并")
		}
		profile.Definition.ID = id
		encoded, e := json.Marshal(profile)
		if e != nil {
			return nil, nil, e
		}
		values[id] = encoded
		order = append(order, id)
	}
	return values, order, nil
}
func NewSharedMerge(base, local, remote SharedDocument) (*SharedMerge, error) {
	b, e := sharedFields(base)
	if e != nil {
		return nil, e
	}
	l, e := sharedFields(local)
	if e != nil {
		return nil, e
	}
	r, e := sharedFields(remote)
	if e != nil {
		return nil, e
	}
	if !equalSharedJSON(b["masterPasswordVerifier"], l["masterPasswordVerifier"]) || !equalSharedJSON(b["masterPasswordVerifier"], r["masterPasswordVerifier"]) {
		return nil, fmt.Errorf("主密码保护已改变，请重新载入或重启解锁后再编辑")
	}
	bp, bo, e := sharedProfiles(base)
	if e != nil {
		return nil, e
	}
	lp, lo, e := sharedProfiles(local)
	if e != nil {
		return nil, e
	}
	rp, ro, e := sharedProfiles(remote)
	if e != nil {
		return nil, e
	}
	m := &SharedMerge{merged: map[string]json.RawMessage{}, local: l, remote: r, profiles: map[string]json.RawMessage{}, localProfiles: lp, remoteProfiles: rp}
	labels := map[string]string{"preferences": "应用设置", "directories": "会话目录", "quickCommands": "快速命令", "highlightSets": "突出显示集", "sessionLinks": "快捷引用", "sessionDefaults": "默认会话属性", "proxies": "共享代理"}
	keys := map[string]bool{}
	for _, fields := range []map[string]json.RawMessage{b, l, r} {
		for key := range fields {
			if key != "profiles" {
				keys[key] = true
			}
		}
	}
	sorted := []string{}
	for key := range keys {
		sorted = append(sorted, key)
	}
	sort.Strings(sorted)
	merge := func(key, title string, b, l, r json.RawMessage) (json.RawMessage, bool) {
		if equalSharedJSON(l, r) || equalSharedJSON(r, b) {
			return l, true
		}
		if equalSharedJSON(l, b) {
			return r, true
		}
		m.Conflicts = append(m.Conflicts, SharedMergeConflict{key, title, describeSharedJSON(l), describeSharedJSON(r)})
		return nil, false
	}
	for _, key := range sorted {
		title := labels[key]
		if title == "" {
			title = key
		}
		if v, ok := merge(key, title, b[key], l[key], r[key]); ok {
			m.merged[key] = v
		}
	}
	seen := map[string]bool{}
	for _, order := range [][]string{ro, lo, bo} {
		for _, id := range order {
			if !seen[id] {
				seen[id] = true
				m.order = append(m.order, id)
			}
		}
	}
	for _, id := range m.order {
		var name struct {
			Name string `json:"name"`
		}
		for _, value := range []json.RawMessage{lp[id], rp[id], bp[id]} {
			if len(value) > 0 {
				_ = json.Unmarshal(value, &name)
				break
			}
		}
		if v, ok := merge("profile:"+id, name.Name, bp[id], lp[id], rp[id]); ok {
			m.profiles[id] = v
		}
	}
	return m, nil
}
func (m *SharedMerge) Resolve(choices map[string]SharedMergeChoice, master []byte) (SharedDocument, error) {
	var document SharedDocument
	fields := map[string]json.RawMessage{}
	profiles := map[string]json.RawMessage{}
	for k, v := range m.merged {
		fields[k] = v
	}
	for k, v := range m.profiles {
		profiles[k] = v
	}
	for _, conflict := range m.Conflicts {
		choice := choices[conflict.Key]
		if choice != SharedKeepLocal && choice != SharedKeepRemote {
			return document, fmt.Errorf("请为每个冲突项目选择要保留的版本")
		}
		source, items := m.local, m.localProfiles
		if choice == SharedKeepRemote {
			source, items = m.remote, m.remoteProfiles
		}
		if id, ok := strings.CutPrefix(conflict.Key, "profile:"); ok {
			profiles[id] = items[id]
		} else {
			fields[conflict.Key] = source[conflict.Key]
		}
	}
	selected := []json.RawMessage{}
	for _, id := range m.order {
		if value := profiles[id]; len(value) > 0 && string(value) != "null" {
			selected = append(selected, value)
		}
	}
	fields["profiles"], _ = json.Marshal(selected)
	for k, v := range fields {
		if len(v) == 0 {
			delete(fields, k)
		}
	}
	data, e := json.Marshal(fields)
	if e != nil {
		return document, e
	}
	if e = json.Unmarshal(data, &document); e != nil {
		return document, e
	}
	// Fail closed on a cross-section reference conflict rather than dropping it.
	if e = document.Validate(master); e != nil {
		return document, e
	}
	return document, nil
}

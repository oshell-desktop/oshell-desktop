// SPDX-License-Identifier: GPL-3.0-only
package core

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"unicode"
)

type ProxyProfile struct {
	Name       string        `json:"name"`
	Settings   ProxySettings `json:"settings"`
	UpstreamID string        `json:"upstreamID,omitempty"`
}

func (p ProxyProfile) ID() string { return p.Settings.ID }
func NewProxyProfile() ProxyProfile {
	return ProxyProfile{Name: "新代理", Settings: ProxySettings{ID: NewID(), Kind: "socks5", Port: 1080, SSHAuthentication: "automatic"}}
}
func ProxyRoute(id string, profiles []ProxyProfile) ([]ProxyProfile, error) {
	entries := map[string]ProxyProfile{}
	for _, p := range profiles {
		key := strings.ToUpper(p.ID())
		if _, exists := entries[key]; exists {
			return nil, fmt.Errorf("代理 ID 重复")
		}
		entries[key] = p
	}
	var result []ProxyProfile
	seen := map[string]bool{}
	for id != "" {
		key := strings.ToUpper(id)
		if seen[key] {
			return nil, fmt.Errorf("代理链存在循环引用")
		}
		seen[key] = true
		if len(result) >= 8 {
			return nil, fmt.Errorf("代理链最多支持 8 级")
		}
		entry, ok := entries[key]
		if !ok {
			return nil, fmt.Errorf("代理或上级代理不存在，请重新选择")
		}
		result = append(result, entry)
		id = entry.UpstreamID
	}
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return result, nil
}
func ValidateProxyCatalog(proxies []ProxyProfile, sessions []Profile) error {
	if len(proxies) > 2000 {
		return fmt.Errorf("代理数量超过限制")
	}
	ids := map[string]bool{}
	for _, p := range sessions {
		ids[strings.ToUpper(p.ID)] = true
	}
	for _, p := range proxies {
		if strings.TrimSpace(p.Name) == "" || len(p.Name) > 256 || strings.ContainsFunc(p.Name, unicode.IsControl) || !uuidText.MatchString(p.ID()) || p.Settings.Kind == "" || p.Settings.Kind == "none" || ids[strings.ToUpper(p.ID())] {
			return fmt.Errorf("代理名称、类型或 ID 无效")
		}
		if e := p.Settings.Validate(); e != nil {
			return e
		}
		if _, e := ProxyRoute(p.ID(), proxies); e != nil {
			return e
		}
	}
	for _, p := range sessions {
		if p.ProxyID != "" {
			if !uuidText.MatchString(p.ProxyID) || (p.Kind != "ssh" && p.Kind != "sftp") || (p.Proxy.Kind != "" && p.Proxy.Kind != "none") {
				return fmt.Errorf("共享代理仅用于 SSH/SFTP，不能同时设置独立代理")
			}
			if _, e := ProxyRoute(p.ProxyID, proxies); e != nil {
				return e
			}
		}
	}
	return nil
}
func (c *Config) MigrateProxyCatalog() error {
	for i := range c.Profiles {
		p := &c.Profiles[i]
		if p.ProxyID != "" || p.Proxy.Kind == "" || p.Proxy.Kind == "none" {
			continue
		}
		if p.Proxy.ID == "" {
			p.Proxy.ID = NewID()
		}
		found := false
		for _, v := range c.Proxies {
			if strings.EqualFold(v.ID(), p.Proxy.ID) {
				if !reflect.DeepEqual(v.Settings, p.Proxy) {
					return fmt.Errorf("旧代理 ID 冲突，未迁移")
				}
				found = true
				break
			}
		}
		if !found {
			c.Proxies = append(c.Proxies, ProxyProfile{Name: p.Proxy.Kind + " · " + p.Proxy.Host, Settings: p.Proxy})
		}
		p.ProxyID = p.Proxy.ID
		p.Proxy = ProxySettings{ID: p.ProxyID, Kind: "none", Port: 1080, SSHAuthentication: "automatic"}
	}
	return ValidateProxyCatalog(c.Proxies, c.Profiles)
}
func (c Config) ConnectionProfile(p Profile) (Profile, error) {
	p.RuntimeProxyRoute = nil
	if p.ProxyID != "" {
		route, e := ProxyRoute(p.ProxyID, c.Proxies)
		if e != nil {
			return p, e
		}
		for _, proxy := range route {
			p.RuntimeProxyRoute = append(p.RuntimeProxyRoute, proxy.Settings)
		}
	}
	return p, nil
}
func RequiredProxies(sessions []Profile, proxies []ProxyProfile) ([]ProxyProfile, error) {
	ids := map[string]bool{}
	for _, p := range sessions {
		if p.ProxyID != "" {
			route, e := ProxyRoute(p.ProxyID, proxies)
			if e != nil {
				return nil, e
			}
			for _, node := range route {
				ids[strings.ToUpper(node.ID())] = true
			}
		}
	}
	var out []ProxyProfile
	for _, p := range proxies {
		if ids[strings.ToUpper(p.ID())] {
			out = append(out, p)
		}
	}
	return out, nil
}
func (p Profile) MarshalJSON() ([]byte, error) {
	if err := p.validateSSHAuthenticationMethods(); err != nil {
		return nil, err
	}
	type plain Profile
	data, e := json.Marshal(plain(p))
	if e != nil {
		return nil, e
	}
	if p.ProxyID == "" {
		return data, nil
	}
	var fields map[string]json.RawMessage
	if e = json.Unmarshal(data, &fields); e != nil {
		return nil, e
	}
	id := p.Proxy.ID
	if id == "" {
		id = p.ProxyID
	}
	fields["proxy"], e = json.Marshal(struct {
		ID   string `json:"id"`
		Kind string `json:"kind"`
	}{id, "shared-reference"})
	if e != nil {
		return nil, e
	}
	return json.Marshal(fields)
}

type PortableProxy struct {
	Definition ProxyProfile
	Password   *LegacyPassword
}

func (p PortableProxy) MarshalJSON() ([]byte, error) {
	if p.Definition.Settings.SSHAuthentication == "" {
		p.Definition.Settings.SSHAuthentication = "automatic"
	}
	data, e := json.Marshal(p.Definition)
	if e != nil {
		return nil, e
	}
	if p.Password == nil {
		return data, nil
	}
	var root map[string]json.RawMessage
	json.Unmarshal(data, &root)
	var settings map[string]json.RawMessage
	json.Unmarshal(root["settings"], &settings)
	settings["encryptedPassword"], e = json.Marshal(p.Password)
	if e != nil {
		return nil, e
	}
	root["settings"], e = json.Marshal(settings)
	if e != nil {
		return nil, e
	}
	return json.Marshal(root)
}
func (p *PortableProxy) UnmarshalJSON(data []byte) error {
	if e := json.Unmarshal(data, &p.Definition); e != nil {
		return e
	}
	var extra struct {
		Settings struct{ EncryptedPassword *LegacyPassword }
	}
	if e := json.Unmarshal(data, &extra); e != nil {
		return e
	}
	p.Password = extra.Settings.EncryptedPassword
	if p.Password != nil {
		return validatePasswordEnvelope(*p.Password)
	}
	return nil
}

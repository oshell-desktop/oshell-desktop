package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// GlobalProxySettings is local connection policy, never part of shared Config.
type GlobalProxySettings struct {
	Version int    `json:"version"`
	Enabled bool   `json:"enabled"`
	ProxyID string `json:"proxyID,omitempty"`
}

func LoadGlobalProxy(dir string) (GlobalProxySettings, error) {
	s := GlobalProxySettings{Version: 1}
	data, err := ReadBounded(filepath.Join(dir, "global-proxy.json"))
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	var disk struct {
		Version int    `json:"version"`
		Enabled *bool  `json:"enabled"`
		ProxyID string `json:"proxyID,omitempty"`
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err = d.Decode(&disk); err != nil {
		return s, fmt.Errorf("全局代理设置无法读取：%w", err)
	}
	if d.Decode(new(any)) != io.EOF || disk.Version != 1 || disk.Enabled == nil || disk.ProxyID != "" && !uuidText.MatchString(disk.ProxyID) {
		return s, fmt.Errorf("全局代理设置格式或版本无效")
	}
	return GlobalProxySettings{Version: disk.Version, Enabled: *disk.Enabled, ProxyID: disk.ProxyID}, nil
}

func (s GlobalProxySettings) Save(dir string) error {
	if s.Version != 1 || s.ProxyID != "" && !uuidText.MatchString(s.ProxyID) {
		return fmt.Errorf("全局代理设置格式或版本无效")
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return AtomicWrite(filepath.Join(dir, "global-proxy.json"), data)
}

func (s GlobalProxySettings) ConnectionProfile(c Config, p Profile, vault *Vault) (Profile, error) {
	if !s.Enabled || p.Kind != "ssh" && p.Kind != "sftp" {
		return c.ConnectionProfile(p)
	}
	if s.ProxyID == "" {
		return p, fmt.Errorf("全局代理已启用，请先选择代理")
	}
	route, err := ProxyRoute(s.ProxyID, c.Proxies)
	if err != nil {
		return p, fmt.Errorf("全局代理不可用：%w", err)
	}
	p, err = c.ConnectionProfile(p)
	if err != nil {
		return p, err
	}
	session := p.RuntimeProxyRoute
	if len(session) == 0 {
		if p.Proxy.Kind != "" && p.Proxy.Kind != "none" {
			session = []ProxySettings{p.Proxy}
		} else if p.JumpHost != "" && !strings.EqualFold(p.JumpHost, "none") {
			tokens := strings.Split(p.JumpHost, ",")
			if len(tokens) > 8 {
				return p, fmt.Errorf("代理链最多支持 8 级")
			}
			for _, token := range tokens {
				hop, err := parseConfiguredJump(token)
				if err != nil {
					return p, err
				}
				if hop.Port == 0 || hop.Username == "" {
					resolved, err := ResolveSSH(context.Background(), hop)
					if err != nil {
						return p, err
					}
					hop.Port, hop.Username = resolved.Profile.Port, resolved.Profile.Username
				}
				session = append(session, ProxySettings{ID: hop.ID, Kind: "jump", Host: hop.Host, Port: hop.Port, Username: hop.Username, SSHAuthentication: "automatic"})
			}
		}
	}
	prefix := make([]ProxySettings, 0, len(route))
	for _, hop := range route {
		prefix = append(prefix, hop.Settings)
	}
	sameHop := func(a, b ProxySettings) bool {
		if strings.EqualFold(a.ID, b.ID) {
			return true
		}
		if vault != nil && (vault.HasStoredCredential(a.ID) || vault.HasStoredCredential(b.ID)) {
			return false
		}
		a.ID = b.ID
		return a == b
	}
	// Only a suffix of the prefix can overlap the start of the session route.
	// Removing a repeated hop anywhere else would change its upstream connection.
	overlap := min(len(prefix), len(session))
	for ; overlap > 0; overlap-- {
		matches := true
		for i, a := range prefix[len(prefix)-overlap:] {
			if !sameHop(a, session[i]) {
				matches = false
				break
			}
		}
		if matches {
			break
		}
	}
	for i, a := range prefix[len(prefix)-overlap:] {
		b := session[i]
		a.ID = b.ID
		if a != b {
			return p, fmt.Errorf("同一代理 ID 的连接或认证配置不一致")
		}
	}
	combined := append(prefix, session[overlap:]...)
	if len(combined) > 8 {
		return p, fmt.Errorf("组合后的代理链最多支持 8 级")
	}
	for i, hop := range combined {
		if hop.Kind == "" || hop.Kind == "none" {
			return p, fmt.Errorf("代理链包含无效代理")
		}
		if err := hop.Validate(); err != nil {
			return p, fmt.Errorf("全局代理不可用：%w", err)
		}
		for _, previous := range combined[:i] {
			if sameHop(previous, hop) {
				return p, fmt.Errorf("代理链存在重复或顺序冲突，请调整代理链")
			}
		}
	}
	p.RuntimeProxyRoute = combined
	return p, nil
}

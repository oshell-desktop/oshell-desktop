package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

func (s GlobalProxySettings) ConnectionProfile(c Config, p Profile) (Profile, error) {
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
	p.RuntimeProxyRoute = nil
	for _, hop := range route {
		if hop.Settings.Kind == "" || hop.Settings.Kind == "none" {
			return p, fmt.Errorf("全局代理链包含无效代理")
		}
		if err := hop.Settings.Validate(); err != nil {
			return p, fmt.Errorf("全局代理不可用：%w", err)
		}
		p.RuntimeProxyRoute = append(p.RuntimeProxyRoute, hop.Settings)
	}
	return p, nil
}

package core

import (
	"fmt"
	"strings"
)

// Device-local policy; these fields never enter the shared-vault contract.
type SyncContentSelection struct {
	Sessions        bool `json:"sessions"`
	ClientSettings  bool `json:"clientSettings"`
	QuickCommands   bool `json:"quickCommands"`
	SessionDefaults bool `json:"sessionDefaults"`
}

type SyncPolicy struct {
	Version       int                  `json:"version"`
	Automatic     bool                 `json:"automatic"`
	OnStartup     bool                 `json:"onStartup"`
	AfterChanges  bool                 `json:"afterChanges"`
	CheckInterval int                  `json:"checkInterval"`
	ChangeDelay   int                  `json:"changeDelay"`
	RetryInterval int                  `json:"retryInterval"`
	Content       SyncContentSelection `json:"content"`
}

func DefaultSyncPolicy() SyncPolicy {
	return SyncPolicy{Version: 1, Automatic: true, OnStartup: true, AfterChanges: true, CheckInterval: 60, ChangeDelay: 2, RetryInterval: 60, Content: SyncContentSelection{Sessions: true}}
}

func (p SyncPolicy) Effective() SyncPolicy {
	if p == (SyncPolicy{}) {
		return DefaultSyncPolicy()
	}
	return p
}

func (p SyncPolicy) Validate() error {
	p = p.Effective()
	if p.Version != 1 {
		return fmt.Errorf("同步策略版本不受支持，请升级客户端")
	}
	if p.Content == (SyncContentSelection{}) {
		return fmt.Errorf("请至少选择一类同步内容")
	}
	interval := func(v int) bool { return v == 0 || v >= 15 && v <= 86400 }
	if !interval(p.CheckInterval) || !interval(p.RetryInterval) || p.ChangeDelay < 1 || p.ChangeDelay > 300 {
		return fmt.Errorf("检查和重试间隔须为 15–86400 秒（0 表示关闭），修改后等待须为 1–300 秒")
	}
	return nil
}

func (s SyncContentSelection) Summary() string {
	var names []string
	for _, item := range []struct {
		enabled bool
		name    string
	}{{s.Sessions, "会话资料"}, {s.ClientSettings, "客户端设置与高亮规则"}, {s.QuickCommands, "快速命令"}, {s.SessionDefaults, "默认会话属性"}} {
		if item.enabled {
			names = append(names, item.name)
		}
	}
	return strings.Join(names, "、")
}

func syncSelection(values []SyncContentSelection) SyncContentSelection {
	if len(values) > 0 {
		return values[0]
	}
	return DefaultSyncPolicy().Content
}

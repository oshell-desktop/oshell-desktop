package desktop

import (
	"context"
	"errors"
	"fmt"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"strings"
	"time"
)

func (a *App) syncPolicySettings(c *ui.Context) {
	f := &a.features.sharedSync
	f.options.Policy = f.options.Policy.Effective()
	p := &f.options.Policy
	ui.Text(c, "同步策略（仅本机）").Bold()
	ui.Checkbox(c, &p.Automatic, "自动同步")
	ui.Checkbox(c, &p.OnStartup, "启动或解锁后检查").Disabled(!p.Automatic)
	ui.Checkbox(c, &p.AfterChanges, "修改后同步").Disabled(!p.Automatic)
	for _, field := range []struct {
		label     string
		value     *int
		low, high float64
	}{
		{"检查间隔（秒，0 关闭）", &p.CheckInterval, 0, 86400},
		{"修改后等待（秒）", &p.ChangeDelay, 1, 300},
		{"失败重试间隔（秒，0 关闭）", &p.RetryInterval, 0, 86400},
	} {
		ui.Field(c, field.label, func() {
			v := float64(*field.value)
			if ui.NumberInput(c.Key(field.label), &v, field.low, field.high, 1).Label(field.label).Disabled(!p.Automatic).Changed() {
				*field.value = int(v)
			}
		})
	}
	ui.Text(c, "同步内容").Bold()
	ui.Checkbox(c, &p.Content.Sessions, "会话资料（会话、目录、引用、代理和密码）")
	ui.Checkbox(c, &p.Content.ClientSettings, "客户端设置与高亮规则")
	ui.Checkbox(c, &p.Content.QuickCommands, "快速命令")
	ui.Checkbox(c, &p.Content.SessionDefaults, "默认会话属性")
	ui.Text(c, "未选类别保留本机和远端原值。改变范围后重新确认已有数据；更新来源、安全决定及平台专属设置始终留在本机。").Wrap().TextColor(c.Theme().TextMuted)
}

func syncPolicySummary(p core.SyncPolicy) string {
	p = p.Effective()
	if !p.Automatic {
		return "仅手动同步 · " + p.Content.Summary()
	}
	parts := []string{"自动同步"}
	if p.OnStartup {
		parts = append(parts, "启动时检查")
	}
	if p.CheckInterval > 0 {
		parts = append(parts, fmt.Sprintf("每 %d 秒检查", p.CheckInterval))
	}
	if p.AfterChanges {
		parts = append(parts, fmt.Sprintf("修改后 %d 秒同步", p.ChangeDelay))
	}
	if p.RetryInterval > 0 {
		parts = append(parts, fmt.Sprintf("失败后 %d 秒重试", p.RetryInterval))
	} else {
		parts = append(parts, "失败后暂停，等待手动重试")
	}
	return strings.Join(parts, " · ") + "\n内容：" + p.Content.Summary()
}

func (h *applicationHub) scheduleStartupSync() {
	windows := h.configurationWindows()
	if len(windows) == 0 {
		h.scheduleSharedSync(time.Minute)
		return
	}
	o, err := windows[0].loadSyncOptions()
	if err != nil {
		return
	}
	p := o.Policy.Effective()
	diagnostics := core.SyncDiagnostics{Directory: windows[0].store.Dir, Target: o.Target(), Backend: o.Backend}
	if doc, err := diagnostics.Snapshot(); err == nil {
		h.syncAutomaticFailed = diagnostics.SummaryForTarget(doc).Failure != nil
	}
	if h.syncAutomaticFailed {
		h.armSharedSync(time.Duration(p.RetryInterval) * time.Second)
		return
	}
	if p.Automatic && p.OnStartup {
		h.armSharedSync(2 * time.Second)
	} else {
		h.scheduleSharedSync(time.Minute)
	}
}

func (a *App) scheduleSyncResult(err error) {
	if a.hub == nil {
		return
	}
	o, e := a.loadSyncOptions()
	if e != nil {
		return
	}
	p := o.Policy.Effective()
	delay := p.CheckInterval
	if !errors.Is(err, context.Canceled) {
		a.hub.syncAutomaticFailed = err != nil && !errors.Is(err, core.ErrSyncDeferred)
	}
	if a.hub.syncAutomaticFailed {
		delay = p.RetryInterval
	}
	a.hub.armSharedSync(time.Duration(delay) * time.Second)
}

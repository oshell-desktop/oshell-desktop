//go:build darwin

package desktop

import (
	"context"
	"fmt"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"os"
	"oshell-desktop/internal/core"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var showUpdateConfirmation = mygo.Dialog.Message

type updateController struct {
	pendingInstall    bool
	confirming        bool
	installing        bool
	scheduledSource   string
	nextCheck         time.Time
	busy              bool
	status, source    string
	downloaded, total int64
	cache             *core.UpdateMetadataCache
	candidate         *core.VerifiedUpdate
	download          *core.VerifiedDownload
	prepared          *core.PreparedUpdate
	cancel            context.CancelFunc
	timer             *time.Timer
}

func (a *App) updateState() *updateController {
	if a.hub != nil {
		return &a.hub.updates
	}
	return &a.features.updates
}
func currentUpdateBundle() (string, core.AppBundleInfo, error) {
	executable, e := os.Executable()
	if e != nil {
		return "", core.AppBundleInfo{}, e
	}
	executable, e = filepath.EvalSymlinks(executable)
	if e != nil {
		return "", core.AppBundleInfo{}, e
	}
	target := filepath.Dir(filepath.Dir(filepath.Dir(executable)))
	if !strings.HasSuffix(target, ".app") {
		return "", core.AppBundleInfo{}, fmt.Errorf("请使用完整 macOS 应用检查更新")
	}
	info, e := core.ReadAppBundleInfo(target)
	return target, info, e
}
func (a *App) prepareUpdateSettings() {
	if a.features.settingsDraft.UpdateRepository == "" {
		if _, info, e := currentUpdateBundle(); e == nil {
			a.features.settingsDraft.UpdateRepository = info.UpdateRepository
		}
	}
}
func (a *App) validateUpdateSettings() error {
	s := &a.features.settingsDraft
	repository := strings.TrimSpace(s.UpdateRepository)
	if repository == "" {
		if s.AutomaticUpdateChecks {
			return fmt.Errorf("请先填写更新仓库，或关闭自动检查")
		}
	} else {
		source, e := core.ParseUpdateSource(repository)
		if e != nil {
			return e
		}
		repository = source.Repository
	}
	state := a.updateState()
	if (state.busy || state.prepared != nil) && repository != a.config.Settings.UpdateRepository {
		return fmt.Errorf("更新正在进行，请完成或取消后再修改仓库")
	}
	s.UpdateRepository = repository
	return nil
}
func (a *App) updateSettingsView(c *ui.Context) {
	state := a.updateState()
	settings := &a.features.settingsDraft
	ui.Text(c, "GitHub Pages 更新").FontSize(18).Bold()
	version := mygo.App.Version()
	if version == "" {
		version = "开发构建"
	}
	ui.Text(c, "当前版本："+version+" · macOS / "+runtime.GOARCH).TextColor(c.Theme().TextMuted)
	ui.TextInput(c, &settings.UpdateRepository).Label("更新仓库").Placeholder("owner/repo 或 https://github.com/owner/repo")
	ui.Checkbox(c, &settings.AutomaticUpdateChecks, "每天自动检查更新（安装前仍需确认）")
	ui.Text(c, "从对应仓库的 GitHub Pages 获取签名更新信息，无需登录或 Token。留空则不检查更新。安装前确认关闭活动会话，完成后重新启动；配置和保存的密码会保留。").Wrap()
	ui.Row(c).Gap(8).Children(func() {
		ui.Button(c, "检查更新").Disabled(state.busy || state.confirming).OnClick(func() {
			if settings.UpdateRepository != a.config.Settings.UpdateRepository {
				state.status = "请先应用更新设置"
				return
			}
			a.checkForUpdates(false)
		})
		if state.candidate != nil && state.prepared == nil {
			ui.Button(c, "下载更新").Disabled(state.busy || state.confirming).OnClick(a.downloadUpdate)
		}
		if state.prepared != nil {
			ui.PrimaryButton(c, "安装并重新启动…").Disabled(state.busy || state.confirming).OnClick(a.installUpdate)
		}
		if state.cancel != nil {
			ui.Button(c, "取消").OnClick(state.cancel)
		}
		if state.prepared != nil && !state.busy && !state.confirming {
			ui.Button(c, "丢弃已下载更新").OnClick(func() {
				state.pendingInstall = false
				state.prepared.Discard()
				state.prepared = nil
				if state.download != nil {
					state.download.Remove()
					state.download = nil
				}
				state.status = "已丢弃更新"
				a.refreshMenu()
			})
		}
	})
	if state.status != "" {
		ui.Text(c, state.status).Wrap()
	}
	if state.total > 0 && state.busy {
		ui.Text(c, fmt.Sprintf("%d / %d KiB", state.downloaded/1024, state.total/1024))
	}
	if state.candidate != nil {
		ui.Text(c, "新版本："+state.candidate.Manifest.Version).Bold()
		ui.Text(c, state.candidate.Manifest.Notes).Wrap()
	}
}
func (a *App) updateMain(fn func()) {
	if mygo.App.IsReady() {
		mygo.RunOnMain(func() { fn(); a.invalidateSecurityWindows() })
	} else {
		a.post(fn)
	}
}
func (a *App) checkForUpdates(automatic bool) {
	state := a.updateState()
	if state.busy || state.confirming || state.prepared != nil || a.passwordLocked() {
		return
	}
	_, info, e := currentUpdateBundle()
	if e != nil {
		state.status = e.Error()
		return
	}
	repository := a.config.Settings.UpdateRepository
	if repository == "" {
		repository = info.UpdateRepository
	}
	if repository == "" {
		state.status = "请先配置更新仓库"
		return
	}
	source, e := core.ParseUpdateSource(repository)
	if e != nil {
		state.status = e.Error()
		return
	}
	key := core.UpdatePublicKey
	if key == "" {
		key = info.UpdateKey
	}
	if _, e = core.ParseUpdatePublicKey(key); e != nil {
		state.status = e.Error()
		return
	}
	build := core.UpdateBuild
	if build == "" {
		if _, e := strconv.ParseUint(info.Build, 10, 64); e == nil {
			build = info.Build
		}
	}
	if state.source != source.Repository || state.cache == nil {
		state.source = source.Repository
		state.cache = &core.UpdateMetadataCache{}
		state.candidate = nil
	}
	state.busy = true
	state.status = "正在检查签名更新…"
	ctx, cancel := context.WithCancel(context.Background())
	state.cancel = cancel
	cache := state.cache
	go func() {
		candidate, e := core.CheckSignedUpdate(ctx, cache, source, core.CurrentMacUpdateTarget(), info.Identifier, info.Version, build, key)
		cancel()
		a.updateMain(func() {
			state.busy = false
			state.cancel = nil
			if e != nil {
				state.status = e.Error()
				return
			}
			state.candidate = candidate
			if candidate == nil {
				state.status = "当前已是最新版本"
			} else {
				state.status = "发现可用更新 " + candidate.Manifest.Version
				if !automatic {
					a.confirmAvailableUpdate()
				}
				if automatic {
					a.status = state.status
				}
			}
		})
	}()
}
func (a *App) updateSourceStillCurrent() bool {
	state := a.updateState()
	repository := a.config.Settings.UpdateRepository
	if repository == "" {
		if _, info, e := currentUpdateBundle(); e == nil {
			repository = info.UpdateRepository
		}
	}
	source, e := core.ParseUpdateSource(repository)
	return e == nil && strings.EqualFold(source.Repository, state.source)
}
func (a *App) downloadUpdate() {
	state := a.updateState()
	if !a.updateSourceStillCurrent() {
		state.status = "更新仓库已变化，请重新检查"
		return
	}
	if state.busy || state.candidate == nil || a.passwordLocked() {
		return
	}
	target, _, e := currentUpdateBundle()
	if e != nil {
		state.status = e.Error()
		return
	}
	state.busy = true
	state.status = "正在下载并校验更新…"
	state.downloaded = 0
	state.total = state.candidate.Archive.Size
	ctx, cancel := context.WithCancel(context.Background())
	state.cancel = cancel
	candidate := state.candidate
	go func() {
		download, e := candidate.Download(ctx, filepath.Join(a.store.Dir, "update-downloads"), func(n, total int64) { a.updateMain(func() { state.downloaded = n; state.total = total }) })
		var prepared *core.PreparedUpdate
		if e == nil {
			a.updateMain(func() { state.status = "正在验证并准备应用…" })
			prepared, e = download.PrepareMacOS(ctx, target)
		}
		if e != nil && download != nil {
			download.Remove()
			download = nil
		}
		cancel()
		a.updateMain(func() {
			state.busy = false
			state.cancel = nil
			if e != nil {
				state.status = e.Error()
				return
			}
			state.download = download
			state.prepared = prepared
			a.refreshMenu()
			state.status = "更新已准备好，安装前会确认关闭活动会话"
		})
	}()
}
func (a *App) installUpdate() {
	state := a.updateState()
	if !a.updateSourceStillCurrent() {
		state.status = "更新仓库已变化，请重新检查"
		return
	}
	if state.busy || state.confirming || state.prepared == nil || a.passwordLocked() {
		return
	}
	if a.configurationBusy() {
		state.status = "请等待配置操作完成后再安装"
		return
	}
	if a.updateDialogsOpen() {
		state.pendingInstall = true
		state.status = "等待其他对话框关闭后继续安装"
		return
	}
	count := a.activeConnections()
	if a.hub != nil {
		count = a.hub.connectionCount()
	}
	state.pendingInstall = false
	if count == 0 {
		a.commitUpdate()
		return
	}
	state.confirming = true
	prepared := state.prepared
	go func() {
		result, e := showUpdateConfirmation(mygo.MessageOptions{Parent: a.preferencesWindow, Type: mygo.MessageQuestion, Title: "安装更新", Message: fmt.Sprintf("关闭 %d 个活动连接并安装更新？", count), Detail: "文件传输将停止。会话配置和已保存密码会保留，安装后自动重新启动。", Buttons: []string{"稍后安装", "安装并重新启动"}, DefaultButton: 0, CancelButton: 0})
		a.updateMain(func() {
			state.confirming = false
			if e != nil {
				state.status = "无法显示安装确认：" + e.Error()
				return
			}
			if result.Button != 1 || a.stopped.Load() || a.hub != nil && a.hub.quitting {
				return
			}
			if state.prepared != prepared {
				state.status = "更新内容已变化，请重新确认安装"
				return
			}
			a.commitUpdate()
		})
	}()
}
func (a *App) commitUpdate() {
	if a.stopped.Load() || a.hub != nil && a.hub.quitting {
		return
	}
	state := a.updateState()
	if !a.updateSourceStillCurrent() {
		state.status = "更新仓库已变化，请重新检查"
		return
	}
	if a.updateDialogsOpen() {
		state.pendingInstall = true
		return
	}
	if state.busy || state.prepared == nil || a.passwordLocked() || a.configurationBusy() {
		return
	}
	state.busy = true
	state.status = "正在关闭会话并安装…"
	state.installing = true
	a.setConfigurationBusy(true)
	apps := []*App{a}
	if a.hub != nil {
		a.hub.quitting = true
		apps = append([]*App(nil), a.hub.windows...)
		if a.hub.fileWindow != nil {
			apps = append(apps, a.hub.fileWindow)
		}
	}
	var panes []*pane
	for _, window := range apps {
		tabs := append([]*tab(nil), window.tabs...)
		for _, tab := range tabs {
			window.walk(tab.root, func(p *pane) { panes = append(panes, p) })
			window.closeTab(tab)
		}
	}
	prepared := state.prepared
	go func() {
		for _, p := range panes {
			p.disposal.Wait()
		}
		_, e := prepared.CommitWithReceipt(context.Background(), a.store.Dir)
		a.updateMain(func() {
			state.busy = false
			state.installing = false
			a.setConfigurationBusy(false)
			if e != nil {
				if a.hub != nil {
					a.hub.quitting = false
					a.hub.scheduleUpdates()
					a.hub.scheduleSharedSync(2 * time.Second)
				}
				state.status = e.Error()
				return
			}
			state.prepared = nil
			if state.download != nil {
				state.download.Remove()
				state.download = nil
			}
			state.status = "更新已安装，正在重新启动…"
			mygo.App.Relaunch()
		})
	}()
}
func (h *applicationHub) initializeUpdates() {
	if target, info, e := currentUpdateBundle(); e == nil {
		go func() {
			e := core.FinishUpdateRecovery(context.Background(), h.store.Dir, target, info.Identifier)
			if e != nil {
				mygo.RunOnMain(func() { h.updates.status = e.Error() })
			}
		}()
	}
	h.scheduleUpdates()
}
func (h *applicationHub) scheduleUpdates() {
	state := &h.updates
	repository := h.config.Settings.UpdateRepository
	if repository == "" {
		if _, info, e := currentUpdateBundle(); e == nil {
			repository = info.UpdateRepository
		}
	}
	if len(h.windows) == 0 && h.fileWindow == nil {
		if state.timer != nil {
			state.timer.Stop()
			state.timer = nil
		}
		return
	}
	if state.source != "" && state.source != repository && !state.busy && state.prepared == nil {
		state.candidate = nil
		state.cache = nil
		state.source = ""
	}
	if !h.config.Settings.AutomaticUpdateChecks || h.quitting || repository == "" {
		if state.timer != nil {
			state.timer.Stop()
			state.timer = nil
		}
		state.scheduledSource = ""
		state.nextCheck = time.Time{}
		return
	}
	if state.scheduledSource != repository {
		if state.timer != nil {
			state.timer.Stop()
			state.timer = nil
		}
		state.scheduledSource = repository
		state.nextCheck = time.Now().Add(2 * time.Second)
	}
	if state.timer != nil {
		return
	}
	delay := time.Until(state.nextCheck)
	if delay < 0 {
		delay = 2 * time.Second
	}
	state.timer = time.AfterFunc(delay, func() {
		mygo.RunOnMain(func() {
			state.timer = nil
			if h.quitting {
				return
			}
			_, locked := h.vault.State()
			if locked {
				return
			} // unlocking schedules the overdue check
			a := h.fileWindow
			if len(h.windows) > 0 {
				a = h.focused(nil)
			}
			if a == nil {
				return
			}
			state.nextCheck = time.Now().Add(24 * time.Hour)
			a.checkForUpdates(true)
			h.scheduleUpdates()
		})
	})
}
func (h *applicationHub) updateInstallationInProgress() bool { return h.updates.installing }
func (h *applicationHub) updateMenuLabel() string {
	if h.updates.prepared != nil {
		return "安装已下载的更新…"
	}
	return "检查更新…"
}
func (a *App) invokeUpdater() {
	owner := a
	if a.hub != nil && a.hub.settingsOwner != nil {
		owner = a.hub.settingsOwner
	}
	owner.features.settingsTab = 4
	owner.openSettings()
	if owner.updateState().prepared != nil {
		owner.installUpdate()
	} else {
		owner.checkForUpdates(false)
	}
}
func (h *applicationHub) stopUpdates() {
	state := &h.updates
	state.pendingInstall = false
	if state.timer != nil {
		state.timer.Stop()
	}
	if state.cancel != nil {
		state.cancel()
	}
	if state.prepared != nil {
		state.prepared.Discard()
	}
	if state.download != nil {
		state.download.Remove()
	}
}

func (a *App) updateDialogsOpen() bool {
	apps := []*App{a}
	if a.hub != nil {
		apps = append([]*App(nil), a.hub.windows...)
		if a.hub.fileWindow != nil {
			apps = append(apps, a.hub.fileWindow)
		}
	}
	for _, window := range apps {
		f := &window.features
		if window.modalActive() || f.vaultBusy || window.hasOpenTerminalSearch() || f.settingsCapture != "" {
			return true
		}
	}
	return false
}

func (a *App) updatesBlockRepositoryReload() bool {
	state := a.updateState()
	return state.busy || state.prepared != nil
}

func (a *App) updateToolbar(c *ui.Context) {
	state := a.updateState()
	label := "更新"
	if state.prepared != nil {
		label = "安装更新"
	}
	text := label
	if a.win != nil {
		w, _ := a.win.Size()
		if w < 1000 {
			text = ""
		}
	}
	toolButton(c, label, arrowUpIcon, text).Disabled(state.busy || state.confirming).OnClick(a.invokeUpdater)
}
func (a *App) confirmAvailableUpdate() {
	state := a.updateState()
	candidate := state.candidate
	if candidate == nil || state.confirming || state.busy {
		return
	}
	state.confirming = true
	go func() {
		result, err := showUpdateConfirmation(mygo.MessageOptions{Parent: a.preferencesWindow, Type: mygo.MessageQuestion, Title: "发现新版本", Message: "下载 OShell Desktop " + candidate.Manifest.Version + "？", Detail: candidate.Manifest.Notes, Buttons: []string{"稍后", "下载更新"}, DefaultButton: 0, CancelButton: 0})
		a.updateMain(func() {
			state.confirming = false
			if err == nil && result.Button == 1 && state.candidate == candidate && a.updateSourceStillCurrent() {
				a.downloadUpdate()
			}
		})
	}()
}

func (a *App) resumeDeferredUpdate() {
	state := a.updateState()
	if !state.pendingInstall || state.prepared == nil || state.busy || state.confirming || a.stopped.Load() || a.hub != nil && a.hub.quitting || a.passwordLocked() || a.configurationBusy() || a.updateDialogsOpen() {
		return
	}
	state.pendingInstall = false
	a.installUpdate()
}

package desktop

import (
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

// Like native OShell, keep snapshots private to this running application.
// Clipboard text is an opaque reference, never a configuration or password.
type sessionConfigurationClipboard struct {
	token, scope, vaultRevision string
	generation                  uint64
	vault                       *core.Vault
	profiles                    []core.Profile
	proxies                     []core.ProxyProfile
}

func (a *App) sessionClipboard() **sessionConfigurationClipboard {
	if a.hub != nil {
		return &a.hub.managerClipboard
	}
	return &a.managerClipboard
}

func (a *App) managerClipboardProfiles() []core.Profile {
	if a.passwordLocked() || a.configurationBusy() || a.vault == nil {
		return nil
	}
	ids := map[string]bool{}
	for _, row := range a.selectedManagerRows() {
		if row.folder {
			return nil
		}
		ids[row.profile.ID] = true
	}
	var profiles []core.Profile
	for _, profile := range a.config.Clone().Profiles {
		if ids[profile.ID] {
			profiles = append(profiles, profile)
		}
	}
	return profiles
}

func (a *App) copyManagerConfigurations(c *ui.Context) {
	profiles := a.managerClipboardProfiles()
	if len(profiles) == 0 {
		return
	}
	snapshot := &sessionConfigurationClipboard{
		token: "oshell-session-configuration:" + core.NewID(), scope: filepath.Clean(a.store.Dir),
		vault: a.vault, vaultRevision: a.vault.Revision(), profiles: profiles, proxies: a.config.Clone().Proxies,
	}
	c.WriteClipboard(snapshot.token)
	if c.ReadClipboard() != snapshot.token {
		a.status = "复制失败：无法写入剪贴板"
		return
	}
	generation, ok := c.ClipboardGeneration()
	text := c.ReadClipboard()
	after, valid := c.ClipboardGeneration()
	if !ok || !valid || generation != after || text != snapshot.token {
		*a.sessionClipboard() = nil
		a.status = "复制失败：无法确认剪贴板代次，请重新复制"
		return
	}
	snapshot.generation = generation
	*a.sessionClipboard() = snapshot
	a.status = fmt.Sprintf("已复制 %d 个会话配置，进入目标目录后按 Ctrl+V 粘贴", len(profiles))
}

func (a *App) readManagerClipboard(c *ui.Context) *sessionConfigurationClipboard {
	before, ok := c.ClipboardGeneration()
	snapshot := a.managerClipboardSnapshot(c.ReadClipboard())
	after, valid := c.ClipboardGeneration()
	if snapshot != nil && (!ok || !valid || before != after || after != snapshot.generation) {
		*a.sessionClipboard() = nil
		return nil
	}
	return snapshot
}

func (a *App) managerClipboardSnapshot(text string) *sessionConfigurationClipboard {
	state := a.sessionClipboard()
	if *state == nil {
		return nil
	}
	if (*state).token != text {
		*state = nil
		return nil
	}
	if a.passwordLocked() || a.configurationBusy() || a.vault == nil || (*state).vault != a.vault || (*state).scope != filepath.Clean(a.store.Dir) {
		return nil
	}
	return *state
}

func (a *App) pasteManagerConfigurations(c *ui.Context) {
	snapshot := a.readManagerClipboard(c)
	if snapshot == nil {
		a.status = "粘贴失败：请先在会话管理中复制会话配置"
		return
	}
	// Credentials are rebound at paste time. Never silently substitute changed
	// credentials or proxy routes for the ones selected at copy time.
	if snapshot.vaultRevision != a.vault.Revision() || !reflect.DeepEqual(snapshot.proxies, a.config.Proxies) {
		a.status = "粘贴失败：密码库或代理配置已变化，请重新复制"
		return
	}
	destination := a.sessionActionGroup()
	if a.sidebar.context == nil && a.managerAll {
		destination = ""
	}
	if destination != "" {
		exists := false
		for _, dir := range a.config.AllDirectories() {
			exists = exists || dir == destination
		}
		if !exists {
			a.status = "粘贴失败：目标目录已不存在"
			return
		}
	}
	a.copyManagerProfiles(snapshot.profiles, &destination, func() {
		// Our own successful paste adds credential entries; repeated paste is
		// still valid, provided the clipboard has not since been replaced.
		if *a.sessionClipboard() == snapshot {
			snapshot.vaultRevision = a.vault.Revision()
		}
	})
}

func (a *App) managerClipboardShortcuts(c *ui.Context, table ui.Element) {
	if a.managerRename != nil {
		return
	}
	mods := []ui.Modifiers{ui.Ctrl}
	if runtime.GOOS == "darwin" {
		mods = append(mods, ui.Cmd)
	}
	for _, mod := range mods {
		if table.Shortcut(mod, ui.KeyC) {
			a.copyManagerConfigurations(c)
		}
		if table.Shortcut(mod, ui.KeyV) {
			a.pasteManagerConfigurations(c)
		}
	}
}

func (a *App) managerClipboardMenu(c *ui.Context, m *ui.Menu) bool {
	if m.Item("复制会话配置").Shortcut(ui.Ctrl, ui.KeyC).Disabled(len(a.managerClipboardProfiles()) == 0).Chosen() {
		a.copyManagerConfigurations(c)
		return true
	}
	if m.Item("粘贴会话配置").Shortcut(ui.Ctrl, ui.KeyV).Disabled(a.readManagerClipboard(c) == nil).Chosen() {
		a.pasteManagerConfigurations(c)
		return true
	}
	m.Separator()
	return false
}

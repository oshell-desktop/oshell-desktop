package desktop

import (
	"errors"
	"oshell-desktop/internal/core"
)

func startupConfiguration(store *core.Store) (core.Config, error) {
	cfg, err := store.Load()
	if errors.Is(err, core.ErrConfigurationLocked) {
		return core.DefaultConfig(), nil
	}
	return cfg, err
}

// workspaceLocked is the startup gate; clearing cached secrets never revokes
// an already authorized workspace or its authenticated terminal connections.
func (a *App) workspaceLocked() bool {
	authorized := a.workspaceAuthorized || a.hub != nil && a.hub.workspaceAuthorized
	return !authorized && a.passwordLocked()
}
func (a *App) authorizeWorkspace() {
	a.workspaceAuthorized = true
	if a.hub != nil {
		a.hub.workspaceAuthorized = true
	}
}
func (a *App) lockWorkspaceSecrets() {
	if !a.workspaceLocked() {
		a.authorizeWorkspace()
	}
	windows := []*App{a}
	if a.hub != nil {
		windows = a.hub.configurationWindows()
	}
	for _, window := range windows {
		window.cancelSharedSync()
		window.cancelWebDAVProbe()
	}
	if a.store != nil {
		a.store.LockConfiguration()
	}
	if a.vault != nil {
		a.vault.Lock()
	}
	a.invalidateSecurityWindows()
}

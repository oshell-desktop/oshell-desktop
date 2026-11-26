//go:build !windows

package desktop

import "github.com/egoist/mygo/ui"

type clientAliasSettings struct{}

func (a *App) prepareClientAliases()         {}
func (a *App) applyClientAliases() error     { return nil }
func (a *App) clientAliasesView(*ui.Context) {}

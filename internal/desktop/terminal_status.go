package desktop

import (
	"github.com/egoist/mygo"
	"oshell-desktop/internal/terminal"
)

var terminalBell = func() {
	if mygo.App.IsReady() {
		mygo.Shell.Beep()
	}
}

func (a *App) configureTerminalStatus(p *pane, opts *terminal.Options) {
	opts.OnBell = func() {
		if owner := p.owner.Load(); owner != nil {
			owner.post(func() {
				if !p.closed && p.owner.Load() == owner {
					terminalBell()
				}
			})
		}
	}
}

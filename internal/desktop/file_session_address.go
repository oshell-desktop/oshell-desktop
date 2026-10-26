package desktop

import (
	"context"
	"oshell-desktop/internal/core"
	"time"
)

func (a *App) resolveFileSessionAddress(parent context.Context, p *pane) {
	profile := p.profile
	p.fileDisplayIP = core.FileSessionLiteral(profile.Host)
	if p.fileDisplayIP != "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(parent, 5*time.Second)
		defer cancel()
		address := core.FileSessionAddress(ctx, profile)
		if ctx.Err() != nil {
			return
		}
		postPane(p, func(_ *App) {
			if !p.closed && p.profile.Identity() == profile.Identity() {
				p.fileDisplayIP = address
			}
		})
	}()
}

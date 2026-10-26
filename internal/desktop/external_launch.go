package desktop

import (
	"context"
	"fmt"
	"github.com/egoist/mygo"
	"oshell-desktop/internal/core"
)

type pendingExternalLaunch struct {
	ctx     context.Context
	request core.ExternalLaunchRequest
	result  chan error
}

func (h *applicationHub) startExternalLaunch() {
	server, e := core.StartLaunchServer(h.store.StorageBootstrapDirectory(), func(ctx context.Context, request core.ExternalLaunchRequest) error {
		pending := &pendingExternalLaunch{ctx: ctx, request: request, result: make(chan error, 1)}
		mygo.RunOnMain(func() { h.externalPending = append(h.externalPending, pending); h.drainExternalLaunch() })
		select {
		case e := <-pending.result:
			return e
		case <-ctx.Done():
			mygo.RunOnMain(func() { h.drainExternalLaunch() })
			return ctx.Err()
		}
	})
	if e != nil {
		if len(h.windows) > 0 {
			h.windows[0].status = "外部启动服务不可用：" + e.Error()
		}
		return
	}
	h.externalServer = server
}
func (h *applicationHub) drainExternalLaunch() {
	remaining := h.externalPending[:0]
	for _, pending := range h.externalPending {
		if pending.ctx.Err() != nil {
			pending.request = core.ExternalLaunchRequest{}
			continue
		}
		if h.quitting {
			pending.result <- fmt.Errorf("应用正在关闭")
			pending.request = core.ExternalLaunchRequest{}
			continue
		}
		a := h.focused(nil)
		if a.passwordLocked() {
			remaining = append(remaining, pending)
			if !a.features.vaultOpen {
				a.showVault()
			}
			a.features.startupUnlock = true
			if a.win != nil {
				a.win.Focus()
			}
			continue
		}
		if e := pending.request.Validate(); e != nil {
			pending.result <- e
			pending.request = core.ExternalLaunchRequest{}
			continue
		}
		var profile core.Profile
		var password *string
		var e error
		if request := pending.request.Terminal; request != nil {
			profile, e = request.ProfileValue()
			password = request.Password
		} else {
			profile = pending.request.File.Profile
			password = pending.request.File.Password
		}
		if e == nil {
			credentials := core.Credentials{ExternalLaunch: true}
			if password != nil {
				credentials.Password = *password
			}
			if pending.request.File != nil {
				h.openExternalFile(profile, credentials)
			} else {
				a.open(profile, credentials, 0)
			}
			if pending.request.File == nil && a.win != nil {
				a.win.Focus()
			}
		}
		pending.result <- e
		pending.request = core.ExternalLaunchRequest{}
	}
	h.externalPending = remaining
}

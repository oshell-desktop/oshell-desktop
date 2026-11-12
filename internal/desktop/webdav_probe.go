package desktop

import (
	"context"
	"oshell-desktop/internal/core"
	"time"
)

func (a *App) cancelWebDAVProbe() {
	f := &a.features.sharedSync
	f.probeGeneration++
	if f.probeCancel != nil {
		f.probeCancel()
		f.probeCancel = nil
	}
	if f.probing {
		f.probeStatus = "连接测试已取消"
	}
	f.probing = false
}

func (a *App) testWebDAVConnection() {
	f := &a.features.sharedSync
	if f.probing || f.busy || a.configurationBusy() {
		return
	}
	protected, locked := a.vault.State()
	if !protected || locked {
		f.probeStatus = "请先启用并解锁主密码保护"
		return
	}
	options, password := f.options, f.password
	draftPassword := f.password
	if options.Backend != "webdav" {
		return
	}
	if password == "" {
		p := options.CredentialProfile()
		// Only credentials bound to this exact draft identity can be reused.
		if a.vault.HasCredential(p) {
			var err error
			password, err = a.vault.GetFor(p, p.Identity())
			if err != nil {
				f.probeStatus = err.Error()
				return
			}
		}
	}
	transport, err := options.Transport(password, a.store.Dir)
	if err != nil {
		f.probeStatus = err.Error()
		return
	}
	f.probeGeneration++
	generation := f.probeGeneration
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	f.probeCancel = cancel
	f.probing = true
	f.probeStatus = "正在测试 WebDAV 连接…"
	go func() {
		started := time.Now()
		webdav := transport.(*core.WebDAVTransport)
		status, err := webdav.ProbeStatus(ctx)
		directory := ""
		if err == nil {
			if resolved, resolveErr := webdav.SyncDirectory(ctx); resolveErr != nil {
				err = resolveErr
			} else {
				directory = resolved.Path
			}
		}
		cancel()
		logErr := (core.SyncDiagnostics{Directory: a.store.Dir, Target: options.Target(), Backend: "webdav"}).Record(core.SyncConnectionTest, true, time.Since(started), err, "")
		a.post(func() {
			if f.probeGeneration != generation {
				return
			}
			f.probing = false
			f.probeCancel = nil
			if f.options != options || f.password != draftPassword {
				f.probeStatus = "设置已改变，请重新测试"
				return
			}
			if err != nil {
				f.probeStatus = err.Error()
			} else {
				f.probeStatus = "连接正常；同步目录：" + directory + "。" + status + "。点击应用保存。"
			}
			if logErr != nil {
				f.logIssue = logErr.Error()
			}
		})
	}()
}

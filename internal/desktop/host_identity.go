package desktop

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"strings"
	"time"
)

func (a *App) initializeHostname(p *pane) {
	if p.titleMode == "" && p.profile.Kind == "local" {
		if _, saved := a.profileByID(p.profile.ID); !saved {
			p.titleMode = a.localUI.DefaultTitleMode
			for _, existing := range a.allPanes() {
				if existing != p && existing.profile.ID == p.profile.ID {
					p.titleMode = a.paneTitleMode(existing)
					break
				}
			}
		}
	}
	p.hostname = p.profile.Name
	p.hostObservation.Mode = a.paneTitleMode(p)
	if p.hostObservation.Mode == "sessionName" {
		p.hostObservation.Mode = "shellIntegration"
	}
	if p.profile.Kind == "local" {
		names, identity := core.LocalIdentitySnapshot()
		p.localNames = names
		p.hostname = names.Hostname
		p.hostObservation.Identity = identity
	} else if p.profile.Kind == "ssh" || p.terminalOnly {
		p.hostname = "主机待识别"
	}
}
func (a *App) showHostname(p *pane) {
	if p.closed {
		return
	}
	if host := p.hostObservation.Identity.Hostname; host != "" {
		p.hostname = host
	}
	if owner := a.ownerTab(p); owner != nil {
		owner.name = a.displayTabTitle(owner)
	}
	a.applyTerminalPreferences(p)
}
func (a *App) receiveHostIdentity(p *pane, identity core.HostIdentity) {
	if p.closed {
		return
	}
	if p.profile.Kind == "local" || (p.term != nil && p.term.InLocalToolsMode()) {
		p.localNames = core.LocalHostnameSnapshot().WithPrevious(p.localNames)
		identity.Hostname = p.localNames.Canonical(identity.Hostname)
	}
	if p.hostObservation.Integration(identity) {
		a.cancelHostProbe(p)
		a.showHostname(p)
	}
}
func (a *App) receiveHostnameHint(p *pane, host string) {
	if p.closed || host == "" {
		return
	}
	if p.profile.Kind == "local" || (p.term != nil && p.term.InLocalToolsMode()) {
		p.localNames = core.LocalHostnameSnapshot().WithPrevious(p.localNames)
		host = p.localNames.Canonical(host)
	}
	if p.hostObservation.Hint(host) {
		a.cancelHostProbe(p)
		p.hostProbe.epoch++
		p.hostProbe.attempted = false
		a.showHostname(p)
	}
}

func scheduleHostnameScan(p *pane) {
	if !p.promptScheduled.CompareAndSwap(false, true) {
		return
	}
	time.AfterFunc(80*time.Millisecond, func() {
		owner := p.owner.Load()
		if owner == nil {
			p.promptScheduled.Store(false)
			return
		}
		if !owner.post(func() {
			p.promptScheduled.Store(false)
			if p.closed || p.term == nil {
				return
			}
			line, ok := p.term.HostPrompt()
			if !ok || (line == p.lastHostPrompt && !p.hostObservation.PromptPending) {
				return
			}
			p.lastHostPrompt = line
			owner.receiveHostnameHint(p, core.HostnameFromPrompt(line))
			owner.scheduleActiveHostProbe(p, line)
		}) {
			p.promptScheduled.Store(false)
		}
	})
}
func (a *App) noteHostInput(p *pane, ev ui.InputEvent) {
	if p.term == nil {
		return
	}
	if ev.Kind == ui.InputKeyDown || ev.Kind == ui.InputText {
		if p.hostProbe.work != nil {
			p.hostProbe.work.Stop()
			p.hostProbe.work = nil
		}
		if ev.Kind == ui.InputText {
			p.hostProbe.editing = true
		}
	}
	enter := ev.Kind == ui.InputKeyDown && ev.Key == ui.KeyEnter
	eof := ev.Kind == ui.InputKeyDown && ev.Key == ui.KeyD && ev.Mods&ui.Ctrl != 0
	if ev.Kind == ui.InputKeyDown && ev.Mods&ui.Ctrl != 0 && (ev.Key == ui.KeyC || ev.Key == ui.KeyU) {
		p.hostProbe.editing = false
	}
	if enter || eof {
		p.hostProbe.editing = false
	}
	if !enter && !eof {
		return
	}
	line, ok := p.term.HostPrompt()
	if !ok {
		return
	}
	if eof || core.ChangesHost(line, true) {
		a.cancelHostProbe(p)
		p.hostProbe.epoch++
		p.hostProbe.attempted = false
		p.hostObservation.HostChange()
		p.lastHostPrompt = ""
		if p.hostObservation.Identity.Hostname == "" {
			p.hostname = "主机待识别"
		}
		a.showHostname(p)
	}
}

type hostProbeState struct {
	work, timeout      *time.Timer
	token              string
	epoch              int
	attempted, editing bool
}

func (a *App) cancelHostProbe(p *pane) {
	if p.hostProbe.work != nil {
		p.hostProbe.work.Stop()
		p.hostProbe.work = nil
	}
	if p.hostProbe.timeout != nil {
		p.hostProbe.timeout.Stop()
		p.hostProbe.timeout = nil
	}
	p.hostProbe.token = ""
	if p.term != nil {
		p.term.EndHostProbe()
	}
}
func (a *App) receiveHostTitle(p *pane, title string) {
	if token := p.hostProbe.token; token != "" && strings.HasPrefix(title, token) {
		a.cancelHostProbe(p)
		if !p.hostObservation.HasIntegration {
			if identity, ok := core.ParseHostIdentity(strings.TrimPrefix(title, token)); ok {
				if p.profile.Kind == "local" {
					identity.Hostname = p.localNames.Canonical(identity.Hostname)
				}
				p.hostObservation.Probe(identity)
				a.showHostname(p)
			}
		}
		return
	}
	a.receiveHostnameHint(p, core.HostnameFromTitle(title))
}
func (a *App) scheduleActiveHostProbe(p *pane, line string) {
	if p.closed || p.term == nil || !p.hostObservation.AllowsActiveProbe() || p.hostProbe.editing || p.hostProbe.attempted || p.hostProbe.token != "" || p.hostProbe.work != nil || core.HostnameFromPrompt(line) == "" || core.IsAuthenticationPrompt(line) || !p.term.ManagedInputAvailable() {
		return
	}
	epoch := p.hostProbe.epoch
	p.hostProbe.work = time.AfterFunc(300*time.Millisecond, func() {
		if owner := p.owner.Load(); owner != nil {
			owner.post(func() {
				p.hostProbe.work = nil
				if p.closed || p.term == nil || p.hostProbe.epoch != epoch || p.hostProbe.editing || !p.hostObservation.AllowsActiveProbe() || !p.term.ManagedInputAvailable() {
					return
				}
				current, ok := p.term.HostPrompt()
				if !ok || current != line {
					return
				}
				token := "OSHELL_INFO_" + strings.ReplaceAll(core.NewID(), "-", "") + ":"
				command, e := core.HostProbeCommand(token)
				if e != nil {
					return
				}
				p.hostProbe.token = token
				p.hostProbe.attempted = true
				p.hostObservation.Identity.Address = ""
				if !p.term.BeginHostProbe(command) {
					p.hostProbe.token = ""
					return
				}
				p.hostProbe.timeout = time.AfterFunc(4*time.Second, func() {
					if current := p.owner.Load(); current != nil {
						current.post(func() {
							if p.hostProbe.token == token {
								current.cancelHostProbe(p)
							}
						})
					}
				})
			})
		}
	})
}

func paneConnectionDetails(p *pane) string {
	if p == nil {
		return ""
	}
	name := p.hostname
	if name == "" {
		name = "主机待识别"
	}
	address := p.hostObservation.Identity.Address
	if address == "" {
		address = "未获取"
	}
	details := "当前主机\n主机名：" + name + "\n主机 IP：" + address
	status := p.status
	if p.closed {
		status = "已关闭"
	} else if p.conn != nil && !p.conn.Alive() {
		status = "连接已结束"
	}
	if status == "" {
		status = "未获取"
	}
	details += "\n连接状态：" + status
	if p.profile.Kind == "ssh" || p.profile.Kind == "sftp" {
		details += fmt.Sprintf("\n\n初始 SSH 连接\n会话：%s\n配置地址：%s\n端口：%d\n用户名：%s", p.profile.Name, p.profile.Host, p.profile.Port, p.profile.Username)
		peer, port := p.conn.ObservedPeer()
		if peer == "" {
			details += "\n实际连接 IP：未获取"
		} else {
			details += fmt.Sprintf("\n实际连接 IP：%s\n实际连接端口：%d", peer, port)
		}
		switch p.conn.TunnelPolicy() {
		case "owned":
			details += "\n隧道：由此 SSH 传输连接承载（渠道共享）"
		case "skipped":
			details += "\n隧道：本次跳过（建立连接时已有同一会话的连接承载）"
		case "none":
			details += "\n隧道：未启用转发规则"
		default:
			details += "\n隧道：尚未获取连接策略"
		}
	}
	return details
}

func (a *App) noteHostCommand(p *pane, text string, submitted bool) {
	if !submitted || !core.ChangesHost(text, false) {
		return
	}
	a.cancelHostProbe(p)
	p.hostProbe.epoch++
	p.hostProbe.attempted = false
	p.hostProbe.editing = false
	p.hostObservation.HostChange()
	p.lastHostPrompt = ""
	if p.hostObservation.Identity.Hostname == "" {
		p.hostname = "主机待识别"
	}
	a.showHostname(p)
}
func (a *App) refreshHostIdentity(p *pane) {
	if p == nil || p.closed || p.term == nil {
		return
	}
	if p.term.InLocalToolsMode() && !p.term.ToolsProcessActive() {
		a.useLocalHostIdentity(p)
		return
	}

	line, ok := p.term.HostPrompt()
	if !ok || core.IsAuthenticationPrompt(line) || p.hostProbe.editing || !p.term.ManagedInputAvailable() {
		return
	}
	if p.hostObservation.AllowsActiveProbe() {
		p.hostProbe.attempted = false
		a.scheduleActiveHostProbe(p, line)
	} else {
		a.receiveHostnameHint(p, core.HostnameFromPrompt(line))
	}
}

func (a *App) useLocalHostIdentity(p *pane) {
	names, identity := core.LocalIdentitySnapshot()
	p.localNames = names.WithPrevious(p.localNames)
	a.cancelHostProbe(p)
	mode := a.paneTitleMode(p)
	if mode == "sessionName" {
		mode = "shellIntegration"
	}
	p.hostObservation = core.HostObservation{Mode: mode, Identity: identity}
	a.showHostname(p)
}

func (a *App) tabConnectionDetails(t *tab) string {
	if t == nil {
		return ""
	}
	details := "标签组：" + a.groupTitle(a.groupByID(t.group))
	index := 0
	a.walk(t.root, func(p *pane) {
		index++
		current := ""
		if p == t.active {
			current = "（当前）"
		}
		details += fmt.Sprintf("\n\n分屏 %d%s · %s\n%s", index, current, a.paneDisplayTitle(p), paneConnectionDetails(p))
	})
	return details
}

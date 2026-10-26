// SPDX-License-Identifier: GPL-3.0-only
package desktop

import (
	"context"
	"fmt"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"slices"
)

type authenticationAnswer struct {
	protection string
	values     []string
	remember   bool
	err        error
}
type authenticationChallenge struct {
	host                *core.HostPrompt
	sequence            uint64
	finished            bool
	protection          string
	pane                *pane
	id                  string
	ctx                 context.Context
	request             core.AuthenticationPrompt
	answers             []string
	allowSave, remember bool
	open                bool
	done                chan authenticationAnswer
}

func askAuthentication(ctx context.Context, p *pane, request core.AuthenticationPrompt) ([]string, error) {
	result := askAuthenticationWithSaving(ctx, p, request, false)
	return result.values, result.err
}

func askAuthenticationWithSaving(ctx context.Context, p *pane, request core.AuthenticationPrompt, allowSave bool) authenticationAnswer {
	allowSave = allowSave && !p.externalLaunch
	challenge := &authenticationChallenge{allowSave: allowSave, pane: p, id: core.NewID(), ctx: ctx, request: request, answers: make([]string, len(request.Questions)), open: true, done: make(chan authenticationAnswer, 1)}
	return waitAuthentication(ctx, p, challenge)
}

func askHostAuthentication(ctx context.Context, p *pane, host core.HostPrompt) bool {
	if host.Context != nil {
		ctx = host.Context
	}
	challenge := &authenticationChallenge{host: &host, pane: p, id: core.NewID(), ctx: ctx, request: core.AuthenticationPrompt{Title: "首次连接：核对服务器指纹"}, open: true, done: make(chan authenticationAnswer, 1)}
	return waitAuthentication(ctx, p, challenge).err == nil
}

func waitAuthentication(ctx context.Context, p *pane, challenge *authenticationChallenge) authenticationAnswer {
	if !postPane(p, func(a *App) {
		if ctx.Err() == nil {
			challenge.protection = "local"
			if a.vault != nil {
				challenge.protection = a.vault.CredentialProtection(p.profile.ID)
			}
			a.enqueueChallenge(challenge)
		}
	}) {
		return authenticationAnswer{err: context.Canceled}
	}
	select {
	case result := <-challenge.done:
		if ctx.Err() != nil {
			return authenticationAnswer{err: ctx.Err()}
		}
		return result
	case <-ctx.Done():
		postPane(p, func(a *App) {
			a.finishChallenge(challenge, true)
		})
		return authenticationAnswer{err: ctx.Err()}
	}
}

func (a *App) enqueueChallenge(q *authenticationChallenge) {
	if a.hub != nil {
		a.hub.authenticationSequence++
		q.sequence = a.hub.authenticationSequence
	}
	a.challenges = append(a.challenges, q)
	a.refreshAuthentication()
}

func (a *App) authenticationWindows() []*App {
	if a.hub != nil {
		return a.hub.configurationWindows()
	}
	return []*App{a}
}

// Queue only operator prompts, never the connection handshake or its ready
// callback. A connection that can authenticate unattended must keep running.
func (a *App) nextChallenge() *authenticationChallenge {
	var next *authenticationChallenge
	blocked := false
	for _, w := range a.authenticationWindows() {
		for _, q := range append([]*authenticationChallenge(nil), w.challenges...) {
			if q.ctx.Err() != nil || q.pane.closed || w.stopped.Load() {
				w.finishChallenge(q, true)
			}
		}
		if w.stopped.Load() {
			continue
		}
		blocked = blocked || w.nonAuthenticationModalActive()
		for _, q := range w.challenges {
			if next == nil || q.sequence < next.sequence {
				next = q
			}
		}
	}
	if blocked {
		return nil
	}
	return next
}

func (a *App) refreshAuthentication() {
	if a.hub == nil {
		return
	}
	q := a.nextChallenge()
	if a.hub.authenticationReady == q {
		return
	}
	a.hub.authenticationReady = q
	for _, w := range a.authenticationWindows() {
		if w.win != nil {
			w.win.Invalidate()
		}
	}
}

func (a *App) finishChallenge(q *authenticationChallenge, cancelled bool) {
	if q.finished {
		return
	}
	q.finished = true
	result := authenticationAnswer{values: append([]string(nil), q.answers...), remember: q.allowSave && q.remember, protection: q.protection}
	if cancelled || q.ctx.Err() != nil || q.pane.closed {
		result = authenticationAnswer{err: context.Canceled}
	}
	q.done <- result
	for _, w := range a.authenticationWindows() {
		w.removeChallenge(q)
		if w.win != nil {
			w.win.Invalidate()
		}
	}
}
func (a *App) removeChallenge(challenge *authenticationChallenge) {
	for i, v := range a.challenges {
		if v == challenge {
			a.challenges = slices.Delete(a.challenges, i, i+1)
			break
		}
	}
	for i := range challenge.answers {
		challenge.answers[i] = ""
	}
}
func (a *App) authenticationChallengeDialog(c *ui.Context) {
	q := a.nextChallenge()
	if q == nil || q.pane.owner.Load() != a {
		return
	}
	finish := func(cancelled bool) { a.finishChallenge(q, cancelled) }
	ui.Modal(c.Key(q.id), &q.open, func() {
		ui.Column(c).Width(500).Gap(12).Children(func() {
			title := q.request.Title
			if title == "" {
				title = "服务器认证"
			}
			ui.Text(c, title).FontSize(18).Bold()
			if q.pane.profile.Name != "" {
				ui.Text(c, q.pane.profile.Name).Bold()
			}
			if q.host != nil {
				ui.Text(c, q.host.Address).Wrap()
				ui.Text(c, q.host.Algorithm+"\n"+q.host.Fingerprint).Wrap()
				ui.Text(c, "请与服务器管理员提供的指纹核对。信任后将保存到 OShell 的 known_hosts。").Wrap()
			} else {
				ui.Text(c, fmt.Sprintf("%s@%s:%d", q.request.Identity.User, q.request.Identity.Host, q.request.Identity.Port))
			}
			if q.request.Name != "" {
				ui.Text(c, q.request.Name)
			}
			if q.request.Instruction != "" {
				ui.Text(c, q.request.Instruction).TextColor(c.Theme().TextMuted)
			}
			ui.Scroll(c).MaxHeight(350).Children(func() {
				for i, prompt := range q.request.Questions {
					ui.Field(c.Key(fmt.Sprint(i)), prompt, func() {
						input := ui.TextInput(c, &q.answers[i]).Label(prompt)
						if i >= len(q.request.Echo) || !q.request.Echo[i] {
							passwordSelection(input, &q.answers[i])
						}
						if i == 0 {
							input.AutoFocus()
						}
					})
				}
			})
			if q.allowSave {
				ui.Checkbox(c, &q.remember, "密码认证成功后保存登录密码")
				if q.remember {
					passwordProtectionField(c, &q.protection)
				}
			}
			ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
				cancel := ui.Button(c, "取消").OnClick(func() { finish(true) })
				label := "继续认证"
				if q.host != nil {
					label = "信任并连接"
					cancel.AutoFocus()
				}
				ui.PrimaryButton(c, label).OnClick(func() { finish(false) })
			})
		})
	})
	if !q.open {
		finish(true)
	}
}

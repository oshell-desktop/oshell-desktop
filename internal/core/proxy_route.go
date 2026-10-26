// SPDX-License-Identifier: GPL-3.0-only
package core

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
)

func (t *transport) connectProxyRoute(ctx context.Context, p Profile, cred Credentials, trust *TrustStore, approve func(HostPrompt) bool) error {
	route := append([]ProxySettings(nil), p.RuntimeProxyRoute...)
	if len(route) > 8 {
		return fmt.Errorf("代理链超过 8 级")
	}
	resolved := make([]Profile, len(route))
	for i, settings := range route {
		if e := settings.Validate(); e != nil {
			return e
		}
		rp := ProxyCredentialProfile(settings)
		rp.IdentityFile = settings.IdentityFile
		rp.LegacySSH = p.LegacySSH
		if settings.Kind == "jump" {
			result, e := ResolveSSH(ctx, rp)
			if e != nil {
				return e
			}
			rp = result.Profile
		}
		resolved[i] = rp
	}
	address := resolved[0].Address()
	socket, e := proxyConnect(ctx, ProxySettings{Kind: "none"}, "", address, p.KeepAlive.TCP)
	if e != nil {
		return e
	}
	t.observeSocket(socket)
	if !t.track(socket) {
		return fmt.Errorf("连接已关闭")
	}
	for i, settings := range route {
		if e = ctx.Err(); e != nil {
			return e
		}
		rp := resolved[i]
		hc := Credentials{Challenge: cred.Challenge, Method: settings.SSHAuthentication}
		if cred.ProxyCredentials != nil {
			hc, e = cred.ProxyCredentials(ctx, settings, rp.Identity())
			if e != nil {
				return e
			}
			hc.Method = settings.SSHAuthentication
			if hc.Challenge == nil {
				hc.Challenge = cred.Challenge
			}
		}
		title := "代理身份验证"
		if settings.Kind == "jump" {
			title = "SSH 跳板机认证"
		}
		hc.Challenge = authenticationTitle(hc.Challenge, title)
		if hc.ExpectedIdentity != nil && *hc.ExpectedIdentity != rp.Identity() {
			return fmt.Errorf("代理连接身份已变化，请重新输入密码")
		}
		target := p.Address()
		if i+1 < len(route) {
			target = resolved[i+1].Address()
		}
		if settings.Kind == "jump" {
			var verified atomic.Bool
			client, e := authenticate(ctx, socket, rp, hc, trust, approve, &verified)
			if e != nil {
				return fmt.Errorf("跳板 %d 认证失败：%w", i+1, e)
			}
			t.clients = append(t.clients, client)
			socket, e = client.DialContext(ctx, "tcp", target)
			if e != nil {
				return fmt.Errorf("跳板 %d 无法连接下一级：%w", i+1, e)
			}
		} else {
			if hc.Password == "" && settings.Username != "" && (settings.Kind == "http" || settings.Kind == "socks5") && hc.Challenge != nil {
				answers, e := hc.Challenge(ctx, AuthenticationPrompt{Identity: rp.Identity(), Name: fmt.Sprintf("第 %d 级代理认证", i+1), Questions: []string{"代理密码"}, Echo: []bool{false}})
				if e != nil {
					return e
				}
				if len(answers) != 1 {
					return fmt.Errorf("代理密码回答无效")
				}
				hc.Password = answers[0]
			}
			socket, e = proxyHandshake(ctx, socket, settings, hc.Password, target)
			if e != nil {
				return fmt.Errorf("第 %d 级代理失败：%w", i+1, e)
			}
		}
	}
	var verified atomic.Bool
	client, e := authenticate(ctx, socket, p, cred, trust, approve, &verified)
	if e != nil {
		socket.Close()
		return e
	}
	t.passwordVerified = verified.Load()
	t.clients = append(t.clients, client)
	return nil
}
func (p ProxySettings) Address() string {
	return net.JoinHostPort(strings.Trim(p.Host, "[]"), strconv.Itoa(p.Port))
}

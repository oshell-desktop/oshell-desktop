package core

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestSSHJumpRecursiveConfigAndExplicitUpstreamPrecedence(t *testing.T) {
	p := NewProfile()
	p.Host = "destination"
	p.Username = "target-only"
	p.JumpHost = "inner,explicit@last:2222"
	var seen []string
	resolver := func(_ context.Context, hop Profile) (ResolvedSSH, error) {
		seen = append(seen, hop.Host)
		if hop.Host != "last" && (hop.Username != "" || hop.Port != 0) {
			t.Fatal("target identity leaked into hop", hop.Host, hop.Username, hop.Port)
		}
		if hop.Host == "last" && (hop.Username != "explicit" || hop.Port != 2222) {
			t.Fatal("explicit identity discarded")
		}
		result := ResolvedSSH{Profile: hop}
		if hop.Port == 0 {
			result.Profile.Port = 2200
		}
		if hop.Username == "" {
			result.Profile.Username = "jump-user"
		}
		switch hop.Host {
		case "inner":
			result.Profile.JumpHost = "outer"
		case "outer":
			result.ProxyCommand = "helper %n %h %p"
		case "last":
			result.Profile.JumpHost = "must-not-visit"
			result.ProxyCommand = "must-not-run"
		default:
			t.Fatal("unexpected route", hop.Host)
		}
		return result, nil
	}
	route, err := resolveJumpRoute(context.Background(), p, resolver)
	if err != nil {
		t.Fatal(err)
	}
	var hosts []string
	for _, hop := range route {
		hosts = append(hosts, hop.Profile.Host)
	}
	if !reflect.DeepEqual(hosts, []string{"outer", "inner", "last"}) || !reflect.DeepEqual(seen, []string{"inner", "outer", "last"}) {
		t.Fatal(hosts, seen)
	}
	if route[0].ProxyCommand == "" || route[1].ProxyCommand != "" || route[2].ProxyCommand != "" || route[0].OriginalHost != "outer" {
		t.Fatal("upstream command precedence", route)
	}
}

func TestSSHJumpCyclesDepthAndURI(t *testing.T) {
	p := NewProfile()
	p.Host = "target"
	p.JumpHost = "loop"
	for _, mode := range []string{"cycle", "depth", "cancel"} {
		ctx, cancel := context.WithCancel(context.Background())
		if mode == "cancel" {
			cancel()
		}
		count := 0
		_, err := resolveJumpRoute(ctx, p, func(_ context.Context, hop Profile) (ResolvedSSH, error) {
			count++
			hop.Port = 22
			hop.Username = "fixture"
			hop.JumpHost = "loop"
			if mode == "depth" {
				hop.JumpHost = fmt.Sprintf("hop-%d", count)
			}
			return ResolvedSSH{Profile: hop}, nil
		})
		cancel()
		if err == nil || count > 32 {
			t.Fatal("unbounded route", mode, count, err)
		}
	}
	for _, tc := range []struct {
		spec, host, user string
		port             int
	}{
		{"alias", "alias", "", 0}, {"user@alias", "alias", "user", 0}, {"[::1]:2222", "::1", "", 2222},
		{"ssh://user@[::1]:2200", "::1", "user", 2200}, {"ssh://alias", "alias", "", 0},
	} {
		hop, err := parseConfiguredJump(tc.spec)
		if err != nil || hop.Host != tc.host || hop.Username != tc.user || hop.Port != tc.port {
			t.Fatal(tc.spec, hop, err)
		}
	}
	for _, bad := range []string{"ssh://user:password@alias", "ssh://alias/path", "alias:0", "alias:65536", "-option"} {
		if _, err := parseConfiguredJump(bad); err == nil {
			t.Fatal("invalid jump accepted", bad)
		}
	}
}

func TestProxyCommandOriginalHostAndUnsupportedTokens(t *testing.T) {
	p := NewProfile()
	p.Host = "resolved.example"
	p.Username = "user"
	s, err := expandProxyCommand("helper %n %h %p %r %%", p, "original-alias")
	if err != nil || !strings.Contains(s, "original-alias") || !strings.Contains(s, "resolved.example") {
		t.Fatal(s, err)
	}
	for _, invalid := range []string{"helper %x", "helper %"} {
		if _, err := expandProxyCommand(invalid, p); err == nil {
			t.Fatal("invalid token accepted", invalid)
		}
	}
}

package core

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// A jump without an explicit port/user must use that host's own SSH config,
// not the destination's login name or a command-line override of port 22.
func parseConfiguredJump(spec string) (Profile, error) {
	if strings.HasPrefix(spec, "ssh://") {
		u, err := url.Parse(spec)
		if err != nil || u.Hostname() == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			return Profile{}, fmt.Errorf("SSH 跳板 URI 无效")
		}
		user := ""
		if u.User != nil {
			if _, password := u.User.Password(); password {
				return Profile{}, fmt.Errorf("SSH 跳板 URI 不能包含密码")
			}
			user = u.User.Username()
		}
		p := NewProfile()
		p.Name = "跳板机"
		p.Host = u.Hostname()
		p.Username = user
		if u.Port() != "" {
			p.Port, err = strconv.Atoi(u.Port())
			if err != nil {
				return p, err
			}
		}
		if err := p.Validate(); err != nil {
			return p, err
		}
		if u.Port() == "" {
			p.Port = 0
		}
		return p, nil
	}
	p, err := parseJump(spec, "")
	if err != nil {
		return p, err
	}
	host := spec
	if i := strings.LastIndex(host, "@"); i >= 0 {
		host = host[i+1:]
	}
	if _, _, err := net.SplitHostPort(host); err != nil {
		p.Port = 0
	}
	return p, nil
}

// OpenSSH visits explicit -J hops in order. Only the first hop may contribute
// its own upstream route; later hops already have an explicit upstream (-J),
// which takes precedence over their ProxyJump/ProxyCommand config.
func resolveJumpRoute(ctx context.Context, p Profile, resolve func(context.Context, Profile) (ResolvedSSH, error)) ([]ResolvedSSH, error) {
	var pending []Profile
	if p.Proxy.Kind == "jump" {
		j := NewProfile()
		j.Name = "跳板机"
		j.Host = p.Proxy.Host
		j.Port = p.Proxy.Port
		j.Username, j.IdentityFile = p.Proxy.Username, p.Proxy.IdentityFile
		pending = append(pending, j)
	}
	parse := func(value string) ([]Profile, error) {
		if value == "" || strings.EqualFold(value, "none") {
			return nil, nil
		}
		var hops []Profile
		for _, spec := range strings.Split(value, ",") {
			j, err := parseConfiguredJump(spec)
			if err != nil {
				return nil, err
			}
			hops = append(hops, j)
		}
		return hops, nil
	}
	hops, err := parse(p.JumpHost)
	if err != nil {
		return nil, err
	}
	pending = append(pending, hops...)
	var result []ResolvedSSH
	visiting := map[string]bool{}
	var expand func([]Profile) error
	expand = func(hops []Profile) error {
		for i, hop := range hops {
			if err := ctx.Err(); err != nil {
				return err
			}
			if len(result)+len(visiting)+len(hops)-i > 32 {
				return fmt.Errorf("SSH 跳板链超过 32 级")
			}
			key := strings.ToLower(hop.Host) + "\x00" + strconv.Itoa(hop.Port) + "\x00" + hop.Username
			if visiting[key] {
				return fmt.Errorf("SSH 跳板配置存在循环")
			}
			visiting[key] = true
			hop.LegacySSH = p.LegacySSH
			r, err := resolve(ctx, hop)
			if err != nil {
				return err
			}
			if r.OriginalHost == "" {
				r.OriginalHost = hop.Host
			}
			check := r.Profile
			check.JumpHost = ""
			if err := check.Validate(); err != nil {
				return err
			}
			if i == 0 && len(result) == 0 {
				upstream, err := parse(r.Profile.JumpHost)
				if err != nil {
					return err
				}
				if len(upstream) > 0 {
					if err := expand(upstream); err != nil {
						return err
					}
					r.ProxyCommand = ""
				}
			} else {
				r.ProxyCommand = ""
			}
			delete(visiting, key)
			result = append(result, r)
		}
		return nil
	}
	if err := expand(pending); err != nil {
		return nil, err
	}
	return result, nil
}

package core

import (
	"context"
	"net"
	"net/netip"
	"strings"
)

func FileSessionLiteral(host string) string {
	ip, err := netip.ParseAddr(strings.Trim(host, "[]"))
	if err != nil {
		return ""
	}
	return ip.Unmap().String()
}
func uniqueFileAddress(addresses []string) string {
	unique := ""
	for _, raw := range addresses {
		if ip := FileSessionLiteral(raw); ip != "" {
			if unique != "" && unique != ip {
				return ""
			}
			unique = ip
		}
	}
	return unique
}
func FileSessionAddress(ctx context.Context, p Profile) string {
	host := p.Host
	if p.Kind == "ssh" || p.Kind == "sftp" {
		if resolved, err := ResolveSSH(ctx, p); err == nil {
			host = resolved.Profile.Host
		}
	}
	if ip := FileSessionLiteral(host); ip != "" {
		return ip
	}
	addresses, err := net.DefaultResolver.LookupHost(ctx, host)
	if err != nil {
		return ""
	}
	return uniqueFileAddress(addresses)
}

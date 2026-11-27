package core

import (
	"fmt"
	"slices"
)

// Nil preserves the client's existing automatic policy; a present list is an
// ordered allowlist shared with native OShell, including unavailable methods.
func (p Profile) validateSSHAuthenticationMethods() error {
	if p.SSHAuthenticationMethods == nil {
		return nil
	}
	if (p.Kind != "ssh" && p.Kind != "sftp") || len(p.SSHAuthenticationMethods) == 0 || len(p.SSHAuthenticationMethods) > 4 {
		return fmt.Errorf("SSH / SFTP 会话须选择 1–4 种身份验证方式")
	}
	for i, method := range p.SSHAuthenticationMethods {
		switch method {
		case "password", "publickey", "keyboard-interactive", "gssapi-with-mic":
		default:
			return fmt.Errorf("SSH 身份验证方式无效")
		}
		if slices.Contains(p.SSHAuthenticationMethods[:i], method) {
			return fmt.Errorf("SSH 身份验证方式不能重复")
		}
	}
	return nil
}

func (p Profile) allowsSSHAuthentication(method string) bool {
	return p.SSHAuthenticationMethods == nil || slices.Contains(p.SSHAuthenticationMethods, method)
}

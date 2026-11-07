package core

import (
	"bytes"
	"fmt"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// HostKeyMismatch retains the actual handshake address (which may be a jump
// host), rather than deriving the cleanup target from a saved session profile.
type HostKeyMismatch struct {
	Address, Algorithm, Fingerprint string
	path                            string
	before                          []byte
	wanted                          []knownhosts.KnownKey
	cause                           error
}

func (e *HostKeyMismatch) Error() string {
	return fmt.Sprintf("主机密钥不匹配，连接已阻止。请核实服务器身份后清理 %s 的旧记录：%s: %v", e.Address, e.path, e.cause)
}
func (e *HostKeyMismatch) Unwrap() error { return e.cause }
func (e *HostKeyMismatch) PreviousFingerprints() []string {
	var result []string
	for _, k := range e.wanted {
		result = append(result, k.Key.Type()+" · "+ssh.FingerprintSHA256(k.Key))
	}
	return result
}

// RemoveHostKey removes only the conflicting address. A changed file requires
// a fresh connection/confirmation, so an old dialog cannot erase new trust.
func (t *TrustStore) RemoveHostKey(e *HostKeyMismatch) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if e == nil || e.path != t.Path {
		return fmt.Errorf("主机密钥记录不属于当前信任文件")
	}
	data, err := ReadBounded(t.Path)
	if err != nil {
		return err
	}
	if !bytes.Equal(data, e.before) {
		return fmt.Errorf("主机密钥记录已变化，请重新连接后再确认清理")
	}
	lines := bytes.SplitAfter(data, []byte("\n"))
	target := knownhosts.Normalize(e.Address)
	for _, k := range e.wanted {
		if k.Line < 1 || k.Line > len(lines) {
			return fmt.Errorf("主机密钥行已变化，请重新连接")
		}
		line := lines[k.Line-1]
		marker, hosts, publicKey, _, _, parseErr := ssh.ParseKnownHosts(line)
		if parseErr != nil || marker != "" || !bytes.Equal(publicKey.Marshal(), k.Key.Marshal()) {
			return fmt.Errorf("该记录包含特殊信任规则或已变化，请手动检查 %s", t.Path)
		}
		var keep []string
		removed := false
		for _, host := range hosts {
			if host == target || len(hosts) == 1 && strings.HasPrefix(host, "|1|") {
				removed = true
			} else {
				keep = append(keep, host)
			}
		}
		if !removed {
			return fmt.Errorf("该主机使用共享或通配信任规则，不能单独清理，请手动检查 %s", t.Path)
		}
		if len(keep) == 0 {
			lines[k.Line-1] = nil
			continue
		}
		// Preserve other aliases, key bytes, comments and line endings exactly.
		start := bytes.Index(line, []byte(strings.Join(hosts, ",")))
		if start < 0 {
			return fmt.Errorf("无法识别主机记录，请手动检查 %s", t.Path)
		}
		end := start + len(strings.Join(hosts, ","))
		next := append([]byte{}, line[:start]...)
		next = append(next, []byte(strings.Join(keep, ","))...)
		lines[k.Line-1] = append(next, line[end:]...)
	}
	if len(e.wanted) == 0 {
		return fmt.Errorf("没有可清理的旧主机密钥")
	}
	if err = AtomicWrite(t.Path+".before-cleanup", data); err != nil {
		return fmt.Errorf("备份主机密钥失败：%w", err)
	}
	return AtomicWrite(t.Path, bytes.Join(lines, nil))
}

package core

import (
	"bytes"
	"fmt"
	"os"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// PreferHostKeyAlgorithms changes only negotiation order. Callback still checks
// the actual key against the current trust file after the handshake, so neither
// an algorithm match nor this snapshot grants trust to a new/replaced key.
func (t *TrustStore) PreferHostKeyAlgorithms(address string, cfg *ssh.ClientConfig) error {
	t.mu.Lock()
	data, err := ReadBounded(t.Path)
	t.mu.Unlock()
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	keys, err := knownhosts.KnownHostKeys(bytes.NewReader(data), address)
	if err != nil {
		return fmt.Errorf("读取主机密钥算法：%w", err)
	}
	if len(keys) == 0 {
		return nil
	}
	types := make(map[string]bool, len(keys))
	for _, key := range keys {
		types[key.Type()] = true
	}
	base := cfg.HostKeyAlgorithms
	if base == nil {
		base = ssh.DefaultHostKeyAlgorithms()
	}
	preferred, remaining := make([]string, 0, len(base)), make([]string, 0, len(base))
	for _, algorithm := range base {
		keyType := algorithm
		switch algorithm {
		case ssh.KeyAlgoRSASHA256, ssh.KeyAlgoRSASHA512:
			keyType = ssh.KeyAlgoRSA
		case ssh.CertAlgoRSASHA256v01, ssh.CertAlgoRSASHA512v01:
			keyType = ssh.CertAlgoRSAv01
		}
		if types[keyType] {
			preferred = append(preferred, algorithm)
		} else {
			remaining = append(remaining, algorithm)
		}
	}
	if len(preferred) > 0 {
		cfg.HostKeyAlgorithms = append(preferred, remaining...)
	}
	return nil
}

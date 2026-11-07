package core

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func bastionKeepAlivePath(dir, endpoint string) string {
	return filepath.Join(dir, "bastion-keepalive", fmt.Sprintf("%x.json", sha256.Sum256([]byte(endpoint))))
}

func LoadBastionKeepAlive(dir string, p *Profile, defaults KeepAliveSettings) error {
	p.KeepAlive = defaults
	data, err := ReadBounded(bastionKeepAlivePath(dir, p.RuntimeBastionEndpoint))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err = json.Unmarshal(data, &p.KeepAlive); err != nil {
		return err
	}
	return p.KeepAlive.Validate()
}

func SaveBastionKeepAlive(dir, endpoint string, k KeepAliveSettings) error {
	if err := k.Validate(); err != nil {
		return err
	}
	dest := bastionKeepAlivePath(dir, endpoint)
	if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(k)
	if err != nil {
		return err
	}
	return AtomicWrite(dest, data)
}

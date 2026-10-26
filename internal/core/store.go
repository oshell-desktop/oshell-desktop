// SPDX-License-Identifier: GPL-3.0-only
package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
)

const MaxArchive = 16 << 20

type Store struct {
	configurationGeneration atomic.Uint64
	Portable                bool
	baseline                *Config
	observed                bool
	observedRevision        string
	protectionState         atomic.Uint32
	Dir                     string
	bootstrapDir            string
	StorageWarning          string
	mu                      sync.Mutex
	pending                 bool
	protected               bool
	master                  []byte
}

func OpenStore() (*Store, error) {
	base, portable, err := defaultStorageDirectory()
	if err != nil {
		return nil, err
	}
	return openStoreDirectory(base, portable)
}
func openStoreDirectory(base string, portable bool) (*Store, error) {
	if err := os.MkdirAll(base, 0700); err != nil {
		return nil, err
	}
	dir, warning := base, ""
	if !portable {
		var err error
		dir, warning, err = activateStorageLocation(base)
		if err != nil {
			return nil, err
		}
	} else {
		probe, err := os.CreateTemp(base, ".oshell-write-check-")
		if err != nil {
			return nil, fmt.Errorf("便携模式需要可写目录，请先完整解压到可写文件夹：%w", err)
		}
		name := probe.Name()
		probe.Close()
		os.Remove(name)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	if err := recoverTransaction(dir); err != nil {
		return nil, err
	}
	return &Store{Dir: dir, bootstrapDir: base, StorageWarning: warning, Portable: portable}, nil
}
func (s *Store) Load() (Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.publishConfigurationState()
	c := DefaultConfig()
	data, err := readConfigurationFile(filepath.Join(s.Dir, "config.json"))
	if os.IsNotExist(err) {
		if s.protected || s.observed && s.observedRevision != "" {
			return c, fmt.Errorf("配置已被移走或删除")
		}
		s.observed = true
		s.observedRevision = ""
		base := c.Clone()
		s.baseline = &base
		return c, nil
	}
	if err != nil {
		return c, err
	}
	defer clear(data)
	if s.protected && !IsSharedVault(data) {
		return c, fmt.Errorf("受保护配置被替换为未加密内容，已停止载入")
	}
	if IsSharedVault(data) {
		s.protected = true
	}
	c, _, err = DecodeConfiguration(data, s.master)
	if err != nil {
		return c, fmt.Errorf("配置文件无法读取，原文件已保留: %w", err)
	}
	s.observeConfiguration(data, c)
	return c, nil
}
func (s *Store) Save(c Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer s.publishConfigurationState()
	if s.pending {
		return ErrPendingTransaction
	}
	c.Version = 3
	c.NormalizeLinkRootOrder()
	if err := c.Validate(); err != nil {
		return err
	}
	// Preserve the pre-upgrade configuration before the first v2 write.
	file := filepath.Join(s.Dir, "config.json")
	expectedRevision := ""
	if existing, err := readConfigurationFile(file); err == nil {
		var header struct {
			Version int
			Format  string
		}
		if json.Unmarshal(existing, &header) == nil && header.Format == "OShell.shared-vault" {
			s.protected = true
			if len(s.master) == 0 {
				return ErrConfigurationLocked
			}
		}
		expectedRevision = configurationDigest(existing)
		if !s.observed || s.observedRevision != expectedRevision {
			return ErrConfigurationChanged
		}
		if json.Unmarshal(existing, &header) == nil && header.Format == "" && (header.Version == 1 || header.Version == 2) {
			backup := filepath.Join(s.Dir, fmt.Sprintf("config-v%d.backup.json", header.Version))
			if _, e := os.Stat(backup); os.IsNotExist(e) {
				if e = AtomicWrite(backup, existing); e != nil {
					return fmt.Errorf("备份旧配置失败: %w", e)
				}
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	} else if s.observed && s.observedRevision != "" {
		return ErrConfigurationChanged
	}

	master, err := s.encodingMasterLocked()
	if err != nil {
		return err
	}
	data, err := EncodeConfiguration(c, master)
	if err != nil {
		return err
	}
	release, lockErr := lockStateDirectory(context.Background(), s.Dir, false)
	if lockErr != nil {
		return lockErr
	}
	defer release()
	if err = s.checkConfigurationRevision(expectedRevision); err != nil {
		return err
	}
	if err = AtomicWrite(filepath.Join(s.Dir, "config.json"), data); err != nil {
		return err
	}
	s.observeConfiguration(data, c)
	return nil
}
func AtomicWrite(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".oshell-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return replaceStateFile(name, path)
}
func ReadBounded(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("请选择普通文件")
	}
	b, err := io.ReadAll(io.LimitReader(f, MaxArchive+1))
	if len(b) > MaxArchive {
		return nil, fmt.Errorf("文件超过 16 MiB")
	}
	return b, err
}

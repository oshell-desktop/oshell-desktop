// SPDX-License-Identifier: GPL-3.0-only
package core

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

var ErrPendingTransaction = errors.New("配置事务已经记录，但尚未完整写入；请重新启动以恢复，期间暂停配置修改")

type stateTransaction struct {
	Version int    `json:"version"`
	Config  []byte `json:"config"`
	Vault   []byte `json:"vault"`
}

const transactionFile = "state-transaction.json"

func (s *Store) Revision() (string, error) {
	data, e := readConfigurationFile(filepath.Join(s.Dir, "config.json"))
	if os.IsNotExist(e) {
		return "", nil
	}
	if e != nil {
		return "", e
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}
func recoverTransaction(dir string) error {
	release, err := lockStateDirectory(context.Background(), dir, true)
	if err != nil {
		return err
	}
	defer release()
	return recoverTransactionLocked(dir)
}

func recoverTransactionLocked(dir string) error {
	file := filepath.Join(dir, transactionFile)
	f, e := os.Open(file)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	data, readErr := io.ReadAll(io.LimitReader(f, 48<<20+1))
	// Windows cannot remove an open journal. Close before publishing or removing
	// it, including on read failures, so a later recovery can retry safely.
	closeErr := f.Close()
	if readErr != nil {
		return fmt.Errorf("读取配置事务: %w", readErr)
	}
	if closeErr != nil {
		return fmt.Errorf("关闭配置事务: %w", closeErr)
	}
	if len(data) > 48<<20 {
		return fmt.Errorf("配置事务大小无效")
	}
	var tx stateTransaction
	if e = json.Unmarshal(data, &tx); e != nil {
		return e
	}
	if tx.Version != 1 || len(tx.Config) > MaxSharedVault || len(tx.Vault) > MaxArchive || !validTransactionConfiguration(tx.Config) || !validVaultFile(tx.Vault) {
		return fmt.Errorf("配置事务内容无效")
	}
	if e = AtomicWrite(filepath.Join(dir, "credentials.json"), tx.Vault); e != nil {
		return e
	}
	if e = AtomicWrite(filepath.Join(dir, "config.json"), tx.Config); e != nil {
		return e
	}
	return os.Remove(file)
}

// Credentials are decrypted and validated before this point. The durable intent
// contains only the normal configuration and locally encrypted credential data.
func (s *Store) CommitCredentials(ctx context.Context, cfg Config, v *Vault, updates []CredentialUpdate, expectedRevision string) error {
	return s.commitCredentials(ctx, cfg, v, updates, expectedRevision, "")
}
func (s *Store) commitCredentials(ctx context.Context, cfg Config, v *Vault, updates []CredentialUpdate, expectedRevision, expectedVaultRevision string, resolutionGuards ...func() (func(), error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v.mu.Lock()
	defer v.mu.Unlock()
	if expectedVaultRevision != "" && v.revisionLocked() != expectedVaultRevision {
		return ErrVaultChanged
	}
	if s.pending || v.blocked {
		return ErrPendingTransaction
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	revision, e := s.Revision()
	if e != nil {
		return e
	}
	if revision != expectedRevision {
		return ErrConfigurationChanged
	}
	if e = cfg.Validate(); e != nil {
		return e
	}
	if v.data.Master && len(v.key) == 0 {
		return ErrVaultLocked
	}
	if len(v.key) == 0 {
		v.key, e = localVaultKey(v.dir)
		if e != nil {
			return e
		}
	}
	next := v.data
	next.Entries = map[string]vaultEntry{}
	for id, entry := range v.data.Entries {
		next.Entries[id] = entry
	}
	for _, change := range updates {
		if e = ctx.Err(); e != nil {
			return e
		}
		if change.Delete {
			delete(next.Entries, change.Profile.ID)
			continue
		}
		identity := change.Identity
		local, err := v.localProtectionLocked(change.Profile.ID, change.Protection)
		if err != nil {
			return err
		}
		if old, ok := next.Entries[change.Profile.ID]; ok && old.Identity == credentialIdentity(change.Profile) && old.Resolved != nil && *old.Resolved == identity && old.Local == local {
			plain, e := v.decryptEntryLocked(change.Profile.ID, old)
			same := e == nil && subtle.ConstantTimeCompare(plain, []byte(change.Password)) == 1
			clear(plain)
			if same {
				continue
			}
		}
		entry := vaultEntry{Identity: credentialIdentity(change.Profile), Resolved: &identity, Local: local}
		entry.Data, e = v.encryptEntryLocked(change.Profile.ID, entry, []byte(change.Password))
		if e != nil {
			return e
		}
		next.Entries[change.Profile.ID] = entry
	}
	pruneBastionTargets(next.Entries, cfg.Profiles)
	next.Check, e = seal(v.key, []byte("OShell"), "OShell vault check v1")
	if e != nil {
		return e
	}
	storageMaster, e := s.encodingMasterLocked()
	if e != nil {
		return e
	}
	cfgBytes, e := EncodeConfiguration(cfg, storageMaster)
	if e != nil {
		return e
	}
	vaultBytes, e := marshalVaultFile(next, v.key, v.dir)
	if e != nil {
		return e
	}
	if len(cfgBytes) > MaxSharedVault || len(vaultBytes) > MaxArchive {
		return fmt.Errorf("配置或密码库超过 16 MiB")
	}
	txBytes, e := json.Marshal(stateTransaction{1, cfgBytes, vaultBytes})
	if e != nil {
		return e
	}
	if len(txBytes) > 48<<20 {
		return fmt.Errorf("配置事务过大")
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	if len(resolutionGuards) != 0 {
		release, err := resolutionGuards[0]()
		if err != nil {
			return err
		}
		defer release()
	}
	release, lockErr := lockStateDirectory(ctx, s.Dir, false)
	if lockErr != nil {
		return lockErr
	}
	defer release()
	if e = v.checkDiskRevisionLocked(); e != nil {
		return e
	}
	if len(resolutionGuards) == 0 {
		if e = s.checkConfigurationRevision(expectedRevision); e != nil {
			return e
		}
	} else {
		// The guard validated the original observed baseline. Resolution deliberately
		// publishes against the newer disk baseline, checked again under the lock.
		current, err := s.Revision()
		if err != nil {
			return err
		}
		if current != expectedRevision {
			return ErrConfigurationChanged
		}
	}
	if e = AtomicWrite(filepath.Join(s.Dir, transactionFile), txBytes); e != nil {
		return e
	}
	v.data = next
	if e = recoverTransactionLocked(s.Dir); e != nil {
		s.pending = true
		v.blocked = true
		return fmt.Errorf("%w: %v", ErrPendingTransaction, e)
	}
	v.diskRevision = configurationDigest(vaultBytes)
	s.observeConfiguration(cfgBytes, cfg)
	return nil
}

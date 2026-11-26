package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type LocalResolutionPlan struct {
	baseUnits                                         map[string]json.RawMessage
	Merge                                             *LocalConfigMerge
	Base, Local, Remote                               Config
	ObservedRevision, DiskRevision, DiskVaultRevision string
	original                                          *Vault
	originalRevision                                  string
	localVault, remoteVault                           *Vault
	mu                                                sync.Mutex
	closed                                            bool
}

func (p *LocalResolutionPlan) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	for _, v := range []*Vault{p.localVault, p.remoteVault} {
		if v != nil {
			clear(v.key)
			v.key = nil
			clear(v.master)
			v.master = nil
		}
	}
}
func (s *Store) PrepareLocalResolution(ctx context.Context, local Config, v *Vault, changes []CredentialUpdate) (*LocalResolutionPlan, error) {
	if v == nil {
		return nil, fmt.Errorf("密码库不可用")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	v.mu.Lock()
	defer v.mu.Unlock()
	if _, err := os.Lstat(filepath.Join(s.Dir, transactionFile)); err == nil {
		return nil, fmt.Errorf("磁盘配置事务正在完成，请稍后重试")
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if s.baseline == nil || s.pending || v.blocked {
		return nil, fmt.Errorf("没有可用于合并的配置基线")
	}
	if v.data.Master && len(v.key) == 0 {
		return nil, ErrVaultLocked
	}
	plan := &LocalResolutionPlan{Base: s.baseline.Clone(), Local: local.Clone(), ObservedRevision: s.observedRevision, original: v, originalRevision: v.revisionLocked()}
	ok := false
	defer func() {
		if !ok {
			plan.Close()
		}
	}()
	clone := &Vault{dir: v.dir, data: v.data, key: append([]byte(nil), v.key...), master: append([]byte(nil), v.master...)}
	clone.data.Entries = map[string]vaultEntry{}
	for id, entry := range v.data.Entries {
		clone.data.Entries[id] = entry
	}
	plan.localVault = clone
	baseUnits, _, err := credentialConfigUnits(plan.Base, v)
	if err != nil {
		return nil, err
	}
	for _, change := range changes {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if change.Delete {
			delete(clone.data.Entries, change.Profile.ID)
			continue
		}
		localProtection, err := clone.localProtectionLocked(change.Profile.ID, change.Protection)
		if err != nil {
			return nil, err
		}
		if len(clone.key) == 0 && !clone.data.Master {
			clone.key, err = localVaultKey(clone.dir)
			if err != nil {
				return nil, err
			}
		}
		identity := change.Identity
		entry := vaultEntry{Identity: credentialIdentity(change.Profile), Resolved: &identity, Local: localProtection}
		entry.Data, err = clone.encryptEntryLocked(change.Profile.ID, entry, []byte(change.Password))
		if err != nil {
			return nil, err
		}
		clone.data.Entries[change.Profile.ID] = entry
	}
	raw, err := readConfigurationFile(filepath.Join(s.Dir, "config.json"))
	if err != nil {
		return nil, err
	}
	defer clear(raw)
	plan.DiskRevision = configurationDigest(raw)
	plan.Remote, _, err = DecodeConfiguration(raw, s.master)
	if err != nil {
		return nil, err
	}
	vaultRaw, err := ReadBounded(filepath.Join(s.Dir, "credentials.json"))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil {
		plan.DiskVaultRevision = configurationDigest(vaultRaw)
	}
	clear(vaultRaw)
	remote, err := OpenVault(s.Dir)
	if err != nil {
		return nil, err
	}
	plan.remoteVault = remote
	if remote.data.Master {
		if err = remote.unlockLocked(string(v.master)); err != nil {
			return nil, err
		}
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	after, err := s.Revision()
	if err != nil || after != plan.DiskRevision {
		return nil, ErrConfigurationChanged
	}
	vaultRaw, err = ReadBounded(filepath.Join(s.Dir, "credentials.json"))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	afterVault := ""
	if err == nil {
		afterVault = configurationDigest(vaultRaw)
	}
	clear(vaultRaw)
	if _, err := os.Lstat(filepath.Join(s.Dir, transactionFile)); err == nil {
		return nil, ErrConfigurationChanged
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if afterVault != plan.DiskVaultRevision {
		return nil, ErrConfigurationChanged
	}
	localUnits, order, err := credentialConfigUnits(plan.Local, clone)
	if err != nil {
		return nil, err
	}
	remoteUnits, extra, err := credentialConfigUnits(plan.Remote, remote)
	if err != nil {
		return nil, err
	}
	plan.baseUnits = baseUnits
	plan.Merge = mergeLocalUnits(baseUnits, localUnits, remoteUnits, order, extra)
	ok = true
	return plan, nil
}
func (v *Vault) credentialVersionLocked(profile Profile) string {
	entry, ok := v.data.Entries[profile.ID]
	if !ok || entry.Identity != credentialIdentity(profile) {
		return ""
	}
	raw, _ := json.Marshal(entry)
	return configurationDigest(raw)
}

// CredentialVersion lets editors detect a replacement without decrypting the password.
func (v *Vault) CredentialVersion(profile Profile) string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.credentialVersionLocked(profile)
}

func credentialConfigUnits(cfg Config, v *Vault) (map[string]json.RawMessage, []string, error) {
	units, order, err := localConfigUnits(cfg)
	if err != nil {
		return nil, nil, err
	}
	inject := func(key, value string) {
		var unit map[string]json.RawMessage
		_ = json.Unmarshal(units[key], &unit)
		unit["_credentialVersion"], _ = json.Marshal(value)
		units[key], _ = json.Marshal(unit)
	}
	for _, p := range cfg.Profiles {
		signature := v.credentialVersionLocked(p)
		if p.ProxyID == "" && p.Proxy.Kind != "" && p.Proxy.Kind != "none" {
			signature += "/" + v.credentialVersionLocked(ProxyCredentialProfile(p.Proxy))
		}
		if target, ok := p.BastionTarget(); ok {
			if version := v.credentialVersionLocked(target.Credential); version != "" {
				signature += "/target:" + version
			}
		}
		inject("profile:"+strings.ToUpper(p.ID), signature)
	}
	for _, p := range cfg.Proxies {
		inject("proxy:"+strings.ToUpper(p.ID()), v.credentialVersionLocked(ProxyCredentialProfile(p.Settings)))
	}
	return units, order, nil
}

// ResolveCredentials binds each selected unit to its own vault snapshot. Returned
// passwords are transient; the caller must clear them after the commit attempt.
func (p *LocalResolutionPlan) ResolveCredentials(choices map[string]SharedMergeChoice) (Config, []CredentialUpdate, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.resolveCredentialsLocked(choices)
}
func (p *LocalResolutionPlan) resolveCredentialsLocked(choices map[string]SharedMergeChoice) (Config, []CredentialUpdate, error) {
	if p.closed {
		return Config{}, nil, fmt.Errorf("冲突计划已关闭")
	}
	cfg, err := p.Merge.Resolve(choices)
	if err != nil {
		return cfg, nil, err
	}
	var updates []CredentialUpdate
	success := false
	defer func() {
		if !success {
			for i := range updates {
				updates[i].Password = ""
			}
		}
	}()
	seen := map[string]bool{}
	appendCredential := func(profile Profile, key string) error {
		source := p.localVault
		if p.Merge.unitSource(key, choices) == SharedKeepRemote {
			source = p.remoteVault
		}
		update, exists, err := source.PrepareCredentialCopy(profile, profile)
		if err != nil {
			return err
		}
		if !exists {
			update = CredentialUpdate{Profile: profile, Delete: true}
		}
		updates = append(updates, update)
		seen[profile.ID] = true
		return nil
	}
	for _, profile := range cfg.Profiles {
		key := "profile:" + strings.ToUpper(profile.ID)
		if err = appendCredential(profile, key); err != nil {
			return Config{}, nil, err
		}
		if target, ok := profile.BastionTarget(); ok {
			if err = appendCredential(target.Credential, key); err != nil {
				return Config{}, nil, err
			}
		}
		if profile.ProxyID == "" && profile.Proxy.Kind != "" && profile.Proxy.Kind != "none" {
			if err = appendCredential(ProxyCredentialProfile(profile.Proxy), key); err != nil {
				return Config{}, nil, err
			}
		}
	}
	for _, proxy := range cfg.Proxies {
		if err = appendCredential(ProxyCredentialProfile(proxy.Settings), "proxy:"+strings.ToUpper(proxy.ID())); err != nil {
			return Config{}, nil, err
		}
	}
	for _, previous := range []Config{p.Base, p.Local, p.Remote} {
		var profiles []Profile
		profiles = append(profiles, previous.Profiles...)
		for _, profile := range previous.Profiles {
			if profile.ProxyID == "" && profile.Proxy.Kind != "" && profile.Proxy.Kind != "none" {
				profiles = append(profiles, ProxyCredentialProfile(profile.Proxy))
			}
		}
		for _, proxy := range previous.Proxies {
			profiles = append(profiles, ProxyCredentialProfile(proxy.Settings))
		}
		for _, profile := range profiles {
			if !seen[profile.ID] {
				updates = append(updates, CredentialUpdate{Profile: profile, Delete: true})
				seen[profile.ID] = true
			}
		}
	}
	success = true
	return cfg, updates, nil
}

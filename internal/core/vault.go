// SPDX-License-Identifier: GPL-3.0-only
package core

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"

	"golang.org/x/crypto/scrypt"
)

var ErrVaultLocked = errors.New("密码库已锁定，请先解锁主密码")

type vaultEntry struct {
	Local    bool         `json:"local,omitempty"`
	Resolved *SSHIdentity `json:"resolved,omitempty"`
	Identity string       `json:"identity"`
	Data     []byte       `json:"data"`
}
type vaultData struct {
	Version int                   `json:"version"`
	Master  bool                  `json:"master"`
	Salt    []byte                `json:"salt,omitempty"`
	Check   []byte                `json:"check"`
	Entries map[string]vaultEntry `json:"entries"`
}
type Vault struct {
	diskRevision   string
	sealedEntries  []byte
	legacyMetadata bool
	blocked        bool
	state          atomic.Uint32
	mu             sync.Mutex
	dir            string
	data           vaultData
	key            []byte
	master         []byte
}

func credentialIdentity(p Profile) string {
	return p.Kind + "\n" + p.Host + "\n" + strconv.Itoa(p.Port) + "\n" + p.Username
}
func seal(key, plain []byte, aad string) ([]byte, error) {
	b, e := aes.NewCipher(key)
	if e != nil {
		return nil, e
	}
	g, e := cipher.NewGCM(b)
	if e != nil {
		return nil, e
	}
	nonce := make([]byte, g.NonceSize())
	if _, e = rand.Read(nonce); e != nil {
		return nil, e
	}
	return g.Seal(nonce, nonce, plain, []byte(aad)), nil
}
func unseal(key, data []byte, aad string) ([]byte, error) {
	b, e := aes.NewCipher(key)
	if e != nil {
		return nil, e
	}
	g, e := cipher.NewGCM(b)
	if e != nil {
		return nil, e
	}
	if len(data) < g.NonceSize() {
		return nil, fmt.Errorf("密码库密文损坏")
	}
	return g.Open(nil, data[:g.NonceSize()], data[g.NonceSize():], []byte(aad))
}
func OpenVault(dir string) (*Vault, error) {
	v := &Vault{dir: dir, data: vaultData{Version: 1, Entries: map[string]vaultEntry{}}}
	b, e := ReadBounded(filepath.Join(dir, "credentials.json"))
	if os.IsNotExist(e) {
		return v, nil
	}
	if e != nil {
		return nil, e
	}
	defer clear(b)
	v.diskRevision = configurationDigest(b)
	var header struct{ Version int }
	if json.Unmarshal(b, &header) != nil || !validVaultFile(b) {
		return nil, fmt.Errorf("不支持或损坏的密码库格式")
	}
	if header.Version == 2 {
		var stored protectedVaultFile
		if e = json.Unmarshal(b, &stored); e != nil {
			return nil, e
		}
		v.data = vaultData{Version: 1, Master: true, Salt: stored.Salt, Check: stored.Check, Entries: map[string]vaultEntry{}}
		v.sealedEntries = stored.ProtectedEntries
		if len(stored.LocalEntries) > 0 {
			if key, err := loadLocalVaultKey(dir); err == nil {
				entries, err := decodeVaultEntries(stored.LocalEntries, key, "OShell vault local metadata v2", true)
				clear(key)
				if err == nil {
					v.data.Entries = entries
				}
			}
		}

		v.state.Store(1)
		return v, nil
	}
	if e = json.Unmarshal(b, &v.data); e != nil {
		return nil, e
	}
	if v.data.Version != 1 || v.data.Entries == nil {
		return nil, fmt.Errorf("不支持的密码库格式")
	}
	if v.data.Master {
		v.legacyMetadata = true
		v.state.Store(1)
		if len(v.data.Salt) != 32 {
			return nil, fmt.Errorf("主密码盐值无效")
		}
		return v, nil
	}
	v.key, e = loadLocalVaultKey(dir)
	if e != nil {
		return nil, fmt.Errorf("本机凭据密钥不可用，未新建或覆盖密码库: %w", e)
	}
	if _, e = unseal(v.key, v.data.Check, "OShell vault check v1"); e != nil {
		return nil, fmt.Errorf("本机密钥与密码库不匹配")
	}
	return v, nil
}
func (v *Vault) State() (master, locked bool) {
	state := v.state.Load()
	return state&1 != 0, state == 1
}
func (v *Vault) Unlock(password string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if err := v.unlockLocked(password); err != nil {
		return err
	}
	if v.legacyMetadata {
		if err := v.write(v.data); err != nil {
			clear(v.key)
			v.key = nil
			clear(v.master)
			v.master = nil
			v.state.Store(1)
			return err
		}
		v.legacyMetadata = false
	}
	return nil
}

func (v *Vault) unlockLocked(password string) error {
	if !v.data.Master {
		return nil
	}
	key, e := scrypt.Key([]byte(password), v.data.Salt, 32768, 8, 1, 32)
	if e != nil {
		return e
	}
	if _, e = unseal(key, v.data.Check, "OShell vault check v1"); e != nil {
		clear(key)
		return fmt.Errorf("主密码错误或密码库已损坏")
	}
	if len(v.sealedEntries) > 0 {
		entries, err := decodeVaultEntries(v.sealedEntries, key, "OShell vault protected metadata v2", false)
		if err != nil {
			clear(key)
			return fmt.Errorf("密码库身份索引解密失败：%w", err)
		}

		v.data.Entries = entries
		clear(v.sealedEntries)
		v.sealedEntries = nil
	}
	clear(v.key)
	v.key = key
	clear(v.master)
	v.master = []byte(password)
	if v.data.Master {
		v.state.Store(3)
	} else {
		v.state.Store(0)
	}
	return nil
}
func (v *Vault) Lock() {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.data.Master {
		clear(v.key)
		v.key = nil
		clear(v.master)
		v.master = nil
		v.state.Store(1)
	}
}
func (v *Vault) Get(p Profile) (string, error) { return v.getFor(p, nil) }
func (v *Vault) GetFor(p Profile, identity SSHIdentity) (string, error) {
	return v.getFor(p, &identity)
}
func (v *Vault) getFor(p Profile, resolved *SSHIdentity) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	entry, ok := v.data.Entries[p.ID]
	if !ok {
		if len(v.sealedEntries) > 0 {
			return "", ErrVaultLocked
		}
		return "", nil
	}
	if entry.Identity != credentialIdentity(p) {
		return "", nil
	}
	if entry.Resolved != nil && (resolved == nil || *entry.Resolved != *resolved) {
		return "", nil
	}
	if entry.Resolved == nil && resolved != nil && p.Identity() != *resolved {
		return "", nil
	}
	if len(v.key) == 0 && !entry.Local {
		return "", ErrVaultLocked
	}
	b, e := v.decryptEntryLocked(p.ID, entry)
	if e != nil {
		return "", e
	}
	defer clear(b)
	return string(b), nil
}
func (v *Vault) write(data vaultData) error { return v.writeWithKey(data, v.key) }
func (v *Vault) writeWithKey(data vaultData, key []byte) error {
	if v.blocked {
		return ErrPendingTransaction
	}
	b, e := marshalVaultFile(data, key, v.dir)
	if e != nil {
		return e
	}
	if len(b) > MaxArchive {
		return fmt.Errorf("密码库过大")
	}
	release, lockErr := lockStateDirectory(context.Background(), v.dir, false)
	if lockErr != nil {
		return lockErr
	}
	defer release()
	if e = v.checkDiskRevisionLocked(); e != nil {
		return e
	}
	if e = AtomicWrite(filepath.Join(v.dir, "credentials.json"), b); e != nil {
		return e
	}
	v.diskRevision = configurationDigest(b)
	return nil
}
func (v *Vault) Put(p Profile, password string) error { return v.putFor(p, password, nil) }
func (v *Vault) PutFor(p Profile, password string, identity SSHIdentity) error {
	return v.putFor(p, password, &identity)
}

// PutForRevision refuses to overwrite credentials changed while authentication ran.
func (v *Vault) PutForRevision(p Profile, password string, identity SSHIdentity, revision string, protection ...string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if revision != v.revisionLocked() {
		return fmt.Errorf("密码库已变化，本次密码未保存")
	}
	return v.putForLocked(p, password, &identity, protection...)
}
func (v *Vault) putFor(p Profile, password string, resolved *SSHIdentity) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.putForLocked(p, password, resolved)
}
func (v *Vault) putForLocked(p Profile, password string, resolved *SSHIdentity, protection ...string) error {
	mode := ""
	if len(protection) > 0 {
		mode = protection[0]
	}
	local, err := v.localProtectionLocked(p.ID, mode)
	if err != nil {
		return err
	}
	if v.data.Master && len(v.key) == 0 {
		return ErrVaultLocked
	}
	if len(v.key) == 0 {
		key, e := localVaultKey(v.dir)
		if e != nil {
			return e
		}
		v.key = key
	}
	if old, ok := v.data.Entries[p.ID]; ok && old.Local == local && old.Identity == credentialIdentity(p) && ((old.Resolved == nil && resolved == nil) || (old.Resolved != nil && resolved != nil && *old.Resolved == *resolved)) {
		plain, e := v.decryptEntryLocked(p.ID, old)
		same := e == nil && subtle.ConstantTimeCompare(plain, []byte(password)) == 1
		clear(plain)
		if same {
			return nil
		}
	}
	data := v.data
	data.Entries = make(map[string]vaultEntry, len(v.data.Entries)+1)
	for k, e := range v.data.Entries {
		data.Entries[k] = e
	}
	identity := credentialIdentity(p)
	entry := vaultEntry{Identity: identity, Resolved: resolved, Local: local}
	b, e := v.encryptEntryLocked(p.ID, entry, []byte(password))
	if e != nil {
		return e
	}
	entry.Data = b
	data.Entries[p.ID] = entry
	data.Check, e = seal(v.key, []byte("OShell"), "OShell vault check v1")
	if e != nil {
		return e
	}
	if e = v.write(data); e != nil {
		return e
	}
	v.data = data
	return nil
}
func (v *Vault) Delete(ids []string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.deleteLocked(ids)
}

func (v *Vault) DeleteForRevision(id, revision string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.data.Master && len(v.key) == 0 {
		return ErrVaultLocked
	}
	if revision != v.revisionLocked() {
		return ErrVaultChanged
	}
	return v.deleteLocked([]string{id})
}

func (v *Vault) deleteLocked(ids []string) error {
	data := v.data
	data.Entries = map[string]vaultEntry{}
	for id, e := range v.data.Entries {
		data.Entries[id] = e
	}
	for _, id := range ids {
		delete(data.Entries, id)
	}
	if len(v.data.Check) == 0 {
		return nil
	}
	if e := v.write(data); e != nil {
		return e
	}
	v.data = data
	return nil
}

// Enable converts local entries; rotation preserves explicitly local entries.
func (v *Vault) SetMaster(password string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.setMasterLocked(password)
}
func (v *Vault) setMasterLocked(password string) error {
	data, key, err := v.prepareMasterLocked(password)
	if err != nil {
		return err
	}
	if err = v.writeWithKey(data, key); err != nil {
		clear(key)
		return err
	}
	v.acceptMasterLocked(data, key, password)
	return nil
}
func (v *Vault) prepareMasterLocked(password string) (vaultData, []byte, error) {
	if v.data.Master && len(v.key) == 0 {
		return vaultData{}, nil, ErrVaultLocked
	}
	data := vaultData{Version: 1, Master: password != "", Entries: map[string]vaultEntry{}}
	var key []byte
	var e error
	if password != "" {
		if len([]rune(password)) < 8 || len(password) > 1024 {
			return vaultData{}, nil, fmt.Errorf("主密码至少需要 8 个字符，且不超过 1024 字节")
		}
		data.Salt = make([]byte, 32)
		if _, e = rand.Read(data.Salt); e != nil {
			return vaultData{}, nil, e
		}
		key, e = scrypt.Key([]byte(password), data.Salt, 32768, 8, 1, 32)
	} else {
		key, e = localVaultKey(v.dir)
	}
	if e != nil {
		return vaultData{}, nil, e
	}
	for id, entry := range v.data.Entries {
		if entry.Local && v.data.Master {
			data.Entries[id] = entry
			continue
		}
		plain, e := v.decryptEntryLocked(id, entry)
		if e != nil {
			clear(key)
			return vaultData{}, nil, e
		}
		next := vaultEntry{Identity: entry.Identity, Resolved: entry.Resolved, Local: password == ""}
		encrypted, e := seal(key, plain, entryAAD(id, next))
		clear(plain)
		if e != nil {
			clear(key)
			return vaultData{}, nil, e
		}
		next.Data = encrypted
		data.Entries[id] = next
	}
	data.Check, e = seal(key, []byte("OShell"), "OShell vault check v1")
	if e != nil {
		clear(key)
		return vaultData{}, nil, e
	}
	return data, key, nil
}
func (v *Vault) acceptMasterLocked(data vaultData, key []byte, password string) {
	clear(v.sealedEntries)
	v.sealedEntries = nil
	v.legacyMetadata = false
	clear(v.key)
	v.key = key
	v.data = data
	clear(v.master)
	v.master = []byte(password)
	if v.data.Master {
		v.state.Store(3)
	} else {
		v.state.Store(0)
	}
}

func entryAAD(id string, e vaultEntry) string {
	aad := id + "\n" + e.Identity
	if e.Local {
		aad += "\nlocal-key:v1"
	}
	if e.Resolved != nil {
		aad += "\nresolved:" + e.Resolved.Host + "\n" + strconv.Itoa(e.Resolved.Port) + "\n" + e.Resolved.User
	}
	return aad
}
func (v *Vault) MasterPassword() ([]byte, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.data.Master || len(v.master) == 0 {
		return nil, ErrVaultLocked
	}
	return append([]byte(nil), v.master...), nil
}

func (v *Vault) CopyCredential(source, destination Profile) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	entry, ok := v.data.Entries[source.ID]
	if !ok {
		return nil
	}
	if entry.Identity != credentialIdentity(source) {
		return nil
	}
	if len(v.key) == 0 {
		return ErrVaultLocked
	}
	plain, e := v.decryptEntryLocked(source.ID, entry)
	if e != nil {
		return e
	}
	defer clear(plain)
	next := vaultEntry{Identity: credentialIdentity(destination), Resolved: entry.Resolved, Local: entry.Local}
	next.Data, e = v.encryptEntryLocked(destination.ID, next, plain)
	if e != nil {
		return e
	}
	data := v.data
	data.Entries = make(map[string]vaultEntry, len(v.data.Entries)+1)
	for k, v := range v.data.Entries {
		data.Entries[k] = v
	}
	data.Entries[destination.ID] = next
	if e = v.write(data); e != nil {
		return e
	}
	v.data = data
	return nil
}

func (v *Vault) HasCredential(p Profile) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	entry, ok := v.data.Entries[p.ID]
	return ok && entry.Identity == credentialIdentity(p) && len(entry.Data) > 0
}

func (v *Vault) revisionLocked() string {
	data, _ := json.Marshal(v.data)
	data = append(data, v.sealedEntries...)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
func (v *Vault) Revision() string { v.mu.Lock(); defer v.mu.Unlock(); return v.revisionLocked() }

// ChangeMaster verifies the current password even when the vault is unlocked.
func (v *Vault) ChangeMaster(current, next string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.data.Master {
		return fmt.Errorf("尚未设置主密码")
	}
	if current == "" {
		return fmt.Errorf("请输入当前主密码")
	}
	if current == next {
		return fmt.Errorf("新主密码不能与原主密码相同")
	}
	if err := v.unlockLocked(current); err != nil {
		return err
	}
	return v.setMasterLocked(next)
}

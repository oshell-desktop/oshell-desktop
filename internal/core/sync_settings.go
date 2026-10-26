package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type SyncOptions struct {
	Policy                   SyncPolicy        `json:"policy,omitzero"`
	TrustedServerCertificate WebDAVCertificate `json:"trustedServerCertificate,omitzero"`
	RequireExisting          bool              `json:"requireExisting,omitempty"`
	Version                  int               `json:"version"`
	Enabled                  bool              `json:"enabled"`
	Backend                  string            `json:"backend"`
	Address                  string            `json:"address"`
	Username                 string            `json:"username,omitempty"`
}

var ErrSyncSettingsLocked = errors.New("WebDAV 设置已加密，请先解锁主密码")

func LoadSyncOptions(dir string, master ...[]byte) (SyncOptions, error) {
	options := SyncOptions{Version: 1, Backend: "webdav"}
	data, e := ReadBounded(filepath.Join(dir, "sync-settings.json"))
	if os.IsNotExist(e) {
		return options, nil
	}
	if e != nil {
		return options, e
	}
	if IsSharedVault(data) {
		if len(master) == 0 || len(master[0]) == 0 {
			return options, ErrSyncSettingsLocked
		}
		plain, err := OpenShared(data, master[0])
		if err != nil {
			return options, err
		}
		defer clear(plain)
		data = plain
	}
	if e = json.Unmarshal(data, &options); e != nil || options.Version != 1 {
		return options, fmt.Errorf("同步设置格式无效")
	}
	if e = options.Policy.Validate(); e != nil {
		return options, e
	}
	return options, nil
}
func (o SyncOptions) Target() string {
	target := o.Backend + "\n" + strings.TrimSpace(o.Address) + "\n" + o.Username
	content := o.Policy.Effective().Content
	if content != DefaultSyncPolicy().Content {
		target += fmt.Sprintf("\nscope:%t:%t:%t:%t", content.Sessions, content.ClientSettings, content.QuickCommands, content.SessionDefaults)
	}
	return target
}
func (o SyncOptions) CredentialProfile() Profile {
	p := NewProfile()
	p.ID = "81E40DA6-0A65-4F9B-BA44-7C794F10C754"
	p.Kind = "local"
	p.Host = o.Address
	p.Username = o.Username
	p.Name = "WebDAV sync service"
	return p
}
func (o SyncOptions) Transport(password, localDirectory string) (SyncTransport, error) {
	switch o.Backend {
	case "webdav":
		w, err := NewWebDAVTransport(o.Address, o.Username, password, false)
		if err == nil && w.directory.Scheme == "https" && o.TrustedServerCertificate != (WebDAVCertificate{}) {
			err = w.trustCertificate(o.TrustedServerCertificate)
		}
		return w, err
	case "directory":
		path, e := filepath.Abs(strings.TrimSpace(o.Address))
		if e != nil {
			return nil, e
		}
		path, e = filepath.EvalSymlinks(path)
		if e != nil {
			return nil, fmt.Errorf("同步目录不可用")
		}
		local, e := filepath.Abs(localDirectory)
		if e != nil {
			return nil, e
		}
		local, e = filepath.EvalSymlinks(local)
		if e != nil {
			return nil, e
		}
		if syncDirectoriesOverlap(path, local) {
			return nil, fmt.Errorf("同步目录必须与本机配置目录相互独立")
		}
		d := DirectoryTransport{Directory: path}
		if e = d.check(); e != nil {
			return nil, e
		}
		return d, nil
	default:
		return nil, fmt.Errorf("请选择同步方式")
	}
}
func SaveSyncOptions(dir string, o SyncOptions, master ...[]byte) error {
	release, err := lockStateDirectory(context.Background(), dir, false)
	if err != nil {
		return err
	}
	defer release()
	return saveSyncOptionsLocked(dir, o, master...)
}
func saveSyncOptionsLocked(dir string, o SyncOptions, master ...[]byte) error {
	if err := o.Policy.Validate(); err != nil {
		return err
	}
	if o.Version != 1 {
		return fmt.Errorf("同步设置版本无效")
	}
	if o.Backend != "webdav" || !o.Enabled || IsHTTPWebDAV(o.Address) {
		o.TrustedServerCertificate = WebDAVCertificate{}
	}
	if o.TrustedServerCertificate != (WebDAVCertificate{}) {
		if err := o.TrustedServerCertificate.Validate(o.Address); err != nil {
			return err
		}
	}
	data, e := json.MarshalIndent(o, "", "  ")
	if e != nil {
		return e
	}
	previous, err := LoadSyncOptions(dir, master...)
	if err != nil {
		return err
	}
	if o.Backend == "webdav" {
		if o.Enabled {
			if len(master) == 0 || len(master[0]) == 0 {
				return ErrSyncSettingsLocked
			}
			plain := data
			data, e = SealShared(plain, master[0])
			clear(plain)
			if e != nil {
				return e
			}
		} else {
			// Native OShell removes disabled WebDAV login settings.
			o.Address = ""
			o.Username = ""
			o.TrustedServerCertificate = WebDAVCertificate{}
			data, e = json.MarshalIndent(o, "", "  ")
			if e != nil {
				return e
			}
		}
	}
	type archivedState struct{ old, archived string }
	var archived []archivedState
	restore := func() error {
		var failures []error
		for i := len(archived) - 1; i >= 0; i-- {
			failures = append(failures, os.Rename(archived[i].archived, archived[i].old))
		}
		return errors.Join(failures...)
	}
	if previous.Enabled && (!o.Enabled || previous.Target() != o.Target()) {
		for _, oldPath := range []string{
			(SharedReplicaStore{Directory: dir, Target: previous.Target()}).path(),
			(SharedConflictStore{Directory: dir, Target: previous.Target()}).path(),
		} {
			newPath := oldPath + ".archived-" + NewID()
			if err = os.Rename(oldPath, newPath); os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return errors.Join(err, restore())
			}
			archived = append(archived, archivedState{oldPath, newPath})
		}
	}
	if err = AtomicWrite(filepath.Join(dir, "sync-settings.json"), data); err != nil {
		return errors.Join(err, restore())
	}
	return nil
}

// SyncOptionsStatus exposes only enabled/backend for locked startup and guards.
func SyncOptionsStatus(dir string) (SyncOptions, error) {
	options, err := LoadSyncOptions(dir)
	if errors.Is(err, ErrSyncSettingsLocked) {
		return SyncOptions{Version: 1, Enabled: true, Backend: "webdav"}, nil
	}
	return options, err
}

// Upgrade under the publication lock, reading the latest version rather than
// republishing a snapshot captured before another process saved its settings.
func LoadAndProtectSyncOptions(dir string, master []byte) (SyncOptions, error) {
	release, err := lockStateDirectory(context.Background(), dir, false)
	if err != nil {
		return SyncOptions{}, err
	}
	defer release()
	options, err := LoadSyncOptions(dir, master)
	if err != nil {
		return options, err
	}
	if len(master) > 0 && options.Enabled && options.Backend == "webdav" {
		raw, err := ReadBounded(filepath.Join(dir, "sync-settings.json"))
		if err != nil {
			return options, err
		}
		if !IsSharedVault(raw) {
			err = saveSyncOptionsLocked(dir, options, master)
			return options, err
		}
	}
	return options, nil
}

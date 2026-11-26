package clientalias

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	appPaths    = `Software\Microsoft\Windows\CurrentVersion\App Paths`
	recoveryKey = `Software\OShell Desktop\ClientAliases`
)

var Names = [...]string{"putty.exe", "xshell.exe"}

type registration struct {
	Target, Launcher, Digest       string
	Previous                       regString
	HadKey, AddedPath, CreatedFile bool
}

type State [2]registration

func (s State) Enabled(i int) bool { return s[i].Target != "" }

func loadRecord(key registry.Key, name string) (registration, error) {
	var record registration
	v, err := readString(key, name)
	if err != nil || v.Kind == 0 {
		return record, err
	}
	if v.Kind != registry.SZ || json.Unmarshal([]byte(v.Text), &record) != nil ||
		!filepath.IsAbs(record.Target) || !filepath.IsAbs(record.Launcher) ||
		!strings.EqualFold(filepath.Base(record.Launcher), name) || len(record.Digest) != sha256.Size*2 ||
		(record.Previous.Kind != 0 && record.Previous.Kind != registry.SZ && record.Previous.Kind != registry.EXPAND_SZ) {
		return registration{}, fmt.Errorf("%s 的恢复记录无效", name)
	}
	if _, err := hex.DecodeString(record.Digest); err != nil {
		return registration{}, fmt.Errorf("%s 的启动器校验值无效", name)
	}
	return record, nil
}

func load(root registry.Key) (State, error) {
	var state State
	key, err := registry.OpenKey(root, recoveryKey, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	defer key.Close()
	for i, name := range Names {
		state[i], err = loadRecord(key, name)
		if err != nil {
			return State{}, err
		}
	}
	return state, nil
}

func Load() (State, error) { return load(registry.CURRENT_USER) }

// Target is used by the packaged launchers, including an already running
// shell with a stale PATH after the user has turned a switch off.
func Target(launcher string) (string, error) {
	return targetFor(registry.CURRENT_USER, launcher)
}

func targetFor(root registry.Key, launcher string) (string, error) {
	name := strings.ToLower(filepath.Base(launcher))
	if !slices.Contains(Names[:], name) {
		return "", fmt.Errorf("未知的堡垒机客户端入口")
	}
	key, err := registry.OpenKey(root, recoveryKey, registry.QUERY_VALUE)
	if err == nil {
		defer key.Close()
		record, e := loadRecord(key, name)
		if e != nil {
			return "", e
		}
		if strings.EqualFold(record.Launcher, launcher) && !strings.EqualFold(record.Target, launcher) {
			return record.Target, nil
		}
	} else if !errors.Is(err, registry.ErrNotExist) {
		return "", err
	}
	return "", fmt.Errorf("请在 OShell 设置中启用 %s 客户端接管", name)
}

func pathIndex(parts []string, directory string) int {
	return slices.IndexFunc(parts, func(part string) bool {
		return strings.EqualFold(filepath.Clean(strings.Trim(part, `"`)), directory)
	})
}

// Publish exact build bytes without ever replacing an existing executable.
func installLauncher(path string, data []byte) (bool, error) {
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Size() != int64(len(data)) {
			return false, fmt.Errorf("客户端入口已有其他文件：%s", path)
		}
		existing, err := os.ReadFile(path)
		if err != nil {
			return false, err
		}
		if bytes.Equal(existing, data) {
			return false, nil
		}
		return false, fmt.Errorf("客户端入口已有其他文件：%s", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return false, err
	}
	f, err := os.CreateTemp(dir, ".oshell-launcher-*")
	if err != nil {
		return false, err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(data)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return false, err
	}
	from, err := windows.UTF16PtrFromString(f.Name())
	if err != nil {
		return false, err
	}
	to, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return false, err
	}
	err = windows.MoveFileEx(from, to, windows.MOVEFILE_WRITE_THROUGH)
	return err == nil, err
}

type manager struct {
	root              registry.Key
	target, directory string
	helper            []byte
}

func (m manager) launcher(name, digest string) string {
	// A still-running old helper can remain locked after restore. A new
	// build can register its own helper without overwriting that process.
	return filepath.Join(m.directory, strings.TrimSuffix(name, ".exe")+"-"+digest[:12], name)
}

// Only unlink our exact launcher bytes under our own directory. A user can
// change files and registry associations independently while OShell is open.
func (m manager) clean(name string, record registration) error {
	if !record.CreatedFile {
		return nil
	}
	path := m.launcher(name, record.Digest)
	if !strings.EqualFold(path, record.Launcher) {
		return nil
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !info.Mode().IsRegular() {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	hash := sha256.New()
	_, err = io.Copy(hash, f)
	f.Close()
	if err != nil || fmt.Sprintf("%x", hash.Sum(nil)) != record.Digest {
		return err
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	_ = os.Remove(filepath.Dir(path)) // Only succeeds for an empty directory.
	return nil
}

func (m manager) enable(name string, key, records registry.Key, existed bool, path *regString) (registration, error) {
	var r registration
	if len(m.helper) == 0 {
		return r, fmt.Errorf("缺少 OShell 堡垒机启动器")
	}
	old, err := readString(key, "")
	if err != nil {
		return r, err
	}
	r = registration{Target: m.target, Digest: fmt.Sprintf("%x", sha256.Sum256(m.helper)), Previous: old, HadKey: existed}
	r.Launcher = m.launcher(name, r.Digest)
	dir := filepath.Dir(r.Launcher)
	if strings.ContainsAny(dir, `;%"`) {
		return r, fmt.Errorf("客户端入口路径含有 PATH 不支持的字符")
	}
	r.CreatedFile, err = installLauncher(r.Launcher, m.helper)
	if err != nil {
		return r, err
	}
	if pathIndex(strings.Split(path.Text, ";"), dir) < 0 {
		r.AddedPath = true
		if path.Kind == 0 {
			path.Kind = registry.EXPAND_SZ
		}
		if path.Text == "" {
			path.Text = dir
		} else {
			path.Text = dir + ";" + path.Text
		}
	}
	data, err := json.Marshal(r)
	if err == nil {
		err = records.SetStringValue(name, string(data))
	}
	if err == nil {
		err = key.SetStringValue("", r.Launcher)
	}
	return r, err
}

func restore(tx transaction, name string, key, records registry.Key, existed bool, r registration, path *regString) error {
	active, err := readString(key, "")
	// The vendor may have replaced its registration with a different value type.
	if err != nil && !errors.Is(err, registry.ErrUnexpectedType) {
		return err
	}
	if active.Kind == registry.SZ && strings.EqualFold(active.Text, r.Launcher) {
		if err := r.Previous.write(key, ""); err != nil {
			return err
		}
	}
	if !r.HadKey || !existed {
		if err := tx.deleteEmpty(appPaths+`\`+name, key); err != nil {
			return err
		}
	}
	if r.AddedPath {
		parts := strings.Split(path.Text, ";")
		if n := pathIndex(parts, filepath.Dir(r.Launcher)); n >= 0 {
			path.Text = strings.Join(slices.Delete(parts, n, n+1), ";")
		}
	}
	return records.DeleteValue(name)
}

func (m manager) apply(original State, want [2]bool) error {
	if err := os.MkdirAll(m.directory, 0700); err != nil {
		return err
	}
	lockName, err := windows.UTF16PtrFromString(filepath.Join(m.directory, ".registration.lock"))
	if err != nil {
		return err
	}
	lock, err := windows.CreateFile(lockName, windows.GENERIC_WRITE|windows.DELETE, 0, nil, windows.OPEN_ALWAYS, windows.FILE_FLAG_DELETE_ON_CLOSE, 0)
	if err != nil {
		return fmt.Errorf("无法锁定客户端注册，请稍后重试：%w", err)
	}
	defer windows.CloseHandle(lock)
	var added State
	err = transact(m.root, func(tx transaction) error {
		records, _, err := tx.key(recoveryKey)
		if err != nil {
			return err
		}
		defer records.Close()
		env, _, err := tx.key("Environment")
		if err != nil {
			return err
		}
		defer env.Close()
		path, err := readString(env, "Path")
		if err != nil {
			return err
		}
		before := path
		// Across both switches remember whether to remove an originally absent
		// PATH when the last OShell directory has been removed.
		emptyPolicy, err := readString(records, "OriginalPathAbsent")
		if err != nil {
			return err
		}
		state := original
		for i, name := range Names {
			current, err := loadRecord(records, name)
			if err != nil {
				return err
			}
			state[i] = current
			if want[i] == original.Enabled(i) {
				continue
			}
			if original[i] != current {
				return fmt.Errorf("%s 的注册已更改，请重新打开设置", name)
			}
			keyPath := appPaths + `\` + name
			key, existed, err := tx.key(keyPath)
			if err != nil {
				return err
			}
			defer key.Close()
			if want[i] {
				if path.Kind == 0 {
					emptyPolicy = regString{"1", registry.SZ}
				}
				added[i], err = m.enable(name, key, records, existed, &path)
				state[i] = added[i]
			} else {
				err = restore(tx, name, key, records, existed, current, &path)
				state[i] = registration{}
			}
			if err != nil {
				return err
			}
		}
		if !state.Enabled(0) && !state.Enabled(1) {
			if emptyPolicy.Text == "1" && path.Text == "" {
				path = regString{}
			}
			emptyPolicy = regString{}
		}
		if err := emptyPolicy.write(records, "OriginalPathAbsent"); err != nil {
			return err
		}
		if path != before {
			return path.write(env, "Path")
		}
		return nil
	})
	for i, name := range Names {
		if err != nil {
			if cleanErr := m.clean(name, added[i]); cleanErr != nil {
				return errors.Join(err, cleanErr)
			}
		} else if original.Enabled(i) && !want[i] {
			// Windows can keep a running launcher locked. Its registration is
			// already gone and it verifies that registration on every invocation.
			_ = m.clean(name, original[i])
		}
	}
	return err
}

func Apply(original State, want [2]bool, target, helper string) error {
	cache, err := os.UserCacheDir()
	if err != nil {
		return err
	}
	m := manager{root: registry.CURRENT_USER, target: target, directory: filepath.Join(cache, "OShell Desktop", "ClientLaunchers")}
	if want[0] && !original.Enabled(0) || want[1] && !original.Enabled(1) {
		m.helper, err = os.ReadFile(helper)
		if err != nil {
			return fmt.Errorf("无法读取 Windows 客户端启动器：%w", err)
		}
	}
	if err = m.apply(original, want); err != nil {
		return err
	}
	environment, _ := windows.UTF16PtrFromString("Environment")
	// Explorer reloads the user environment for subsequently launched clients.
	notifyEnvironment.Call(0xffff, 0x1a, 0, uintptr(unsafe.Pointer(environment)), 2, 200, 0)
	return nil
}

var notifyEnvironment = windows.NewLazySystemDLL("user32.dll").NewProc("SendMessageTimeoutW")

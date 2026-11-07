package core

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/sftp"
)

type savedSFTPFragment struct {
	Name     string
	Size     int64
	Modified time.Time
}

func (c *SFTPCheckpoint) statTarget(client *sftp.Client, name string) (os.FileInfo, error) {
	if c.Upload {
		return client.Lstat(name)
	}
	return os.Lstat(name)
}

func (c *SFTPCheckpoint) runPersistent(ctx context.Context, client *sftp.Client, progress func(TransferProgress)) error {
	if c.Upload {
		c.Source, c.Target = filepath.Clean(c.Source), path.Clean(c.Target)
		if c.Fragment != "" {
			c.Fragment = path.Clean(c.Fragment)
		}
	} else {
		c.Source, c.Target = path.Clean(c.Source), filepath.Clean(c.Target)
		if c.Fragment != "" {
			c.Fragment = filepath.Clean(c.Fragment)
		}
	}
	dir := filepath.Join(c.StateDir, "sftp-checkpoints")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	key, _ := json.Marshal([]any{c.Identity, c.Upload, c.Source, c.Target})
	sum := sha256.Sum256(key)
	record := filepath.Join(dir, fmt.Sprintf("%x.json", sum))
	// Two tabs (or processes) must not append to the same destination together.
	lockKey := c.Target
	if c.Upload {
		lockKey = fmt.Sprintf("%s@%s:%d/%s", c.Identity.User, c.Identity.Host, c.Identity.Port, c.Target)
	}
	lockHash := sha256.Sum256([]byte(strings.ToLower(lockKey)))
	unlock, err := lockNamedDirectory(ctx, dir, fmt.Sprintf("transfer-%02x.lock", lockHash[0]%32))
	if err != nil {
		return err
	}
	defer unlock()
	base, join, parent := filepath.Base, filepath.Join, filepath.Dir
	if c.Upload {
		base, join, parent = path.Base, path.Join, path.Dir
	}
	validFragment := func(name string) bool {
		return name == base(c.Target) || safeEntry(name) && strings.HasPrefix(name, ".oshell-") && strings.HasSuffix(name, ".part")
	}
	if c.Fragment != "" && (!validFragment(base(c.Fragment)) || c.Fragment != join(parent(c.Target), base(c.Fragment))) {
		return errors.New("请选择目标目录中的同名文件或 .oshell-*.part 旧片段")
	}
	data, err := ReadBounded(record)
	if err == nil {
		var saved savedSFTPFragment
		if json.Unmarshal(data, &saved) != nil || !validFragment(saved.Name) || saved.Size < 0 {
			return errors.New("本机 SFTP 断点记录无效")
		}
		staged := join(parent(c.Target), saved.Name)
		if c.Fragment != "" && c.Fragment != staged {
			return errors.New("此路径已有另一份续传片段")
		}
		if _, err = c.statTarget(client, staged); err == nil {
			c.temporary, c.size, c.modified, c.initialized = staged, saved.Size, saved.Modified, true
		} else if os.IsNotExist(err) {
			c.initialized = false
			c.temporaryInfo = nil
		} else {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if !c.initialized && c.Fragment == "" && c.ResumeExisting {
		if _, err := c.statTarget(client, c.Target); err == nil {
			c.Fragment = c.Target
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	err = c.runFile(ctx, client, progress, func() error {
		out, e := json.Marshal(savedSFTPFragment{base(c.temporary), c.size, c.modified})
		if e != nil {
			return e
		}
		return AtomicWrite(record, out)
	})
	if err != nil {
		return err
	}
	if err = os.Remove(record); os.IsNotExist(err) {
		return nil
	}
	return err
}

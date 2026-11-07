package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/pkg/sftp"
)

var ErrResumeRegularOnly = errors.New("断点续传仅支持 SFTP 普通文件")

// Each Run owns its file handles. Persistent checkpoints are scoped to the
// resolved SSH identity, direction and source/destination pair.
type SFTPCheckpoint struct {
	Upload         bool
	Source, Target string
	StateDir       string
	Identity       SSHIdentity
	ResumeExisting bool
	Fragment       string
	Directory      bool
	temporary      string
	size           int64
	modified       time.Time
	sourceInfo     os.FileInfo
	temporaryInfo  os.FileInfo
	initialized    bool
	complete       bool
}

type resumeFile interface {
	io.Reader
	io.Writer
	io.Seeker
	io.Closer
	Stat() (os.FileInfo, error)
}

func (c *SFTPCheckpoint) Run(ctx context.Context, client *sftp.Client, progress func(TransferProgress)) error {
	if c.Directory {
		tree := transferState{ctx: ctx, client: client, report: progress, checkpoint: c}
		if c.Upload {
			return tree.upload(c.Source, c.Target, 0)
		}
		return tree.download(c.Source, c.Target, 0)
	}
	if c.StateDir != "" {
		return c.runPersistent(ctx, client, progress)
	}
	return c.runFile(ctx, client, progress, nil)
}

func (c *SFTPCheckpoint) runFile(ctx context.Context, client *sftp.Client, progress func(TransferProgress), save func() error) error {
	if c.complete {
		return errors.New("传输已经完成")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	var source, destination resumeFile
	var info os.FileInfo
	var beforeOpen os.FileInfo
	var err error
	if c.Upload {
		info, err = os.Lstat(c.Source)
	} else {
		info, err = client.Lstat(c.Source)
	}
	if err != nil {
		return err
	}
	if !RemoteFileTypeKnown(info) || !info.Mode().IsRegular() {
		return ErrResumeRegularOnly
	}
	if c.initialized && (info.Size() != c.size || !info.ModTime().Equal(c.modified) || c.Upload && c.sourceInfo != nil && !os.SameFile(c.sourceInfo, info)) {
		return errors.New("源文件已经变化，拒绝拼接旧片段；请重新选择传输")
	}
	if c.Upload {
		source, err = os.Open(c.Source)
	} else {
		source, err = client.Open(c.Source)
	}
	if err != nil {
		return err
	}
	defer source.Close()
	opened, err := source.Stat()
	if err != nil {
		return err
	}
	if !RemoteFileTypeKnown(opened) || !opened.Mode().IsRegular() || opened.Size() != info.Size() || !opened.ModTime().Equal(info.ModTime()) || c.Upload && !os.SameFile(opened, info) {
		return errors.New("源文件在打开期间发生变化")
	}
	c.sourceInfo = info
	if !c.initialized && c.Fragment != "" {
		c.temporary = c.Fragment
		c.size, c.modified = info.Size(), info.ModTime()
		c.initialized = true
	}
	if !c.initialized {
		c.size, c.modified, c.sourceInfo = info.Size(), info.ModTime(), info
		if c.Upload {
			c.temporary = path.Join(path.Dir(c.Target), ".oshell-"+NewID()+".part")
			var f *sftp.File
			f, err = client.OpenFile(c.temporary, os.O_RDWR|os.O_CREATE|os.O_EXCL)
			if err == nil {
				destination = f
				err = f.Chmod(0600)
			}
		} else {
			var f *os.File
			f, err = os.CreateTemp(filepath.Dir(c.Target), ".oshell-*.part")
			if err == nil {
				destination = f
				c.temporary = f.Name()
			}
		}
		if err != nil {
			if destination != nil {
				destination.Close()
			}
			return err
		}
		c.initialized = true
	} else {
		var staged os.FileInfo
		if c.Upload {
			staged, err = client.Lstat(c.temporary)
		} else {
			staged, err = os.Lstat(c.temporary)
		}
		if err != nil {
			return fmt.Errorf("续传片段不可用：%w", err)
		}
		if !RemoteFileTypeKnown(staged) || !staged.Mode().IsRegular() {
			return errors.New("续传片段不是普通文件")
		}
		beforeOpen = staged
		if c.temporary != c.Target {
			_, e := c.statTarget(client, c.Target)
			if !os.IsNotExist(e) {
				if e == nil {
					return os.ErrExist
				}
				return e
			}
		}
		flags := os.O_RDWR
		if staged.Size() == c.size {
			flags = os.O_RDONLY
		}
		if c.Upload {
			destination, err = client.OpenFile(c.temporary, flags)
		} else {
			if flags == os.O_RDONLY {
				destination, err = openImportFile(c.temporary)
			} else {
				destination, err = openWritableFragment(c.temporary)
			}
		}
		if err != nil {
			return err
		}
	}
	defer destination.Close()
	staged, err := destination.Stat()
	if err != nil {
		return err
	}
	if !RemoteFileTypeKnown(staged) || !staged.Mode().IsRegular() || staged.Size() > c.size {
		return errors.New("续传片段大小无效")
	}
	if beforeOpen != nil && (beforeOpen.Size() != staged.Size() || !beforeOpen.ModTime().Equal(staged.ModTime()) || !c.Upload && !os.SameFile(beforeOpen, staged)) {
		return errors.New("续传片段在打开期间发生变化")
	}
	if !c.Upload {
		if c.temporaryInfo != nil && !os.SameFile(c.temporaryInfo, staged) {
			return errors.New("本地续传片段已被替换")
		}
		c.temporaryInfo = staged
	}
	offset := staged.Size()
	// Verify every previously transferred byte, not merely a sampled range.
	left, right := make([]byte, 64<<10), make([]byte, 64<<10)
	for remaining := offset; remaining > 0; {
		if err = ctx.Err(); err != nil {
			return err
		}
		n := int(min(remaining, int64(len(left))))
		if _, err = io.ReadFull(source, left[:n]); err != nil {
			return err
		}
		if _, err = io.ReadFull(destination, right[:n]); err != nil {
			return err
		}
		if !bytes.Equal(left[:n], right[:n]) {
			return errors.New("源文件与已传片段不一致，拒绝续传")
		}
		remaining -= int64(n)
	}
	if offset > 0 {
		now, e := destination.Stat()
		if e != nil {
			return e
		}
		if now.Size() != offset || !now.ModTime().Equal(staged.ModTime()) {
			return errors.New("续传片段在校验期间发生变化")
		}
	}
	if save != nil && (offset < c.size || c.temporary != c.Target) {
		if err := save(); err != nil {
			return err
		}
	}
	report := func() {
		if progress != nil {
			progress(TransferProgress{Path: c.Target, Bytes: offset})
		}
	}
	report()
	last := time.Now()
	for offset < c.size {
		if err = ctx.Err(); err != nil {
			return err
		}
		n := int(min(c.size-offset, int64(len(left))))
		if _, err = io.ReadFull(source, left[:n]); err != nil {
			return err
		}
		if err = waitTransfer(ctx, n); err != nil {
			return err
		}
		written, e := destination.Write(left[:n])
		offset += int64(written)
		if e != nil {
			report()
			return e
		}
		if written != n {
			return io.ErrShortWrite
		}
		if time.Since(last) > 100*time.Millisecond {
			report()
			last = time.Now()
		}
	}
	report()
	if err = ctx.Err(); err != nil {
		return err
	}
	end, err := source.Stat()
	if err != nil {
		return err
	}
	if !RemoteFileTypeKnown(end) || !end.Mode().IsRegular() || end.Size() != c.size || !end.ModTime().Equal(c.modified) {
		return errors.New("传输过程中源文件已变化，未发布目标文件")
	}
	var current os.FileInfo
	if c.Upload {
		current, err = os.Lstat(c.Source)
	} else {
		current, err = client.Lstat(c.Source)
	}
	if err != nil {
		return err
	}
	if !RemoteFileTypeKnown(current) || !current.Mode().IsRegular() || current.Size() != c.size || !current.ModTime().Equal(c.modified) || c.Upload && !os.SameFile(current, c.sourceInfo) {
		return errors.New("源路径已变化，未发布目标文件")
	}
	if !c.Upload && staged.Size() < c.size {
		if f, ok := destination.(*os.File); ok {
			if err = f.Sync(); err != nil {
				return err
			}
		}
	}
	if err = destination.Close(); err != nil {
		return err
	}
	current, err = c.statTarget(client, c.temporary)
	if err != nil {
		return err
	}
	if !RemoteFileTypeKnown(current) || !current.Mode().IsRegular() || current.Size() != c.size || !c.Upload && !os.SameFile(current, staged) {
		return errors.New("续传片段在传输期间发生变化")
	}
	if c.temporary == c.Target {
		c.complete = true
		return nil
	}
	if c.Upload {
		// SFTP v3 RENAME must fail when the destination exists. Do not use
		// posix-rename, whose replacement semantics would destroy existing files.
		if _, err = client.Lstat(c.Target); err == nil {
			return os.ErrExist
		} else if !os.IsNotExist(err) {
			return err
		}
		err = client.Rename(c.temporary, c.Target)
	} else {
		err = publishDownload(c.temporary, c.Target, false, false)
		if err == nil {
			_ = os.Remove(c.temporary)
		}
	}
	if err != nil {
		return err
	}
	c.complete = true
	return nil
}

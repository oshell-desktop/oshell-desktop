// SPDX-License-Identifier: GPL-3.0-only
package core

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/pkg/sftp"
)

// SafeTransferName validates one portable file name, never a path.
func SafeTransferName(name string) bool { return safeEntry(name) }

func safeEntry(name string) bool {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00:\"<>|?*") || strings.TrimRight(name, " .") != name || strings.ContainsFunc(name, unicode.IsControl) {
		return false
	}
	base := strings.ToUpper(strings.TrimRight(strings.SplitN(name, ".", 2)[0], " "))
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || base == "CONIN$" || base == "CONOUT$" {
		return false
	}
	if len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9' {
		return false
	}
	if strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT") {
		switch base[3:] {
		case "¹", "²", "³":
			return false
		}
	}
	return true
}

type TransferProgress struct {
	Path  string
	Bytes int64
	Files int
}
type transferState struct {
	ctx     context.Context
	client  *sftp.Client
	report  func(TransferProgress)
	bytes   int64
	files   int
	visited int
	last    time.Time
}

func (s *transferState) copy(dst io.Writer, src io.Reader, name string) error {
	b := make([]byte, 64<<10)
	for {
		if e := s.ctx.Err(); e != nil {
			return e
		}
		n, e := src.Read(b)
		if n > 0 {
			if err := waitTransfer(s.ctx, n); err != nil {
				return err
			}
			w, we := dst.Write(b[:n])
			s.bytes += int64(w)
			if we != nil {
				return we
			}
			if w != n {
				return io.ErrShortWrite
			}
			if s.report != nil && time.Since(s.last) > 200*time.Millisecond {
				s.last = time.Now()
				s.report(TransferProgress{name, s.bytes, s.files})
			}
		}
		if e == io.EOF {
			s.files++
			return nil
		}
		if e != nil {
			return e
		}
	}
}
func (s *transferState) check(depth int) error {
	if e := s.ctx.Err(); e != nil {
		return e
	}
	s.visited++
	if depth > 32 || s.visited > 10000 {
		return fmt.Errorf("目录传输超过 32 层或 10000 个项目")
	}
	return nil
}

// Upload never replaces an existing remote entry, including directories.
func Upload(ctx context.Context, client *sftp.Client, local, remote string, progress func(TransferProgress)) error {
	return (&transferState{ctx: ctx, client: client, report: progress}).upload(local, remote, 0)
}
func (s *transferState) upload(local, remote string, depth int) error {
	if e := s.check(depth); e != nil {
		return e
	}
	info, e := os.Lstat(local)
	if e != nil {
		return e
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("不跟随符号链接：%s", local)
	}
	if info.IsDir() {
		if e = s.client.Mkdir(remote); e != nil {
			return e
		}
		entries, e := os.ReadDir(local)
		if e != nil {
			return e
		}
		for _, entry := range entries {
			if !safeEntry(entry.Name()) {
				return fmt.Errorf("文件名无效")
			}
			if e = s.upload(filepath.Join(local, entry.Name()), path.Join(remote, entry.Name()), depth+1); e != nil {
				return e
			}
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("不支持的文件类型：%s", local)
	}
	src, e := os.Open(local)
	if e != nil {
		return e
	}
	defer src.Close()
	dst, e := s.client.OpenFile(remote, os.O_CREATE|os.O_EXCL|os.O_WRONLY)
	if e != nil {
		return e
	}
	if e = dst.Chmod(0600); e == nil {
		e = s.copy(dst, src, remote)
	}
	ce := dst.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		s.client.Remove(remote)
	}
	return e
}

// Download stages content next to the chosen destination. Existing directories
// are never merged; a file is replaced only when the caller explicitly allows it.
func Download(ctx context.Context, client *sftp.Client, remote, local string, replaceFile bool, progress func(TransferProgress)) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	info, e := client.Stat(remote)
	if e != nil {
		return e
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("不跟随远程符号链接")
	}
	if _, e = os.Lstat(local); e == nil && (info.IsDir() || !replaceFile) {
		return fmt.Errorf("目标已存在，不覆盖目录或合并内容")
	}
	temp, e := os.MkdirTemp(filepath.Dir(local), ".oshell-download-*")
	if e != nil {
		return e
	}
	defer os.RemoveAll(temp)
	target := filepath.Join(temp, "content")
	s := &transferState{ctx: ctx, client: client, report: progress}
	if e = s.download(remote, target, 0); e != nil {
		return e
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	if info.IsDir() {
		if _, e = os.Lstat(local); !os.IsNotExist(e) {
			return fmt.Errorf("目标目录已存在")
		}
	}
	return publishDownload(target, local, info.IsDir(), replaceFile)
}
func (s *transferState) download(remote, local string, depth int) error {
	if e := s.check(depth); e != nil {
		return e
	}
	info, e := s.client.Lstat(remote)
	if depth == 0 {
		info, e = s.client.Stat(remote)
	}
	if e != nil {
		return e
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil // Identified recursive child links are skipped.
	}
	if info.IsDir() {
		if e = os.Mkdir(local, 0700); e != nil {
			return e
		}
		entries, e := s.client.ReadDir(remote)
		if e != nil {
			return e
		}
		for _, entry := range entries {
			if !safeEntry(entry.Name()) {
				return fmt.Errorf("服务器返回不安全的文件名")
			}
			if e = s.download(path.Join(remote, entry.Name()), filepath.Join(local, entry.Name()), depth+1); e != nil {
				return e
			}
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("不支持的远程文件类型")
	}
	src, e := s.client.Open(remote)
	if e != nil {
		return e
	}
	defer src.Close()
	dst, e := os.OpenFile(local, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	e = s.copy(dst, src, remote)
	if e == nil {
		e = dst.Sync()
	}
	ce := dst.Close()
	if e != nil {
		return e
	}
	return ce
}

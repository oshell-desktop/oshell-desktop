// SPDX-License-Identifier: GPL-3.0-only
package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/jlaffaye/ftp"
)

type FTPSession struct {
	mu       sync.Mutex
	profile  Profile
	password []byte
	closed   bool
}

func LoginFTP(ctx context.Context, p Profile, password string) (*FTPSession, RemoteFileSystem, error) {
	if e := p.Validate(); e != nil {
		return nil, nil, e
	}
	session := &FTPSession{profile: p, password: []byte(password)}
	files, e := session.OpenFiles(ctx)
	if e != nil {
		session.Close()
		return nil, nil, e
	}
	return session, files, nil
}

// Clone retains login credentials in a separately owned session; neither close
// operation clears the other's password. No credentials are written to disk.
func (s *FTPSession) Clone() (*FTPSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, fmt.Errorf("FTP 会话已关闭")
	}
	return &FTPSession{profile: s.profile, password: append([]byte(nil), s.password...)}, nil
}
func (s *FTPSession) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	clear(s.password)
	s.password = nil
}
func (s *FTPSession) OpenFiles(ctx context.Context) (RemoteFileSystem, error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, fmt.Errorf("FTP 会话已关闭")
	}
	p := s.profile
	password := append([]byte(nil), s.password...)
	s.mu.Unlock()
	defer clear(password)
	f := &FTPFileSystem{sockets: map[net.Conn]bool{}}
	dial := func(network, address string) (net.Conn, error) {
		c, e := (&net.Dialer{Timeout: 15 * time.Second}).DialContext(ctx, network, address)
		if e != nil {
			return nil, e
		}
		c = &ftpSocket{Conn: c, owner: f}
		f.mu.Lock()
		if f.closed {
			f.mu.Unlock()
			c.Close()
			return nil, context.Canceled
		}
		f.sockets[c] = true
		f.mu.Unlock()
		return c, nil
	}
	stop := context.AfterFunc(ctx, func() { f.Close() })
	defer stop()
	client, e := ftp.Dial(p.Address(), ftp.DialWithDialFunc(dial), ftp.DialWithTimeout(15*time.Second), ftp.DialWithTrustPasvIP(false))
	if e == nil {
		e = client.Login(p.Username, string(password))
	}
	if e != nil {
		f.Close()
		return nil, e
	}
	f.client = client
	f.mu.Lock()
	for c := range f.sockets {
		c.SetDeadline(time.Time{})
	}
	f.mu.Unlock()
	return f, nil
}

type FTPFileSystem struct {
	client  *ftp.ServerConn
	mu      sync.Mutex
	sockets map[net.Conn]bool
	closed  bool
}

func (f *FTPFileSystem) Close() error {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return nil
	}
	f.closed = true
	sockets := f.sockets
	f.sockets = nil
	f.mu.Unlock()
	for c := range sockets {
		c.Close()
	}
	return nil
}
func validFTPPath(p string) error {
	if strings.ContainsAny(p, "\r\n\x00") {
		return fmt.Errorf("FTP 路径不能包含换行或 NUL")
	}
	return nil
}
func (f *FTPFileSystem) RealPath(p string) (string, error) {
	if e := validFTPPath(p); e != nil {
		return "", e
	}
	if e := f.client.ChangeDir(p); e != nil {
		return "", e
	}
	return f.client.CurrentDir()
}
func (f *FTPFileSystem) ReadDir(p string) ([]os.FileInfo, error) {
	if e := validFTPPath(p); e != nil {
		return nil, e
	}
	entries, e := f.client.ListWithFallback(p)
	if e != nil {
		if errors.Is(e, ftp.ErrListingTooLarge) || errors.Is(e, ftp.ErrListingEncoding) {
			return nil, e
		}
		names, fallbackErr := f.client.NameList(p)
		if fallbackErr != nil {
			return nil, errors.Join(e, fallbackErr)
		}
		out := make([]os.FileInfo, 0, len(names))
		for _, name := range names {
			if !utf8.ValidString(name) {
				return nil, fmt.Errorf("FTP 文件名不是 UTF-8")
			}
			if safeEntry(name) {
				out = append(out, ftpUnknownInfo{ftpInfo{&ftp.Entry{Name: name}}})
			}
		}
		return out, nil
	}
	out := make([]os.FileInfo, 0, len(entries))
	for _, entry := range entries {
		if !utf8.ValidString(entry.Name) {
			return nil, fmt.Errorf("FTP 文件名不是 UTF-8")
		}
		if !safeEntry(entry.Name) {
			continue
		}
		out = append(out, ftpInfo{entry})
	}
	return out, nil
}
func (f *FTPFileSystem) Lstat(p string) (os.FileInfo, error) {
	if e := validFTPPath(p); e != nil {
		return nil, e
	}
	if p == "/" || p == "." {
		return ftpInfo{&ftp.Entry{Name: p, Type: ftp.EntryTypeFolder}}, nil
	}
	entries, e := f.ReadDir(path.Dir(p))
	if e != nil {
		return nil, e
	}
	for _, entry := range entries {
		if entry.Name() == path.Base(p) {
			return entry, nil
		}
	}
	return nil, os.ErrNotExist
}
func (f *FTPFileSystem) Mkdir(p string) error {
	if e := validFTPPath(p); e != nil {
		return e
	}
	return f.client.MakeDir(p)
}
func (f *FTPFileSystem) Rename(from, to string) error {
	if e := validFTPPath(from); e != nil {
		return e
	}
	if e := validFTPPath(to); e != nil {
		return e
	}
	return f.client.Rename(from, to)
}
func (f *FTPFileSystem) Remove(p string) error {
	if e := validFTPPath(p); e != nil {
		return e
	}
	return f.client.Delete(p)
}
func (f *FTPFileSystem) RemoveDirectory(p string) error {
	if e := validFTPPath(p); e != nil {
		return e
	}
	return f.client.RemoveDir(p)
}

type ftpInfo struct{ entry *ftp.Entry }

// NLST supplies names only. Preserve that uncertainty instead of claiming that
// a directory is an empty regular file. Opening it can probe CWD explicitly.
type ftpUnknownInfo struct{ ftpInfo }

func (ftpUnknownInfo) TypeKnown() bool { return false }

func (i ftpInfo) Name() string { return i.entry.Name }
func (i ftpInfo) Size() int64  { return int64(i.entry.Size) }
func (i ftpInfo) Mode() os.FileMode {
	if i.entry.UNIXModeKnown {
		n := i.entry.UNIXMode
		mode := os.FileMode(n & 0777)
		if n&04000 != 0 {
			mode |= os.ModeSetuid
		}
		if n&02000 != 0 {
			mode |= os.ModeSetgid
		}
		if n&01000 != 0 {
			mode |= os.ModeSticky
		}
		if i.entry.Type == ftp.EntryTypeFolder {
			mode |= os.ModeDir
		}
		if i.entry.Type == ftp.EntryTypeLink {
			mode |= os.ModeSymlink
		}
		return mode
	}
	if i.entry.Type == ftp.EntryTypeFolder {
		return os.ModeDir | 0755
	}
	if i.entry.Type == ftp.EntryTypeLink {
		return os.ModeSymlink | 0777
	}
	return 0644
}
func (i ftpInfo) ModTime() time.Time { return i.entry.Time }
func (i ftpInfo) IsDir() bool        { return i.entry.Type == ftp.EntryTypeFolder }
func (i ftpInfo) Sys() any           { return nil }

// Only parsed server attributes are displayed; synthetic defaults remain hidden.
func (i ftpInfo) PermissionsKnown() bool       { return i.entry.UNIXModeKnown }
func (i ftpInfo) OwnerNames() (string, string) { return i.entry.UNIXOwner, i.entry.UNIXGroup }

type ftpReader struct {
	ctx      context.Context
	source   io.Reader
	progress func(TransferProgress)
	state    *TransferProgress
	last     time.Time
}

func (r *ftpReader) Read(b []byte) (int, error) {
	if e := r.ctx.Err(); e != nil {
		return 0, e
	}
	n, e := r.source.Read(b[:min(len(b), 64<<10)])
	if n > 0 {
		if err := waitTransfer(r.ctx, n); err != nil {
			return 0, err
		}
	}
	r.state.Bytes += int64(n)
	if r.progress != nil && time.Since(r.last) > 200*time.Millisecond {
		r.last = time.Now()
		r.progress(*r.state)
	}
	return n, e
}
func (f *FTPFileSystem) Upload(ctx context.Context, local, remote string, report func(TransferProgress)) error {
	stop := context.AfterFunc(ctx, func() { f.Close() })
	defer stop()
	state := TransferProgress{}
	visited := 0
	var upload func(string, string, int) error
	upload = func(local, remote string, depth int) error {
		visited++
		if depth > 32 || visited > 10000 {
			return fmt.Errorf("目录项目或层级过多")
		}
		if e := ctx.Err(); e != nil {
			return e
		}
		if _, e := f.Lstat(remote); !os.IsNotExist(e) {
			if e != nil {
				return e
			}
			return fmt.Errorf("远程目标已存在")
		}
		info, e := os.Lstat(local)
		if e != nil {
			return e
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if depth > 0 {
				return nil
			}
			return fmt.Errorf("不跟随符号链接")
		}
		if info.IsDir() {
			if e = f.Mkdir(remote); e != nil {
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
				if e = upload(filepath.Join(local, entry.Name()), path.Join(remote, entry.Name()), depth+1); e != nil {
					return e
				}
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("不是普通文件")
		}
		source, e := os.Open(local)
		if e != nil {
			return e
		}
		defer source.Close()
		state.Path = remote
		e = f.client.Stor(remote, &ftpReader{ctx: ctx, source: source, progress: report, state: &state})
		if e != nil {
			f.client.Delete(remote)
			return e
		}
		state.Files++
		return nil
	}
	return upload(local, remote, 0)
}
func (f *FTPFileSystem) Download(ctx context.Context, remote, local string, replace bool, report func(TransferProgress)) error {
	stop := context.AfterFunc(ctx, func() { f.Close() })
	defer stop()
	info, e := f.Lstat(remote)
	if e != nil {
		return e
	}
	if _, e = os.Lstat(local); e == nil && (info.IsDir() || !replace) {
		return fmt.Errorf("本地目标已存在")
	}
	temp, e := os.MkdirTemp(filepath.Dir(local), ".oshell-ftp-*")
	if e != nil {
		return e
	}
	defer os.RemoveAll(temp)
	dest := filepath.Join(temp, "content")
	state := TransferProgress{}
	visited := 0
	var download func(string, string, int) error
	download = func(remote, local string, depth int) error {
		visited++
		if depth > 32 || visited > 10000 {
			return fmt.Errorf("目录项目或层级过多")
		}
		if e := ctx.Err(); e != nil {
			return e
		}
		info, e := f.Lstat(remote)
		if e != nil {
			return e
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("不跟随符号链接")
		}
		if info.IsDir() {
			if e = os.Mkdir(local, 0700); e != nil {
				return e
			}
			entries, e := f.ReadDir(remote)
			if e != nil {
				return e
			}
			for _, entry := range entries {
				if !safeEntry(entry.Name()) {
					return fmt.Errorf("服务器文件名无效")
				}
				if e = download(path.Join(remote, entry.Name()), filepath.Join(local, entry.Name()), depth+1); e != nil {
					return e
				}
			}
			return nil
		}
		reader, e := f.client.Retr(remote)
		if e != nil {
			return e
		}
		out, e := os.OpenFile(local, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			reader.Close()
			return e
		}
		state.Path = remote
		_, e = io.Copy(out, &ftpReader{ctx: ctx, source: reader, progress: report, state: &state})
		ce := out.Close()
		re := reader.Close()
		if e != nil {
			return e
		}
		if ce != nil {
			return ce
		}
		if re != nil {
			return re
		}
		state.Files++
		return nil
	}
	if e = download(remote, dest, 0); e != nil {
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
	return publishDownload(dest, local, info.IsDir(), replace)
}

type ftpSocket struct {
	net.Conn
	owner *FTPFileSystem
	once  sync.Once
}

func (c *ftpSocket) Read(b []byte) (int, error) {
	c.Conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	return c.Conn.Read(b)
}
func (c *ftpSocket) Write(b []byte) (int, error) {
	c.Conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
	return c.Conn.Write(b)
}
func (c *ftpSocket) Close() error {
	var err error
	c.once.Do(func() { c.owner.mu.Lock(); delete(c.owner.sockets, c); c.owner.mu.Unlock(); err = c.Conn.Close() })
	return err
}

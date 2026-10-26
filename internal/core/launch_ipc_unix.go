//go:build darwin || linux

package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"net"
	"os"
	"path/filepath"
)

func launchDirectory(configuration string) (string, error) {
	absolute, e := filepath.Abs(configuration)
	if e != nil {
		return "", e
	}
	digest := sha256.Sum256([]byte(filepath.Clean(absolute)))
	dir := filepath.Join("/tmp", fmt.Sprintf("oshell-desktop-launch-%d-%s", os.Geteuid(), hex.EncodeToString(digest[:10])))
	if e = os.Mkdir(dir, 0700); e != nil && !os.IsExist(e) {
		return "", e
	}
	var stat unix.Stat_t
	if e = unix.Lstat(dir, &stat); e != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR || stat.Uid != uint32(os.Geteuid()) || stat.Mode&0077 != 0 {
		return "", fmt.Errorf("启动通道目录权限无效")
	}
	return dir, nil
}
func launchLock(dir, name string) (*os.File, error) {
	fd, e := unix.Open(filepath.Join(dir, name), unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if e != nil {
		return nil, e
	}
	var stat unix.Stat_t
	if e = unix.Fstat(fd, &stat); e != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Uid != uint32(os.Geteuid()) || stat.Mode&0077 != 0 {
		unix.Close(fd)
		return nil, fmt.Errorf("启动通道锁属性无效")
	}
	if e = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); e != nil {
		unix.Close(fd)
		if errors.Is(e, unix.EWOULDBLOCK) {
			return nil, ErrLaunchBusy
		}
		return nil, e
	}
	return os.NewFile(uintptr(fd), name), nil
}
func listenLaunch(configuration string) (net.Listener, func(), error) {
	dir, e := launchDirectory(configuration)
	if e != nil {
		return nil, nil, e
	}
	lock, e := launchLock(dir, "server.lock")
	if e != nil {
		return nil, nil, e
	}
	path := filepath.Join(dir, "s")
	var stat unix.Stat_t
	if e = unix.Lstat(path, &stat); e == nil {
		if stat.Mode&unix.S_IFMT != unix.S_IFSOCK || stat.Uid != uint32(os.Geteuid()) {
			lock.Close()
			return nil, nil, fmt.Errorf("启动通道被其他文件占用")
		}
		if e = os.Remove(path); e != nil {
			lock.Close()
			return nil, nil, e
		}
	} else if !errors.Is(e, unix.ENOENT) {
		lock.Close()
		return nil, nil, e
	}
	listener, e := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if e != nil {
		lock.Close()
		return nil, nil, e
	}
	if e = os.Chmod(path, 0600); e != nil {
		listener.Close()
		lock.Close()
		return nil, nil, e
	}
	return listener, func() { _ = lock.Close() }, nil
}
func dialLaunch(ctx context.Context, configuration string) (net.Conn, error) {
	dir, err := launchDirectory(configuration)
	if err != nil {
		return nil, err
	}
	return (&net.Dialer{}).DialContext(ctx, "unix", filepath.Join(dir, "s"))
}
func verifyLaunchConnection(c net.Conn, server bool) error {
	return verifyLaunchPeer(c.(*net.UnixConn))
}

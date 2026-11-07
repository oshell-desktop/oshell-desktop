//go:build !windows

package core

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func openWritableFragment(name string) (*os.File, error) {
	fd, err := unix.Open(name, unix.O_RDWR|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	var info unix.Stat_t
	if err = unix.Fstat(fd, &info); err != nil {
		file.Close()
		return nil, err
	}
	if info.Nlink != 1 {
		file.Close()
		return nil, errors.New("不能续写链接到其他文件的本地片段")
	}
	return file, nil
}

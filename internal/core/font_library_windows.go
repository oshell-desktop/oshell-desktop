package core

import (
	"io/fs"
	"syscall"
)

func fontFileHidden(info fs.FileInfo) bool {
	data, ok := info.Sys().(*syscall.Win32FileAttributeData)
	return ok && data.FileAttributes&syscall.FILE_ATTRIBUTE_HIDDEN != 0
}

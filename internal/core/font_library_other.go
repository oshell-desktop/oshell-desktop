//go:build !windows

package core

import "io/fs"

func fontFileHidden(info fs.FileInfo) bool { return false }

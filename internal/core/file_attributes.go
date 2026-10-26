package core

import (
	"fmt"
	"github.com/pkg/sftp"
	"os"
	"strconv"
)

type FileAttributes struct{ Permissions, Octal, User, Group string }

func RemoteFileTypeKnown(info os.FileInfo) bool {
	if info == nil {
		return false
	}
	if source, ok := info.(interface{ TypeKnown() bool }); ok {
		return source.TypeKnown()
	}
	if stat, ok := info.Sys().(*sftp.FileStat); ok && stat != nil {
		return stat.Mode != 0
	}
	return true
}

// SFTP v3 supplies numeric UID/GID, not account names. Never resolve these
// against the local machine: the remote host has a different account database.
func RemoteFileAttributes(info os.FileInfo) FileAttributes {
	result := FileAttributes{Permissions: "—", Octal: "—", User: "—", Group: "—"}
	known := true
	if source, ok := info.(interface{ PermissionsKnown() bool }); ok {
		known = source.PermissionsKnown()
	}
	if stat, ok := info.Sys().(*sftp.FileStat); ok && stat != nil {
		result.User = strconv.FormatUint(uint64(stat.UID), 10)
		result.Group = strconv.FormatUint(uint64(stat.GID), 10)
		if stat.Mode == 0 {
			known = false
		}
	} else if source, ok := info.(sftp.FileInfoUidGid); ok {
		result.User = strconv.FormatUint(uint64(source.Uid()), 10)
		result.Group = strconv.FormatUint(uint64(source.Gid()), 10)
	}
	if source, ok := info.(interface{ OwnerNames() (string, string) }); ok {
		user, group := source.OwnerNames()
		if user != "" {
			result.User = user
		}
		if group != "" {
			result.Group = group
		}
	}
	if stat, ok := info.Sys().(*sftp.FileStat); ok && stat != nil {
		for _, fact := range stat.Extended {
			switch fact.ExtType {
			case "owner", "owner@openssh.com", "unix.ownername":
				if fact.ExtData != "" {
					result.User = fact.ExtData
				}
			case "group", "group@openssh.com", "unix.groupname":
				if fact.ExtData != "" {
					result.Group = fact.ExtData
				}
			}
		}
	}
	if known {
		mode := info.Mode()
		text := []byte("----------")
		switch {
		case mode&os.ModeSymlink != 0:
			text[0] = 'l'
		case mode.IsDir():
			text[0] = 'd'
		case mode&os.ModeSocket != 0:
			text[0] = 's'
		case mode&os.ModeNamedPipe != 0:
			text[0] = 'p'
		case mode&os.ModeCharDevice != 0:
			text[0] = 'c'
		case mode&os.ModeDevice != 0:
			text[0] = 'b'
		}
		for i := 0; i < 9; i++ {
			if mode.Perm()&(1<<uint(8-i)) != 0 {
				text[i+1] = "rwx"[i%3]
			}
		}
		numeric := uint32(mode.Perm())
		for _, flag := range []struct {
			mode    os.FileMode
			index   int
			on, off byte
			value   uint32
		}{{os.ModeSetuid, 3, 's', 'S', 04000}, {os.ModeSetgid, 6, 's', 'S', 02000}, {os.ModeSticky, 9, 't', 'T', 01000}} {
			if mode&flag.mode != 0 {
				numeric |= flag.value
				if text[flag.index] == 'x' {
					text[flag.index] = flag.on
				} else {
					text[flag.index] = flag.off
				}
			}
		}
		result.Permissions = string(text)
		result.Octal = fmt.Sprintf("%04o", numeric)
	}
	return result
}

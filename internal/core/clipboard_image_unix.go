//go:build !windows

package core

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
)

func validateImageCacheDirectory(dir string) error {
	var st unix.Stat_t
	if err := unix.Lstat(dir, &st); err != nil {
		return err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFDIR || st.Uid != uint32(os.Geteuid()) || st.Mode&0077 != 0 {
		return fmt.Errorf("图片缓存必须是当前用户私有的目录（权限 0700）")
	}
	return nil
}

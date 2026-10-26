//go:build darwin || linux

package core

import (
	"fmt"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

// ReadFileZillaSites reads only the caller-selected site database; parsing still
// selects exactly one entry. It never imports or rewrites the other sites.
func ReadFileZillaSites(site string) ([]byte, error) {
	if _, e := FileZillaSitePath(site); e != nil {
		return nil, e
	}
	home, e := os.UserHomeDir()
	if e != nil {
		return nil, e
	}
	paths := []string{filepath.Join(home, ".config/filezilla/sitemanager.xml"), filepath.Join(home, ".filezilla/sitemanager.xml")}
	if runtime.GOOS == "linux" {
		if config, err := os.UserConfigDir(); err == nil {
			paths[0] = filepath.Join(config, "filezilla", "sitemanager.xml")
		}
	}
	if explicit := os.Getenv("OSHELL_FILEZILLA_CONFIG"); explicit != "" {
		paths = []string{explicit}
	}
	for _, path := range paths {
		file, e := openImportFile(path)
		if e != nil {
			continue
		}
		var stat unix.Stat_t
		e = unix.Fstat(int(file.Fd()), &stat)
		if e != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Uid != uint32(os.Geteuid()) || stat.Size > 8<<20 {
			file.Close()
			return nil, fmt.Errorf("站点配置文件属性无效")
		}
		data, e := io.ReadAll(io.LimitReader(file, 8<<20+1))
		file.Close()
		if e != nil || len(data) > 8<<20 {
			return nil, fmt.Errorf("站点配置读取失败或过大")
		}
		if _, e = ParseFileZillaSite(site, data); e != nil {
			clear(data)
			return nil, e
		}
		return data, nil
	}
	return nil, fmt.Errorf("未找到有效的 FileZilla 站点配置，请从外部程序重新打开")
}

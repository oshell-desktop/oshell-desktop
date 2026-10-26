package core

import (
	"fmt"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"path/filepath"
)

func ReadFileZillaSites(site string) ([]byte, error) {
	if _, err := FileZillaSitePath(site); err != nil {
		return nil, err
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	p := filepath.Join(dir, "FileZilla", "sitemanager.xml")
	if explicit := os.Getenv("OSHELL_FILEZILLA_CONFIG"); explicit != "" {
		p = explicit
	}
	f, err := openImportFile(p)
	if err != nil {
		return nil, fmt.Errorf("未找到有效的 FileZilla 站点配置")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 8<<20 {
		return nil, fmt.Errorf("站点配置文件属性无效")
	}
	sd, err := windows.GetSecurityInfo(windows.Handle(f.Fd()), windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return nil, err
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return nil, err
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	if !owner.Equals(user.User.Sid) {
		return nil, fmt.Errorf("站点配置不属于当前用户")
	}
	data, err := io.ReadAll(io.LimitReader(f, 8<<20+1))
	if err != nil || len(data) > 8<<20 {
		return nil, fmt.Errorf("站点配置读取失败或过大")
	}
	if _, err = ParseFileZillaSite(site, data); err != nil {
		clear(data)
		return nil, err
	}
	return data, nil
}

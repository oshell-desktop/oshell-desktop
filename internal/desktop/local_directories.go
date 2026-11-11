package desktop

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"oshell-desktop/internal/core"
)

func localDirectories(values []string, fileURLs bool) ([]string, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("请先选择本机目录")
	}
	var directories []string
	seen := map[string]bool{}
	for _, value := range values {
		if fileURLs {
			u, err := url.Parse(value)
			if err != nil || u.Scheme != "file" || u.Host != "" && !strings.EqualFold(u.Host, "localhost") || u.RawQuery != "" || u.Fragment != "" {
				return nil, fmt.Errorf("仅支持本机文件目录")
			}
			value = u.Path
		}
		if !filepath.IsAbs(value) {
			return nil, fmt.Errorf("请选择本机目录完整路径")
		}
		value = filepath.Clean(value)
		info, err := os.Stat(value)
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("所选项目不是可用目录：%s", value)
		}
		if !seen[value] {
			directories = append(directories, value)
			seen[value] = true
		}
	}
	return directories, nil
}
func (a *App) openLocalDirectories(values []string, fileURLs bool) error {
	directories, err := localDirectories(values, fileURLs)
	if err != nil {
		return err
	}
	if a.workspaceLocked() || a.modalActive() || a.settingsOpen {
		return fmt.Errorf("请先解锁并关闭当前对话框")
	}
	for _, directory := range directories {
		profile := core.NewProfile()
		profile.Kind = "local"
		profile.Name = "本地终端"
		a.openConnection(profile, core.Credentials{}, sessionOpenOptions{split: -2, localDirectory: directory}, nil)
	}
	return nil
}

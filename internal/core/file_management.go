package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// Local presentation and user-authored command; never part of the shared vault.
type FileManagementSettings struct {
	Version   int    `json:"version"`
	Mode      string `json:"mode"`
	Placement string `json:"placement"`
	Command   string `json:"command"`
}

func DefaultFileManagementSettings() FileManagementSettings {
	return FileManagementSettings{1, "builtIn", "window", "yazi"}
}
func (s FileManagementSettings) Validate() error {
	if s.Version != 1 {
		return fmt.Errorf("文件管理设置版本不受支持")
	}
	if s.Mode != "builtIn" && s.Mode != "terminal" {
		return fmt.Errorf("文件管理打开方式无效")
	}
	switch s.Placement {
	case "window", "left", "right", "bottom":
	default:
		return fmt.Errorf("文件管理停靠位置无效")
	}
	if strings.TrimSpace(s.Command) == "" || len(s.Command) > 4096 || strings.ContainsFunc(s.Command, unicode.IsControl) {
		return fmt.Errorf("文件管理命令须为非空单行文本，最多 4096 字节，不能包含控制字符")
	}
	return nil
}
func LoadFileManagementSettings(dir string) (FileManagementSettings, error) {
	s := DefaultFileManagementSettings()
	data, err := ReadBounded(filepath.Join(dir, "file-management.json"))
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	var next FileManagementSettings
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err = d.Decode(&next); err != nil {
		return s, err
	}
	if !json.Valid(data) {
		return s, fmt.Errorf("文件管理设置格式无效")
	}
	if err = next.Validate(); err != nil {
		return s, err
	}
	return next, nil
}
func SaveFileManagementSettings(dir string, s FileManagementSettings) error {
	if _, err := LoadFileManagementSettings(dir); err != nil {
		return err
	}
	if err := s.Validate(); err != nil {
		return err
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return AtomicWrite(filepath.Join(dir, "file-management.json"), data)
}

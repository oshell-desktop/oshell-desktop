package desktop

import (
	"fmt"
	"oshell-desktop/internal/core"
	"reflect"
	"runtime"
)

type systemShortcutResult struct {
	conflict string
	readable bool
}

var systemShortcutProbe = probeSystemShortcut

func (a *App) checkSystemShortcut(key core.KeyboardShortcut) error {
	result := systemShortcutProbe(key)
	if result.conflict != "" {
		return fmt.Errorf("%s 与系统快捷键冲突：%s", key.Display(runtime.GOOS), result.conflict)
	}
	if !result.readable {
		a.features.shortcutSystemNotice = "无法读取系统快捷键；仅完成应用内部检查，请在系统设置中核对。"
	}
	return nil
}
func (a *App) checkChangedSystemShortcuts() error {
	a.features.shortcutSystemNotice = ""
	for _, action := range core.ShortcutActions {
		original := a.config.Settings.KeyboardShortcuts.Bindings(action.ID, runtime.GOOS)
		next := a.features.settingsDraft.KeyboardShortcuts.Bindings(action.ID, runtime.GOOS)
		if reflect.DeepEqual(original, next) {
			continue
		}
		for _, key := range next {
			if err := a.checkSystemShortcut(key); err != nil {
				return err
			}
		}
	}
	for _, pair := range [][2]core.KeyboardShortcut{{a.localUI.SidebarShortcut, a.localUIDraft.SidebarShortcut}, {a.localUI.DeepFocusShortcut, a.localUIDraft.DeepFocusShortcut}} {
		if pair[0] != pair[1] {
			if err := a.checkSystemShortcut(pair[1]); err != nil {
				return err
			}
		}
	}
	return nil
}

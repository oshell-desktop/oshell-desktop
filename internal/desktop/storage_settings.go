package desktop

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *App) prepareStorageSettings() {
	a.storageDirectoryDraft = a.store.Dir
	location, err := a.store.StorageLocation()
	if err != nil {
		a.features.settingsError = err.Error()
	} else if location.Pending != "" {
		a.storageDirectoryDraft = location.Pending
	}
	a.storageDirectoryOriginal = a.storageDirectoryDraft
	a.storagePickerGeneration++
}
func (a *App) storageSettings(c *ui.Context) {
	sectionTitle(c, "数据保存位置")
	ui.Text(c, "当前数据目录").FontSize(12).TextColor(c.Theme().TextMuted)
	ui.Text(c, a.store.Dir).Selectable().FontSize(12)
	ui.Button(c, "打开当前数据目录").OnClick(func() {
		if err := mygo.Shell.OpenPath(a.store.Dir); err != nil {
			a.features.settingsError = err.Error()
		}
	})
	if a.store.Portable {
		ui.Text(c, "绿色便携模式：配置和数据保存在 EXE 所在目录。移动时请复制整个程序文件夹。").FontSize(12).TextColor(c.Theme().TextMuted)
		return
	}
	ui.Divider(c)
	sectionTitle(c, "自定义数据目录")
	ui.Row(c).Gap(8).Children(func() {
		ui.TextInput(c, &a.storageDirectoryDraft).Label("自定义数据目录").Grow(1).Placeholder("输入新的绝对路径，或选择空目录")
		ui.Button(c, "选择目录…").OnClick(a.chooseStorageDirectory)
	})
	ui.Button(c, "保持当前目录 / 取消待迁移").OnClick(func() { a.storageDirectoryDraft = a.store.Dir })
	ui.Text(c, "应用后重启生效，现有数据自动迁移，旧目录保留。请选择空目录或尚不存在的新目录。").FontSize(12).TextColor(c.Theme().TextMuted)
	if a.storageDirectoryOriginal != a.store.Dir {
		ui.Text(c, "已安排下次启动迁移至："+a.storageDirectoryOriginal).FontSize(12)
	}
	if a.store.StorageWarning != "" {
		ui.Text(c, a.store.StorageWarning).TextColor(c.Theme().Danger)
	}
}
func (a *App) chooseStorageDirectory() {
	generation := a.storagePickerGeneration
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.preferencesWindow, Title: "选择新的空数据目录", Directory: true})
		a.post(func() {
			if !a.settingsOpen || a.storagePickerGeneration != generation {
				return
			}
			if err != nil {
				a.features.settingsError = err.Error()
				return
			}
			if len(paths) > 0 {
				a.storageDirectoryDraft = paths[0]
			}
		})
	}()
}
func (a *App) applyStorageSettings() bool {
	if a.storageDirectoryDraft == a.storageDirectoryOriginal {
		return true
	}
	if err := a.store.ScheduleStorageDirectory(a.storageDirectoryDraft); err != nil {
		a.features.settingsError = "保存位置未修改：" + err.Error()
		return false
	}
	a.storageDirectoryOriginal = a.storageDirectoryDraft
	a.status = "数据保存位置已设置，下次启动生效；旧数据目录将保留"
	return true
}

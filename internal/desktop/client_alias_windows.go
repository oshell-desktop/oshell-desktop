package desktop

import (
	"os"
	"path/filepath"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/bundled"
	"oshell-desktop/internal/clientalias"
)

func init() {
	settingsPages = append(settingsPages, settingsPageInfo{"堡垒机客户端", "选择由 OShell 打开的 Windows SSH 客户端。", settingsIcon})
}

type clientAliasSettings struct {
	saved clientalias.State
	want  [2]bool
	exe   string
	err   error
}

func (a *App) prepareClientAliases() {
	f := &a.clientAliases
	f.saved, f.err = clientalias.Load()
	if f.err == nil {
		f.exe, f.err = os.Executable()
	}
	for i := range f.want {
		f.want[i] = f.saved.Enabled(i)
	}
}

func (a *App) applyClientAliases() error {
	f := &a.clientAliases
	if f.err != nil || f.want == [2]bool{f.saved.Enabled(0), f.saved.Enabled(1)} {
		return nil
	}
	helper := filepath.Join(filepath.Dir(f.exe), "oshell-client.bin")
	if bundled.Enabled() && (f.want[0] && !f.saved.Enabled(0) || f.want[1] && !f.saved.Enabled(1)) {
		path, err := bundled.Path("oshell-client.bin")
		if err != nil {
			return err
		}
		helper = path
	}
	if err := clientalias.Apply(f.saved, f.want, f.exe, helper); err != nil {
		return err
	}
	a.prepareClientAliases()
	return nil
}

func (a *App) clientAliasesView(c *ui.Context) {
	f := &a.clientAliases
	if f.err != nil {
		ui.Text(c, "无法读取 Windows 客户端注册："+f.err.Error()).Wrap().TextColor(c.Theme().Danger)
		return
	}
	sectionTitle(c, "当前 Windows 用户")
	ui.Text(c, "勾选并应用后，按程序名查找客户端的应用可以使用 OShell。取消勾选并应用可恢复原注册。").Wrap()
	for i, name := range clientalias.Names {
		ui.Checkbox(c, &f.want[i], "接管 "+name)
		if f.saved.Enabled(i) {
			ui.Text(c, "启动入口："+f.saved[i].Launcher).FontSize(12).Wrap().Selectable()
			ui.Text(c, "当前指向："+f.saved[i].Target).FontSize(12).Wrap().Selectable()
		}
	}
	ui.Divider(c)
	sectionTitle(c, "堡垒机使用固定路径时")
	ui.Text(c, "在堡垒机的 PuTTY 或 Xshell 客户端设置中选择下面的程序。若只允许选择同名文件，可选上方已启用的启动入口。").Wrap()
	ui.Text(c, f.exe).Selectable().Wrap()
	ui.Text(c, "更改后重新打开堡垒机客户端。它自带的同名程序或系统 PATH 中优先匹配的程序仍可能先启动。").FontSize(12).TextColor(c.Theme().TextMuted).Wrap()
}

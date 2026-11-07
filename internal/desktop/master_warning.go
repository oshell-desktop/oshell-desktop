package desktop

import (
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

func (a *App) masterWarning(c *ui.Context) {
	if a.vault == nil || a.focusFullscreen.chrome || a.config.Settings.MasterWarningAcknowledged {
		return
	}
	if protected, _ := a.vault.State(); protected {
		return
	}
	ui.Row(c).Padding(5, 12).Gap(8).AlignItems(ui.Center).Background(c.Theme().Warning.Alpha(.07)).BorderWidth(0, 0, 1, 0).BorderColor(c.Theme().Warning.Alpha(.18)).Children(func() {
		ui.Text(c, "未设置主密码，建议设置主密码保护保存的凭据。").FontSize(11).Grow(1)
		ui.Button(c, "立即设置主密码").Disabled(a.configurationBusy()).OnClick(func() { a.showVault(); a.features.vaultAction = "set" })
		ui.Button(c, "我已知晓，不再提醒").Disabled(a.configurationBusy()).OnClick(func() {
			a.changeConfig(func(cfg *core.Config) error { cfg.Settings.MasterWarningAcknowledged = true; return nil })
		})
	})
}

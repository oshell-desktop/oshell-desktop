package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/bundled"
	"oshell-desktop/internal/core"
)

type aboutDetails struct{ version, build, minimum string }

func readAboutDetails() aboutDetails {
	value := aboutDetails{version: mygo.App.Version(), build: "开发构建", minimum: "以当前平台发布包要求为准"}
	if value.version == "" {
		value.version = "开发构建"
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				value.build = setting.Value
				if len(value.build) > 12 {
					value.build = value.build[:12]
				}
			}
		}
	}
	if runtime.GOOS == "darwin" {
		value.minimum = "macOS 13.0 或更新版本"
		if executable, err := os.Executable(); err == nil {
			if resolved, err := filepath.EvalSymlinks(executable); err == nil {
				executable = resolved
			}
			bundle := filepath.Dir(filepath.Dir(filepath.Dir(executable)))
			if strings.HasSuffix(bundle, ".app") {
				if info, err := core.ReadAppBundleInfo(bundle); err == nil {
					value.version = info.Version
					value.build = info.Build
					if info.MinimumOS != "" {
						value.minimum = "macOS " + info.MinimumOS + " 或更新版本"
					}
				}
			}
		}
	}
	return value
}
func (a *App) openAboutDocument(name string) {
	if bundled.Enabled() {
		file, err := bundled.Path(name)
		if err == nil {
			err = mygo.Shell.OpenPath(file)
		}
		if err != nil {
			a.features.settingsError = "无法打开随附文档：" + err.Error()
			a.status = a.features.settingsError
		}
		return
	}
	resources, err := mygo.App.Path(mygo.PathResources)
	if err == nil {
		file := filepath.Join(resources, name)
		if _, err = os.Stat(file); err == nil {
			err = mygo.Shell.OpenPath(file)
		}
	}
	if err != nil {
		a.features.settingsError = "无法打开随附文档：" + err.Error()
		a.status = a.features.settingsError
	}
}
func (a *App) aboutSettings(c *ui.Context) {
	if a.features.about == nil {
		value := readAboutDetails()
		a.features.about = &value
	}
	info := a.features.about
	ui.Text(c, "OShell Desktop").FontSize(26).Bold()
	ui.Text(c, fmt.Sprintf("版本：%s\n构建：%s\n架构：%s / %s\n最低系统要求：%s", info.version, info.build, runtime.GOOS, runtime.GOARCH, info.minimum)).Wrap()
	ui.Text(c, "源码核对基准：OShell 0.2.92；Windows / Linux 平台适配。").TextColor(c.Theme().TextMuted)
	ui.Row(c).Gap(8).Children(func() {
		ui.Button(c, "开源许可证").OnClick(func() { a.openAboutDocument("LICENSE") })
		ui.Button(c, "第三方声明").OnClick(func() { a.openAboutDocument("THIRD_PARTY_NOTICES.md") })
		source, err := core.ParseUpdateSource(a.config.Settings.UpdateRepository)
		ui.Button(c, "项目主页").Disabled(err != nil).OnClick(func() {
			if err := mygo.Shell.OpenExternal("https://github.com/" + source.Repository); err != nil {
				a.features.settingsError = err.Error()
			}
		})
	})
}

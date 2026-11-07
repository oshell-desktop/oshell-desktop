// SPDX-License-Identifier: GPL-3.0-only
package desktop

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"
	"unicode"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

func (a *App) openFileAction(p *pane, action string) {
	if p == nil || p.closed || p.files == nil || p.files.busy || p.files.client == nil {
		return
	}
	f := &a.features
	f.fileActionState = p.files
	f.fileActionDirectory = p.files.dir
	f.fileActionOriginal = ""
	f.filePane = p
	f.fileAction = action
	f.fileName = "新目录"
	if action == "重命名远程项目" {
		names := p.files.selectedNames()
		if len(names) != 1 {
			return
		}
		p.files.selected = names[0]
		f.fileName = names[0]
		f.fileActionOriginal = names[0]
	}
	f.fileError = ""
	f.fileOpen = true
}
func (a *App) fileActionDialog(c *ui.Context) {
	f := &a.features
	desktopModal(c, &f.fileOpen, func() {
		dialogBody(c, 420).Gap(12).Children(func() {
			windowHeading(c, f.fileAction)
			if f.filePane != nil {
				ui.Text(c, f.filePane.profile.Name+" · "+f.filePane.profile.Host+" · "+f.fileActionDirectory).FontSize(12).TextColor(c.Theme().TextMuted)
			}
			if f.fileActionOriginal != "" {
				ui.Text(c, "原名称："+f.fileActionOriginal).FontSize(12)
			}
			input := ui.TextInput(c, &f.fileName).Label("文件名称").AutoFocus()
			if input.Changed() {
				f.fileError = ""
			}
			if input.Submitted() {
				a.applyFileAction()
			}
			if f.fileError != "" {
				ui.Text(c, f.fileError).TextColor(c.Theme().Danger)
			}
			dialogActions(c).Children(func() {
				ui.Button(c, "取消").OnClick(func() { f.fileOpen = false })
				ui.PrimaryButton(c, "应用").OnClick(a.applyFileAction)
			})
		})
	})
}
func (a *App) deleteRemote(p *pane) {
	f := p.files
	names, directory := f.selectedNames(), f.dir
	if len(names) == 0 {
		return
	}
	for _, name := range names {
		if !validRemoteFileName(name) {
			f.status = fmt.Sprintf("无法删除 %q：远程项目名称无效", name)
			f.statusError = true
			return
		}
	}
	a.withTransfer(p, func(ctx context.Context, client core.RemoteFileSystem) error {
		result, err := mygo.Dialog.Message(mygo.MessageOptions{Parent: a.fileParent(), Type: mygo.MessageWarning, Title: "删除远程项目", Message: fmt.Sprintf("删除 %d 个远程项目？", len(names)), Detail: strings.Join(names, "\n") + "\n永久删除，不进入废纸篓；目录必须为空。", Buttons: []string{"取消", "删除"}, DefaultButton: 0, CancelButton: 0})
		if err != nil {
			return err
		}
		if result.Button != 1 {
			return context.Canceled
		}
		return removeRemoteSelection(ctx, client, directory, names)
	})
}

func removeRemoteSelection(ctx context.Context, client core.RemoteFileSystem, directory string, names []string) error {
	for _, name := range names {
		if !validRemoteFileName(name) {
			return fmt.Errorf("远程项目名称无效：%q", name)
		}
	}
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return err
		}
		remote := path.Join(directory, name)
		info, err := client.Lstat(remote)
		if err != nil {
			return err
		}
		if info.IsDir() {
			err = client.RemoveDirectory(remote)
		} else {
			err = client.Remove(remote)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (a *App) applyFileAction() {
	f := &a.features
	p := f.filePane
	if !f.fileOpen {
		return
	}
	if p == nil || p.closed || p.files == nil || p.files != f.fileActionState || p.files.dir != f.fileActionDirectory {
		f.fileError = "连接或目录已改变，请关闭弹窗后重试"
		return
	}
	if p.files.busy || p.files.client == nil {
		f.fileError = "文件连接正在忙或已断开，请稍后重试"
		return
	}
	name := f.fileName
	if !validRemoteFileName(name) {
		f.fileError = "请填写不含路径分隔符或控制字符的名称"
		return
	}
	if f.fileAction == "重命名远程项目" && name == f.fileActionOriginal {
		f.fileOpen = false
		return
	}
	for _, entry := range p.files.entries {
		if entry.Name() == name {
			f.fileError = "目标名称已存在"
			return
		}
	}
	action, directory := f.fileAction, f.fileActionDirectory
	from, to := path.Join(directory, f.fileActionOriginal), path.Join(directory, name)
	f.fileOpen = false
	a.withTransfer(p, func(_ context.Context, client core.RemoteFileSystem) error {
		if action == "新建远程目录" {
			return client.Mkdir(to)
		}
		if _, err := client.Lstat(to); !os.IsNotExist(err) {
			return fmt.Errorf("目标已存在或无法确认，不覆盖")
		}
		return client.Rename(from, to)
	}, directory)
}

// Remote-only operations do not create Windows files. Allow server names such
// as report:2026 and NUL, while keeping each operation inside its chosen folder.
func validRemoteFileName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, "/\\") && !strings.ContainsFunc(name, unicode.IsControl)
}

// Initial directories still resolve against the server's login directory.
// Once browsing an absolute directory, relative input is relative to that view.
func resolveRemoteDirectory(current, requested string) string {
	if path.IsAbs(current) && !path.IsAbs(requested) {
		return path.Join(current, requested)
	}
	return requested
}

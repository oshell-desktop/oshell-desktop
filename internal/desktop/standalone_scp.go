package desktop

import (
	"context"
	"fmt"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"path"
	"strings"
)

type standaloneSCPDraft struct {
	open        bool
	pane        *pane
	remote, err string
}

// This adapter deliberately does not preflight via SFTP: this entry point is
// also used by servers which have no SFTP subsystem.
type standaloneSCPFileSystem struct {
	core.RemoteFileSystem
	connection *core.Connection
}

func (f *standaloneSCPFileSystem) Upload(ctx context.Context, source, target string, p func(core.TransferProgress)) error {
	if f.connection == nil || !f.connection.Alive() {
		return fmt.Errorf("SSH 连接已断开")
	}
	return f.connection.SCPUpload(ctx, source, target, p)
}
func (f *standaloneSCPFileSystem) Download(ctx context.Context, source, target string, replace bool, p func(core.TransferProgress)) error {
	if f.connection == nil || !f.connection.Alive() {
		return fmt.Errorf("SSH 连接已断开")
	}
	return f.connection.SCPDownload(ctx, source, target, replace, p)
}

func (a *App) openStandaloneSCP(p *pane) {
	if p == nil || p.closed || p.conn == nil || !p.conn.Alive() || p.files == nil || p.files.busy {
		return
	}
	a.features.scpDraft = &standaloneSCPDraft{open: true, pane: p, remote: p.files.dir}
}
func (a *App) standaloneSCPDialog(c *ui.Context) {
	draft := a.features.scpDraft
	if draft == nil {
		return
	}
	run := func(upload bool) {
		p := draft.pane
		remote := draft.remote
		if p.closed || p.conn == nil || !p.conn.Alive() || p.files == nil || p.files.busy {
			draft.err = "连接不可用"
			return
		}
		if strings.ContainsAny(remote, "\x00\r\n") || !core.SafeTransferName(path.Base(remote)) {
			draft.err = "请填写完整的远程文件或目录路径"
			return
		}
		a.features.scpDraft = nil
		connection := p.conn
		bindTransferIdentity(p.files, connection.Identity)
		records := p.files.records
		a.withFileOperation(p, p.files.client != nil, func(ctx context.Context) error {
			task, _ := ctx.Value(fileTransferContextKey{}).(*fileTransfer)
			client := &recordedFileSystem{RemoteFileSystem: &standaloneSCPFileSystem{connection: connection}, log: records, task: task, protocol: "SCP"}
			defer client.finish()
			if upload {
				paths, err := chooseUploadItems(a.fileParent(), false)
				if err != nil {
					return err
				}
				if len(paths) == 0 {
					return context.Canceled
				}
				if len(paths) != 1 {
					return fmt.Errorf("独立 SCP 每次请选择一个文件或目录")
				}
				return client.Upload(ctx, paths[0], remote, a.transferProgress(p, ctx))
			}
			directories, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.fileParent(), Title: "选择 SCP 下载保存目录", Directory: true})
			if err != nil {
				return err
			}
			if len(directories) == 0 {
				return context.Canceled
			}
			target, err := uniqueLocalTransferPath(directories[0], path.Base(remote))
			if err != nil {
				return err
			}
			return client.Download(ctx, remote, target, false, a.transferProgress(p, ctx))
		})
	}
	desktopModal(c.Key("standalone-scp"), &draft.open, func() {
		dialogBody(c, 530).Gap(10).Children(func() {
			windowHeading(c, "SCP 文件传输")
			ui.Text(c, "可用于没有 SFTP 子系统的 SSH 服务器。填写完整远程路径；上传可能覆盖该路径，下载重名自动编号。")
			ui.TextInput(c, &draft.remote).Label("SCP 远程路径").AutoFocus()
			if draft.err != "" {
				ui.Text(c, draft.err).TextColor(c.Theme().Danger)
			}
			dialogActions(c).Children(func() {
				ui.Button(c, "取消").OnClick(func() { a.features.scpDraft = nil })
				ui.Button(c, "上传…").OnClick(func() { run(true) })
				ui.Button(c, "下载…").OnClick(func() { run(false) })
			})
		})
	})
	if !draft.open {
		a.features.scpDraft = nil
	}
}

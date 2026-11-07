package desktop

import (
	"context"
	"fmt"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

type standaloneRsyncDraft struct {
	open               bool
	pane               *pane
	local, remote, err string
	confirmed          bool
}

// Directory transfers also work when the server has no SFTP subsystem.
type standaloneRsyncFileSystem struct {
	core.RemoteFileSystem
	connection *core.Connection
}

func (f *standaloneRsyncFileSystem) Upload(ctx context.Context, source, target string, p func(core.TransferProgress)) error {
	return f.connection.Rsync(ctx, core.RsyncOptions{Local: source, Remote: target, Upload: true, Directory: true}, p)
}
func (f *standaloneRsyncFileSystem) Download(ctx context.Context, source, target string, _ bool, p func(core.TransferProgress)) error {
	return f.connection.Rsync(ctx, core.RsyncOptions{Local: target, Remote: source, Directory: true}, p)
}
func (a *App) openStandaloneRsync(p *pane) {
	if p == nil || p.closed || p.conn == nil || !p.conn.Alive() || p.files != nil && p.files.busy {
		return
	}
	if p.files == nil {
		p.files = &fileState{dir: ".", input: "."}
	}
	a.features.rsyncDraft = &standaloneRsyncDraft{open: true, pane: p, remote: p.files.dir}
}
func (a *App) standaloneRsyncDialog(c *ui.Context) {
	d := a.features.rsyncDraft
	if d == nil {
		return
	}
	run := func(upload bool) {
		p := d.pane
		if p.closed || p.conn == nil || !p.conn.Alive() || p.files.busy {
			d.err = "连接不可用"
			return
		}
		if !d.confirmed {
			d.err = "请先确认增量更新规则"
			return
		}
		// Normalize pasted form padding before validation, transfer records and
		// execution. A leading space otherwise turns /data into ./ /data.
		// Keep this at the form boundary: filesystem-provided names are literal.
		d.remote = strings.TrimSpace(d.remote)
		if strings.TrimSpace(d.local) == "" || d.remote == "" {
			d.err = "请指定本机和远程目录"
			return
		}
		options := core.RsyncOptions{Local: d.local, Remote: d.remote, Upload: upload, Directory: true}
		if _, _, err := core.RsyncArguments(options, "oshell"); err != nil {
			d.err = err.Error()
			return
		}
		connection := p.conn
		bindTransferIdentity(p.files, connection.Identity)
		if p.files.records == nil {
			p.files.records = &transferLog{}
		}
		records := p.files.records
		a.features.rsyncDraft = nil
		a.withFileOperation(p, p.files.client != nil, func(ctx context.Context) error {
			task, _ := ctx.Value(fileTransferContextKey{}).(*fileTransfer)
			client := &recordedFileSystem{RemoteFileSystem: &standaloneRsyncFileSystem{connection: connection}, log: records, task: task, protocol: "rsync"}
			defer client.finish()
			if upload {
				return client.Upload(ctx, options.Local, options.Remote, a.transferProgress(p, ctx))
			}
			return client.Download(ctx, options.Remote, options.Local, true, a.transferProgress(p, ctx))
		})
	}
	desktopModal(c.Key("standalone-rsync"), &d.open, func() {
		dialogBody(c, 560).Gap(10).Children(func() {
			windowHeading(c, "rsync 目录传输")
			ui.Text(c, fmt.Sprintf("目标：%s · %s", d.pane.profile.Name, d.pane.profile.Host)).Wrap()
			ui.Text(c, "两端需要 rsync。复用当前 SSH 连接，无需 SFTP；将源目录内容合并到目标目录，不自动创建同名子目录。").Wrap()
			ui.TextInput(c, &d.local).Label("rsync 本机目录").Placeholder("本机目录完整路径")
			ui.Button(c, "选择本机目录…").OnClick(func() {
				parent := a.fileParent()
				go func() {
					paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: parent, Title: "选择 rsync 本机目录", Directory: true})
					a.post(func() {
						if a.features.rsyncDraft != d {
							return
						}
						if err != nil {
							d.err = err.Error()
						} else if len(paths) > 0 {
							d.local = paths[0]
						}
					})
				}()
			})
			ui.TextInput(c, &d.remote).Label("rsync 远程目录").Placeholder("远程目录路径")
			ui.Checkbox(c, &d.confirmed, "确认同名文件可更新；保留目标额外文件").Label("确认 rsync 增量更新")
			ui.Text(c, "取消可留下 .oshell-rsync-partial；再次运行同一方向可继续传输。限速使用当前文件标签设置，下次传输生效。").FontSize(11).Wrap()
			fileRateControls(c, d.pane.files)
			if d.err != "" {
				ui.Text(c, d.err).TextColor(c.Theme().Danger).Wrap()
			}
			dialogActions(c).Children(func() {
				ui.Button(c, "取消").OnClick(func() { a.features.rsyncDraft = nil })
				ui.Button(c, "增量上传").OnClick(func() { run(true) })
				ui.Button(c, "增量下载").OnClick(func() { run(false) })
			})
		})
	})
	if !d.open {
		a.features.rsyncDraft = nil
	}
}

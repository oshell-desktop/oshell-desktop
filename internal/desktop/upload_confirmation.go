package desktop

import (
	"context"
	"fmt"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"path"
	"path/filepath"
	"strings"
)

type uploadConfirmation struct {
	open           bool
	pane           *pane
	files          *fileState
	paths          []string
	directory, err string
	baseDirectory  string
	protocol       string
}

func (a *App) canPrepareUpload(p *pane) bool {
	return p != nil && !p.closed && p.files != nil && !p.files.busy && p.files.client != nil && a.features.uploadDraft == nil && (p.ftp != nil || p.conn != nil && p.conn.Alive())
}
func (a *App) prepareUpload(p *pane, paths []string) {
	if !a.canPrepareUpload(p) || len(paths) == 0 {
		return
	}
	if p.files.protocol == "" {
		p.files.protocol = "SFTP"
		if p.ftp != nil {
			p.files.protocol = "FTP"
		}
	}
	a.features.uploadDraft = &uploadConfirmation{open: true, pane: p, files: p.files, paths: append([]string(nil), paths...), directory: p.files.dir, baseDirectory: p.files.dir, protocol: p.files.protocol}
}
func (a *App) uploadDroppedOn(p *pane, paths []string, name string) {
	if !a.canPrepareUpload(p) || len(paths) == 0 {
		return
	}
	directory := p.files.dir
	if name != "" {
		found := false
		for _, entry := range p.files.visibleEntries() {
			if entry.Name() == name {
				found = true
				if entry.IsDir() {
					directory = path.Join(directory, name)
				}
				break
			}
		}
		if !found {
			return
		}
	}
	a.prepareUpload(p, paths)
	if draft := a.features.uploadDraft; draft != nil {
		draft.directory = directory
	}
}

func (a *App) uploadConfirmationDialog(c *ui.Context) {
	draft := a.features.uploadDraft
	if draft == nil {
		return
	}
	clear := func() { draft.paths = nil; a.features.uploadDraft = nil }
	desktopModal(c.Key("upload-confirmation"), &draft.open, func() {
		dialogBody(c, 500).Gap(10).Children(func() {
			windowHeading(c, fmt.Sprintf("上传 %d 个项目", len(draft.paths)))
			ui.Text(c, fmt.Sprintf("目标：%s · %s@%s:%d", draft.pane.profile.Name, draft.pane.profile.Username, draft.pane.profile.Host, draft.pane.profile.Port))
			for _, source := range draft.paths[:min(len(draft.paths), 10)] {
				ui.Text(c, filepath.Base(source)).SingleLine()
			}
			if len(draft.paths) > 10 {
				ui.Text(c, fmt.Sprintf("另有 %d 个项目", len(draft.paths)-10))
			}
			if draft.protocol == "rsync" {
				ui.Text(c, "rsync 按同名路径增量更新，可能覆盖同名文件；目录合并，保留目标额外文件。取消可能留下 .oshell-rsync-partial，再次运行可继续传输。").Wrap()
			} else if draft.protocol == "SFTP" {
				ui.Text(c, "同名目标可选择续传或保留两份。目录按文件校验并恢复。")
			} else {
				ui.Text(c, "重名自动编号；文件夹将递归上传。")
			}
			ui.Field(c, "远程目标目录", func() { ui.TextInput(c, &draft.directory).Label("远程目标目录").AutoFocus() })
			if draft.err != "" {
				ui.Text(c, draft.err).TextColor(c.Theme().Danger)
			}
			dialogActions(c).Children(func() {
				ui.Button(c, "取消").OnClick(clear)
				ui.PrimaryButton(c, "上传").Label("确认上传").OnClick(func() {
					p := draft.pane
					if p.closed || p.files != draft.files || p.files.protocol != draft.protocol || p.files.busy || (p.conn != nil && !p.conn.Alive()) {
						draft.err = "连接已变化，请重新选择上传内容"
						return
					}
					if strings.TrimSpace(draft.directory) == "" || strings.ContainsRune(draft.directory, '\x00') {
						draft.err = "请输入有效的远程目录"
						return
					}
					paths, directory := append([]string(nil), draft.paths...), resolveRemoteDirectory(draft.baseDirectory, draft.directory)
					clear()
					a.withTransfer(p, func(ctx context.Context, client core.RemoteFileSystem) error {
						return uploadUnique(ctx, client, paths, directory, a.transferProgress(p, ctx))
					}, directory)
				})
			})
		})
	})
	if !draft.open {
		clear()
	}
}

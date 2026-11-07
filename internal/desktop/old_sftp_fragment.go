package desktop

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"

	"github.com/egoist/mygo"
	"oshell-desktop/internal/core"
)

func (a *App) resumeOldFragment(p *pane, upload bool) {
	names := p.files.selectedNames()
	if len(names) != 1 || !core.SafeTransferName(names[0]) {
		return
	}
	remote := path.Join(p.files.dir, names[0])
	a.withTransfer(p, func(ctx context.Context, client core.RemoteFileSystem) error {
		rec, ok := client.(*recordedFileSystem)
		if !ok || rec.protocol != "SFTP" {
			return core.ErrResumeRegularOnly
		}
		title := "选择已下载的同名文件或 .oshell-*.part 旧片段"
		if upload {
			title = "选择与远程片段对应的本地源文件"
		}
		files, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.fileParent(), Title: title})
		if err != nil {
			return err
		}
		if len(files) != 1 {
			return context.Canceled
		}
		local := files[0]
		info, err := os.Lstat(local)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return core.ErrResumeRegularOnly
		}
		info, err = client.Lstat(remote)
		if err != nil {
			return err
		}
		if !core.RemoteFileTypeKnown(info) || !info.Mode().IsRegular() {
			return core.ErrResumeRegularOnly
		}
		if upload {
			name := filepath.Base(local)
			if !core.SafeTransferName(name) {
				return errors.New("本地源文件名无效")
			}
			rec.fragment = remote
			return rec.Upload(ctx, local, path.Join(path.Dir(remote), name), a.transferProgress(p, ctx))
		}
		rec.fragment = local
		return rec.Download(ctx, remote, filepath.Join(filepath.Dir(local), names[0]), false, a.transferProgress(p, ctx))
	})
}

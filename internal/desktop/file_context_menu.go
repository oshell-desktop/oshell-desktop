package desktop

import (
	"github.com/egoist/mygo/ui"
	"os"
	"oshell-desktop/internal/core"
	"path"
)

func fileCanNavigate(info os.FileInfo) bool {
	return info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !core.RemoteFileTypeKnown(info)
}

func (a *App) fileContextMenu(menu *ui.Menu, p *pane, name string) {
	if p == nil || p.closed || p.files == nil {
		return
	}
	f := p.files
	if name == "" {
		f.selected = ""
		f.selection = nil
		f.selectionAnchor = ""
	} else if !f.isSelected(name) {
		found := false
		for i, entry := range f.visibleEntries() {
			if entry.Name() == name {
				f.selectEntry(i, false, false)
				found = true
				break
			}
		}
		if !found {
			f.selected = ""
			f.selection = nil
			f.selectionAnchor = ""
		}
	}
	selected := f.selectedNames()
	active := !f.busy && f.client != nil && (p.ftp != nil || p.conn != nil && p.conn.Alive())
	if len(selected) == 1 {
		for _, entry := range f.entries {
			if entry.Name() == selected[0] && fileCanNavigate(entry) {
				if menu.Item("进入目录").Disabled(!active).Chosen() {
					a.loadFiles(p, path.Join(f.dir, entry.Name()))
				}
				break
			}
		}
	}
	if menu.Item("下载…").Disabled(!active || len(selected) == 0).Chosen() {
		a.download(p)
	}
	if menu.Item("重命名…").Disabled(!active || len(selected) != 1).Chosen() {
		a.openFileAction(p, "重命名远程项目")
	}
	if menu.Item("删除…").Disabled(!active || len(selected) == 0).Chosen() {
		a.deleteRemote(p)
	}
	menu.Separator()
	if menu.Item("上传…").Disabled(!active).Chosen() {
		a.uploadPath(p, false)
	}
	if menu.Item("新建目录…").Disabled(!active).Chosen() {
		a.openFileAction(p, "新建远程目录")
	}
	if menu.Item("刷新").Disabled(!active).Chosen() {
		a.loadFiles(p, f.dir)
	}
}

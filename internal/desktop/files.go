// SPDX-License-Identifier: GPL-3.0-only
package desktop

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"
	"os"
	"oshell-desktop/internal/core"
	"path"
	"sort"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type fileTransfer struct {
	pane         *pane
	notice       *fileTransferNotice // Accessed only on the UI thread.
	pause        atomic.Bool
	lastProgress time.Time
	lastBytes    int64
	refresh      bool
	files        *fileState
	started      time.Time
}
type fileTransferContextKey struct{}

type fileState struct {
	notice                       *fileTransferNotice
	records                      *transferLog
	recordsOpen                  bool
	recordDetail                 string
	recordList                   ui.ListState
	rateOpen                     bool
	rateUnit                     string
	limiter                      core.TransferLimiter
	rateChoice                   string
	rateCustom                   string
	rateError                    string
	statusError                  bool
	pageRows                     int
	rowHeight                    float32
	typeQuery                    string
	typeTime                     time.Time
	focusList                    bool
	readCancel                   context.CancelFunc
	readGeneration               uint64
	activeTransfer               *fileTransfer
	filter, visibleFilter        string
	visibleCache                 []os.FileInfo
	filterReady                  bool
	list                         ui.ListState
	focusRow                     string
	selection                    map[string]bool
	selectionAnchor              string
	transferDirectory            string
	protocol                     string
	started                      time.Time
	client                       core.RemoteFileSystem
	dir, input, selected, status string
	entries                      []os.FileInfo
	busy                         bool
	cancel                       context.CancelFunc
	lease                        func()
}

type fileReadNotice struct {
	status string
	failed bool
}

func (a *App) loadFiles(p *pane, dir string) {
	a.readFiles(p, dir, nil)
}

// A failed or cancelled batch may have completed some items. Refresh the list
// without replacing its operation result with a misleading item count.
func (a *App) readFiles(p *pane, dir string, notice *fileReadNotice) {
	if p == nil || (p.conn == nil && p.ftp == nil) || p.closed {
		return
	}
	if p.files == nil {
		p.files = &fileState{dir: ".", input: "."}
	}
	f := p.files
	if f.busy {
		return
	}
	if p.conn != nil {
		bindTransferIdentity(f, p.conn.Identity)
	}
	dir = resolveRemoteDirectory(f.dir, dir)
	f.busy = true
	f.readGeneration++
	generation := f.readGeneration
	ctx, cancel := context.WithCancel(context.Background())
	f.readCancel = cancel
	f.status = "读取目录…"
	f.statusError = false
	client := f.client
	lease := f.lease
	conn := p.conn
	ftp := p.ftp
	go func() {
		defer cancel()
		var err error
		if client == nil {
			if ftp != nil {
				client, err = ftp.OpenFiles(ctx)
				lease = func() {}
			} else {
				client, lease, err = conn.OpenFiles(ctx)
			}
		}

		if err != nil {
			postFileReadResult(p, func() {
				if p.closed || p.files != f || f.readGeneration != generation {
					return
				}
				f.readCancel = nil
				f.busy = false
				f.status = "读取目录失败：" + err.Error()
				if notice != nil {
					f.status = notice.status + "；" + f.status
				}
				f.statusError = true
			}, func() {})
			return
		}
		stop := context.AfterFunc(ctx, func() { client.Close() })
		resolved, err := client.RealPath(dir)
		var entries []os.FileInfo
		if err == nil {
			entries, err = client.ReadDir(resolved)
		}
		stop()
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		order := collate.New(language.Und, collate.Numeric)
		sort.SliceStable(entries, func(i, j int) bool {
			if entries[i].IsDir() != entries[j].IsDir() {
				return entries[i].IsDir()
			}
			return order.CompareString(entries[i].Name(), entries[j].Name()) < 0
		})
		discard := func() {
			go func() {
				client.Close()
				if lease != nil {
					lease()
				}
			}()
		}
		postFileReadResult(p, func() {
			if p.closed || p.files != f || f.readGeneration != generation {
				discard()
				return
			}
			f.readCancel = nil
			if errors.Is(err, context.Canceled) {
				f.client = nil
				f.lease = nil
				f.busy = false
				f.status = "目录读取已取消"
				go func() {
					client.Close()
					if lease != nil {
						lease()
					}
				}()
				return
			}
			f.client = client
			f.lease = lease
			f.busy = false
			if err != nil {
				f.status = "读取目录失败：" + err.Error()
				if notice != nil {
					f.status = notice.status + "；" + f.status
				}
				f.statusError = true
				p.status = "读取目录失败"
				return
			}
			p.status = "已连接"
			previousSelection := f.selectedNames()
			sameDirectory := f.dir == resolved
			f.dir = resolved
			f.input = resolved
			f.entries = entries
			f.typeQuery = ""
			f.focusList = len(entries) == 0
			f.filterReady = false
			f.visibleCache = nil
			f.selected = ""
			f.selection = nil
			f.selectionAnchor = ""
			f.focusRow = ""
			f.list = ui.ListState{Columns: f.list.Columns}
			if sameDirectory {
				retained := map[string]bool{}
				for _, name := range previousSelection {
					retained[name] = true
				}
				for _, entry := range f.visibleEntries() {
					name := entry.Name()
					if retained[name] {
						if f.selection == nil {
							f.selection = map[string]bool{}
						}
						f.selection[name] = true
						if f.selected == "" {
							f.selected = name
							f.selectionAnchor = name
						}
					}
				}
			}
			if visible := f.visibleEntries(); len(visible) > 0 {
				f.focusRow = visible[0].Name()
				if f.selected != "" {
					f.focusRow = f.selected
				}
			}
			f.status = fmt.Sprintf("%d 个项目", len(entries))
			if notice != nil {
				f.status = notice.status
				f.statusError = notice.failed
			}
			if owner := p.owner.Load(); owner != nil {
				owner.resumePendingTransfer(p)
			}
		}, discard)
	}()
}

// Follow a moved pane even if its old window closes between enqueue and drain.
// The result still runs for a disposed pane so its generation check can release
// a client that was opened after disposal started.
func postFileReadResult(p *pane, apply, discard func()) {
	for {
		owner := p.owner.Load()
		if owner == nil {
			discard()
			return
		}
		if owner.post(func() {
			if p.owner.Load() != owner {
				postFileReadResult(p, apply, discard)
				return
			}
			apply()
		}) {
			return
		}
		if p.owner.Load() == owner {
			discard()
			return
		}
	}
}
func (a *App) filesView(c *ui.Context, p *pane) { a.fileBrowser(c, p, false) }
func (a *App) fileBrowser(c *ui.Context, p *pane, full bool) {
	root := ui.Column(c).Padding(12).Gap(8).Background(c.Theme().Surface)
	if full {
		root.Fill()
	} else {
		root.Width(340)
	}
	if a.canPrepareUpload(p) {
		if paths := root.DroppedFiles(); len(paths) > 0 {
			a.uploadDropped(p, paths)
		}
	}
	root.Children(func() {
		ui.Row(c).Gap(6).Children(func() {
			protocol := "SFTP"
			if p != nil && p.profile.Kind == "ftp" {
				protocol = "FTP"
			}
			ui.Text(c, "远程文件 · "+protocol).Bold().Grow(1)
			if p != nil && p.files != nil {
				fileRateButton(c, p.files)
			}
			ui.Button(c, "×").Label("关闭文件面板").OnClick(func() {
				if full {
					if t := a.activeTab(); t != nil {
						a.requestSessionClose(t, p)
					}
				} else {
					a.filesVisible = false
				}
			})
		})
		if p == nil || (p.conn == nil && p.ftp == nil) {
			ui.Text(c, "请先连接 SSH 会话").TextColor(c.Theme().TextMuted)
			return
		}
		if p.files == nil {
			ui.Button(c, "加载文件").OnClick(func() { a.loadFiles(p, ".") })
			return
		}
		f := p.files
		if p.conn != nil && f.protocol == "" {
			f.protocol = "SFTP"
		}
		if p.conn != nil && !full {
			if f.protocol == "" {
				f.protocol = "SFTP"
			}
			ui.Select(c, &f.protocol, []string{"SFTP", "SCP"}).Label("传输协议").Disabled(f.busy)
		}
		ui.Row(c).Gap(6).Children(func() {
			if ui.TextInput(c, &f.input).Label("远程目录").Tooltip(f.dir).Grow(1).Disabled(f.busy).Submitted() {
				a.loadFiles(p, f.input)
			}
			ui.Button(c, "转到").Disabled(f.busy || f.input == "").OnClick(func() { a.loadFiles(p, f.input) })
		})
		ui.Row(c).Gap(6).Children(func() {
			ui.Button(c, "上级").Disabled(f.busy || f.dir == "/").OnClick(func() { a.loadFiles(p, path.Dir(f.dir)) })
			ui.Button(c, "刷新").Disabled(f.busy).OnClick(func() { a.loadFiles(p, f.dir) })
			if supportsMixedUploadSelection {
				ui.Button(c, "上传").Disabled(f.busy || f.client == nil || p.conn != nil && !p.conn.Alive()).OnClick(func() { a.uploadPath(p, false) })
			} else {
				ui.MenuButton(c, "上传", func(m *ui.Menu) {
					if m.Item("上传文件…").Chosen() {
						a.uploadPath(p, false)
					}
					if m.Item("上传目录…").Chosen() {
						a.uploadPath(p, true)
					}
				}).Disabled(f.busy || f.client == nil || p.conn != nil && !p.conn.Alive())
			}
			if p.conn != nil {
				ui.Button(c, "SCP…").Disabled(f.busy || !p.conn.Alive()).OnClick(func() { a.openStandaloneSCP(p) })
			}
			if full && p.conn != nil {
				ui.Select(c, &f.protocol, []string{"SFTP", "SCP"}).Label("传输协议").Width(125).Disabled(f.busy).Tooltip("目录浏览使用 SFTP；SCP 选项用于兼容传输")
			}
			if directoryFilter(c.Key("directory-filter"), &f.filter) {
				f.typeQuery = ""
				f.selected = ""
				f.selection = nil
				f.selectionAnchor = ""
				f.focusRow = ""
				f.list.ScrollTo(0, ui.Start)
			}
		})
		if full {
			ui.Row(c).Padding(6).Gap(6).Children(func() {
				ui.Text(c, "名称").Grow(1)
				ui.Text(c, "类型").Width(55)
				ui.Text(c, "大小").Width(75)
				ui.Text(c, "权限").Width(104)
				ui.Text(c, "用户 (UID)").Width(70)
				ui.Text(c, "组 (GID)").Width(70)
				ui.Text(c, "修改时间").Width(140)
			})
		}
		visible := f.visibleEntries()
		f.list.Key = func(i int) any { return visible[i].Name() }
		f.list.Label = func(i int) string { return visible[i].Name() }
		filesList := ui.List(c, &f.list, len(visible), func(i int) {
			entry := visible[i]
			name := entry.Name()
			presentation := presentFile(entry)
			detail := fileAttributeHint(entry, presentation.kind)
			row := ui.Row(c.Key(name)).Padding(6).Gap(6).Radius(4).Focusable().Label(name)
			if height := row.Bounds().H; height > 0 {
				f.rowHeight = height
			}
			row.ContextMenu(func(menu *ui.Menu) { a.fileContextMenu(menu, p, name) })
			if a.canPrepareUpload(p) {
				if paths := row.DroppedFiles(); len(paths) > 0 {
					a.uploadDroppedOn(p, paths, name)
				}
			}
			row.HandleInput(func(event ui.InputEvent) bool {
				if a.fileNavigationInput(p, i, event) {
					return true
				}
				if event.Kind == ui.InputCommand && event.Text == "selectAll" {
					f.selectAll()
					return true
				}
				if event.Kind == ui.InputKeyDown && event.Mods & ^ui.Shift == 0 {
					next := i
					switch event.Key {
					case ui.KeyUp:
						next = max(0, i-1)
					case ui.KeyDown:
						next = min(len(visible)-1, i+1)
					case ui.KeyPageUp:
						next = f.filePageIndex(i, -1)
					case ui.KeyPageDown:
						next = f.filePageIndex(i, 1)
					case ui.KeyHome:
						next = 0
					case ui.KeyEnd:
						next = len(visible) - 1
					case ui.KeyEnter:
						if !f.busy && fileCanNavigate(entry) && len(f.selectedNames()) == 1 {
							a.loadFiles(p, path.Join(f.dir, name))
						}
						return true
					default:
						return false
					}
					f.typeQuery = ""
					f.selectEntry(next, event.Mods&ui.Shift != 0, false)
					f.focusRow = visible[next].Name()
					f.list.ScrollIntoView(next)
					return true
				}
				return false
			})
			if f.focusRow == name {
				row.Focus()
				f.focusRow = ""
			}
			if f.isSelected(name) {
				row.Background(c.Theme().Selection)
			}
			row.Children(func() {
				ui.Icon(c, presentation.icon).FontSize(17).Width(20).Shrink(0).TextColor(presentation.color).Label(presentation.kind).Tooltip(detail)
				ui.Text(c, name).SingleLine().Grow(1).MinWidth(0).Tooltip(detail)
				if full {
					ui.Text(c, presentation.kind).Width(55).FontSize(11).Tooltip(detail)
				}
				size := "—"
				if !entry.IsDir() && core.RemoteFileTypeKnown(entry) {
					size = bytesLabel(entry.Size())
				}
				sizeHint := fmt.Sprintf("%d 字节", entry.Size())
				if !core.RemoteFileTypeKnown(entry) {
					sizeHint = "服务器未提供大小"
				}
				if entry.IsDir() {
					sizeHint = "目录大小未统计"
				}
				ui.Text(c, size).Tooltip(sizeHint).Width(75).FontSize(11)
				if full {
					attributes := core.RemoteFileAttributes(entry)
					ui.Text(c, attributes.Permissions).Font("monospace").Width(104).FontSize(11).SingleLine().Tooltip("权限：" + attributes.Permissions + " · " + attributes.Octal)
					ui.Text(c, attributes.User).Width(70).FontSize(11).SingleLine().Tooltip("所属用户 UID：" + attributes.User)
					ui.Text(c, attributes.Group).Width(70).FontSize(11).SingleLine().Tooltip("所属组 GID：" + attributes.Group)
					modified := "—"
					if !entry.ModTime().IsZero() {
						modified = entry.ModTime().Format("2006-01-02 15:04")
					}
					ui.Text(c, modified).Tooltip(fileModifiedDetail(entry)).Width(140).FontSize(11)
				}
			})
			if row.Clicked() {
				f.typeQuery = ""
				row.Focus()
				mods := row.ClickModifiers()
				f.selectEntry(i, mods&ui.Shift != 0, mods&ui.Cmd != 0)
			}
			if row.DoubleClicked() && !f.busy {
				f.selectEntry(i, false, false)
				if fileCanNavigate(entry) {
					a.loadFiles(p, path.Join(f.dir, name))
				} else {
					a.download(p)
				}
			}
		}).Grow(1).MinHeight(0).Gap(3).Label("远程文件列表")
		if height := filesList.Bounds().H; height > 0 && f.rowHeight > 0 {
			f.pageRows = max(1, int(height/(f.rowHeight+3))-1)
		}
		if len(visible) == 0 {
			filesList.Focusable().HandleInput(func(event ui.InputEvent) bool { return a.fileNavigationInput(p, -1, event) })
			if filesList.Clicked() || f.focusList {
				filesList.Focus()
				f.focusList = false
			}
		}
		filesList.ContextMenu(func(menu *ui.Menu) { a.fileContextMenu(menu, p, "") })
		if filesList.Shortcut(ui.Cmd, ui.KeyA) {
			f.selectAll()
		}
		if len(visible) == 0 && !f.busy {
			message := "当前目录为空，可上传文件或新建目录"
			if f.filter != "" {
				message = "没有匹配的项目，可修改或清除筛选"
			}
			ui.Text(c, message).FontSize(12).TextColor(c.Theme().TextMuted)
		}
		selected := f.selectedNames()
		ui.Row(c).Gap(6).Children(func() {
			ui.Button(c, "下载所选").Disabled(f.busy || len(selected) == 0).OnClick(func() { a.download(p) })
			ui.MenuButton(c, "操作", func(m *ui.Menu) {
				if m.Item("新建目录…").Chosen() {
					a.openFileAction(p, "新建远程目录")
				}
				if m.Item("重命名…").Disabled(len(selected) != 1).Chosen() {
					a.openFileAction(p, "重命名远程项目")
				}
				if m.Item("删除文件 / 空目录…").Disabled(len(selected) == 0).Chosen() {
					a.deleteRemote(p)
				}
			}).Disabled(f.busy || f.client == nil || p.conn != nil && !p.conn.Alive())
			if f.readCancel != nil {
				ui.Button(c, "取消读取").OnClick(func() {
					if f.readCancel != nil {
						f.readCancel()
					}
				})
			}
			if f.cancel != nil && (f.notice == nil || !f.notice.active) {
				if f.records != nil && f.activeTransfer != nil {
					for _, r := range f.records.snapshot() {
						if r.Status == "传输中" && r.Resume {
							ui.Button(c, "暂停").OnClick(func() { f.activeTransfer.pause.Store(true); f.cancel() })
							break
						}
					}
				}
				ui.Button(c, "取消传输").OnClick(func() { f.cancel() })
			}
		})
		ui.Text(c, "拖入文件或文件夹上传；拖到目录时上传至该目录。重名传输默认自动编号。").FontSize(11).TextColor(c.Theme().TextMuted)
		a.transferRecordsView(c, p)
		status := f.status
		if f.notice != nil && f.notice.active {
			status = "正在传输，进度见右上角"
		}
		if f.filter != "" {
			status = fmt.Sprintf("显示 %d / %d 项 · %s", len(visible), len(f.entries), status)
		}
		if len(selected) > 0 {
			status += fmt.Sprintf(" · 已选 %d 项", len(selected))
		}
		ui.Row(c).Gap(6).Children(func() {
			if f.busy {
				ui.Spinner(c).Width(14).Height(14)
			}
			color := c.Theme().TextMuted
			if f.statusError {
				color = c.Theme().Danger
			}
			ui.Text(c, status).FontSize(11).TextColor(color).Tooltip(status)
		})
	})
}
func bytesLabel(n int64) string {
	if n < 0 {
		return "—"
	}
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	value := float64(n)
	for _, unit := range []string{"KiB", "MiB", "GiB", "TiB", "PiB", "EiB"} {
		value /= 1024
		if value < 1024 || unit == "EiB" {
			return fmt.Sprintf("%.1f %s", value, unit)
		}
	}
	return "—"
}
func fileModifiedDetail(entry os.FileInfo) string {
	if entry.ModTime().IsZero() {
		return "修改时间未知"
	}
	return entry.ModTime().Format("2006-01-02 15:04:05 -07:00")
}

func (a *App) withTransfer(p *pane, operation func(context.Context, core.RemoteFileSystem) error, refreshDirectory ...string) {
	if p == nil || p.closed || p.files == nil {
		return
	}
	connection, ftp, protocol := p.conn, p.ftp, p.files.protocol
	if connection != nil {
		bindTransferIdentity(p.files, connection.Identity)
	}
	if p.files.records == nil {
		p.files.records = &transferLog{}
	}
	records := p.files.records
	if protocol == "" {
		protocol = "SFTP"
	}
	if ftp != nil {
		protocol = "FTP"
	}
	a.withFileOperation(p, true, func(ctx context.Context) error {
		var client core.RemoteFileSystem
		var lease func()
		var err error
		if ftp != nil {
			client, err = ftp.OpenFiles(ctx)
			lease = func() {}
		} else {
			client, lease, err = connection.OpenFiles(ctx)
		}
		if err != nil {
			return err
		}
		defer lease()
		if protocol == "SCP" && connection != nil {
			client = &core.SCPFileSystem{RemoteFileSystem: client, Connection: connection}
		}
		defer client.Close()
		stop := context.AfterFunc(ctx, func() { client.Close() })
		defer stop()
		task, _ := ctx.Value(fileTransferContextKey{}).(*fileTransfer)
		wrapped := &recordedFileSystem{RemoteFileSystem: client, log: records, task: task, protocol: protocol}
		defer wrapped.finish()
		return operation(ctx, wrapped)
	}, refreshDirectory...)
}
func (a *App) withFileOperation(p *pane, refresh bool, operation func(context.Context) error, refreshDirectory ...string) {
	if p == nil || p.closed || p.files == nil || p.files.busy {
		return
	}
	f := p.files
	f.busy = true
	f.transferDirectory = ""
	if len(refreshDirectory) > 0 {
		f.transferDirectory = refreshDirectory[0]
	}
	f.statusError = false
	f.status = "正在处理文件…"
	f.started = time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	task := &fileTransfer{pane: p, files: f, started: f.started, refresh: refresh, lastProgress: f.started}
	f.activeTransfer = task
	ctx = context.WithValue(ctx, fileTransferContextKey{}, task)
	ctx = core.WithTransferLimiter(ctx, &f.limiter)
	f.cancel = cancel
	go func() {
		defer cancel()
		err := operation(ctx)
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		a.transferDone(p, task, err)
	}()
}
func (a *App) transferDone(p *pane, task *fileTransfer, err error) {
	postPane(p, func(owner *App) {
		if task == nil || p.files != task.files || p.files.activeTransfer != task {
			return
		}
		f := task.files
		f.activeTransfer = nil
		f.busy = false
		f.cancel = nil
		refreshDirectory := f.dir
		if f.transferDirectory != "" {
			refreshDirectory = f.transferDirectory
		}
		f.transferDirectory = ""
		if task.notice != nil {
			task.notice.active = false
			if err == nil {
				task.notice.status = "传输完成"
				task.notice.dismissAt = time.Now().Add(transferNoticeDismissDelay)
			} else {
				task.notice.status = "传输失败：" + err.Error()
				if errors.Is(err, context.Canceled) {
					task.notice.status = "传输已取消"
					if task.pause.Load() {
						task.notice.status = "传输已暂停"
					}
				}
			}
		}
		if err != nil {
			if errors.Is(err, context.Canceled) {
				f.status = "传输已取消"
				if task.pause.Load() {
					f.status = "传输已暂停；未开始的项目需重新选择"
				}
			} else {
				f.status = "操作失败：" + err.Error()
				f.statusError = true
			}
			if task.refresh {
				owner.readFiles(p, refreshDirectory, &fileReadNotice{status: f.status, failed: f.statusError})
			}
		} else {
			f.status = "操作完成"
			owner.status = "文件操作完成"
			if task.refresh {
				owner.loadFiles(p, refreshDirectory)
			}
		}
	})
}
func (a *App) transferProgress(p *pane, ctx context.Context) func(core.TransferProgress) {
	task, _ := ctx.Value(fileTransferContextKey{}).(*fileTransfer)
	return func(progress core.TransferProgress) {
		if task == nil || ctx.Err() != nil {
			return
		}
		postPane(p, func(_ *App) {
			if ctx.Err() != nil || p.files != task.files || p.files.activeTransfer != task {
				return
			}
			now := time.Now()
			delta := progress.Bytes - task.lastBytes
			if delta < 0 {
				delta = progress.Bytes
			}
			elapsed := now.Sub(task.lastProgress).Seconds()
			speed := int64(float64(delta) / max(elapsed, 0.1))
			task.files.status = fmt.Sprintf("%s · %s · %s/s · %d 个文件", progress.Path, bytesLabel(progress.Bytes), bytesLabel(speed), progress.Files)
			if task.notice != nil {
				task.notice.progress.Bytes = progress.Bytes
				task.notice.speed = speed
			}
			task.lastBytes = progress.Bytes
			task.lastProgress = now
		})
	}
}
func (a *App) uploadPath(p *pane, directory bool) {
	if !a.canPrepareUpload(p) {
		return
	}
	files := p.files
	files.busy = true
	go func() {
		paths, err := chooseUploadItems(a.win, directory)
		postPane(p, func(owner *App) {
			if p.files != files {
				return
			}
			files.busy = false
			if err != nil {
				files.status = "选择上传内容失败：" + err.Error()
				return
			}
			owner.prepareUpload(p, paths)
		})
	}()
}

func (a *App) download(p *pane) {
	names, directory := p.files.selectedNames(), p.files.dir
	if len(names) == 0 {
		return
	}
	for _, name := range names {
		if !core.SafeTransferName(name) {
			p.files.status = fmt.Sprintf("无法下载 %q：文件名不兼容本地文件系统，请先在服务器上重命名", name)
			p.files.statusError = true
			return
		}
	}
	a.withTransfer(p, func(ctx context.Context, client core.RemoteFileSystem) error {
		directories, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Title: "选择下载保存位置（重名自动编号）", Directory: true})
		if err != nil {
			return err
		}
		if len(directories) == 0 {
			return context.Canceled
		}
		return downloadBatch(ctx, client, names, directory, directories[0], a.transferProgress(p, ctx))
	})
}

func (a *App) uploadDropped(p *pane, files []string) {
	a.uploadDroppedOn(p, files, "")
}

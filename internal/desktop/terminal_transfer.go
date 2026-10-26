// SPDX-License-Identifier: GPL-3.0-only
package desktop

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"oshell-desktop/internal/core"
	"oshell-desktop/internal/terminal"
	"oshell-desktop/internal/zmodem"
)

func postPane(p *pane, fn func(*App)) bool {
	if owner := p.owner.Load(); owner != nil {
		return owner.post(func() {
			if current := p.owner.Load(); current != nil && !p.closed {
				fn(current)
			}
		})
	}
	return false
}

func (a *App) configureTerminalTransfer(p *pane, opts *terminal.Options, enabled bool) {
	a.configureTerminalTransferWithPicker(p, opts, enabled, chooseTransferPaths)
}

func (a *App) configureTerminalTransferWithPicker(p *pane, opts *terminal.Options, enabled bool, choose func(context.Context, *mygo.Window, bool) ([]string, error)) {
	opts.AutomaticTransfers = enabled
	opts.TransferHandler = func(ctx context.Context, upload bool, conn net.Conn) error {
		window := make(chan *mygo.Window, 1)
		if !postPane(p, func(owner *App) {
			p.transfer = transferNoticeState{active: true, visible: true, upload: upload, status: "ZMODEM · 等待选择文件"}
			window <- owner.win
		}) {
			return context.Canceled
		}
		var parent *mygo.Window
		select {
		case parent = <-window:
		case <-ctx.Done():
			return ctx.Err()
		}
		paths, err := choose(ctx, parent, upload)
		if err != nil {
			return err
		}
		if len(paths) == 0 {
			return context.Canceled
		}
		postPane(p, func(*App) { p.transfer.status = "ZMODEM · 正在协商传输" })
		started, last := time.Now(), time.Time{}
		progress := func(value zmodem.FileProgress) {
			now := time.Now()
			if now.Sub(last) < 100*time.Millisecond && value.Bytes != value.Total {
				return
			}
			last = now
			speed := int64(float64(value.Bytes) / max(now.Sub(started).Seconds(), 0.1))
			status := fmt.Sprintf("ZMODEM · %s · %s / %s · %s/s", value.Name, bytesLabel(value.Bytes), bytesLabel(value.Total), bytesLabel(speed))
			postPane(p, func(*App) {
				p.transfer.status = status
				p.transfer.progress, p.transfer.hasProgress, p.transfer.speed = transferNoticeProgress{value.Name, value.Bytes, value.Total}, true, speed
			})
		}
		if upload {
			var files []zmodem.SendFile
			for _, path := range paths {
				file, e := os.Open(path)
				if e != nil {
					return e
				}
				defer file.Close()
				info, e := file.Stat()
				if e != nil {
					return e
				}
				if !info.Mode().IsRegular() {
					return fmt.Errorf("只能上传普通文件：%s", info.Name())
				}
				files = append(files, zmodem.SendFile{Name: info.Name(), Size: info.Size(), Reader: file})
			}
			return zmodem.SendFiles(conn, files, progress)
		}
		var rejected []string
		err = zmodem.ReceiveFiles(conn, func(file zmodem.ReceiveFile) (io.WriteCloser, error) {
			writer, e := newTransferFile(paths[0], file.Name)
			if e != nil {
				rejected = append(rejected, file.Name+"："+e.Error())
			}
			return writer, e
		}, progress)
		if len(rejected) > 0 && err == nil {
			postPane(p, func(*App) {
				p.transfer.status = "ZMODEM · 已结束，跳过 " + strings.Join(rejected, "；")
				p.transfer.active = false
			})
		}
		return err
	}
	opts.TransferFinished = func(err error) {
		postPane(p, func(*App) {
			if !p.transfer.active {
				return
			}
			p.transfer.active = false
			switch {
			case err == nil:
				p.transfer.status = "ZMODEM · 传输完成"
				p.transfer.dismissAt = time.Now().Add(transferNoticeDismissDelay)
			case errors.Is(err, context.Canceled) || errors.Is(err, zmodem.ErrCancelled):
				p.transfer.status = "ZMODEM · 已取消"
			case errors.Is(err, zmodem.ErrSkipped):
				p.transfer.status = "ZMODEM · 已结束，" + err.Error()
			default:
				p.transfer.status = "ZMODEM · 传输失败：" + err.Error()
			}
		})
	}
}

func chooseTransferPaths(ctx context.Context, parent *mygo.Window, upload bool) ([]string, error) {
	title := "选择 ZMODEM 下载目录（同名文件将跳过）"
	if upload {
		title = "选择 ZMODEM 上传文件"
	}
	paths, err := mygo.Dialog.OpenContext(ctx, mygo.OpenDialogOptions{Parent: parent, Title: title, Multiple: upload, Directory: !upload, CreateDirectories: !upload})
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, context.Canceled
	}
	return paths, nil
}

// Complete downloads become visible in one operation without replacing an
// existing file. Failed/cancelled transfers leave no partial destination.
type transferFile struct {
	file    *os.File
	target  string
	aborted error
	closed  bool
}

func newTransferFile(dir, name string) (*transferFile, error) {
	if !core.SafeTransferName(name) {
		return nil, fmt.Errorf("无效的文件名")
	}
	target := filepath.Join(dir, name)
	if _, e := os.Lstat(target); !os.IsNotExist(e) {
		return nil, fmt.Errorf("目标已存在或无法访问")
	}
	f, e := os.CreateTemp(dir, ".oshell-zmodem-*")
	if e != nil {
		return nil, e
	}
	return &transferFile{file: f, target: target}, nil
}
func (f *transferFile) Write(b []byte) (int, error) { return f.file.Write(b) }
func (f *transferFile) Abort(e error)               { f.aborted = e }
func (f *transferFile) Close() error {
	if f.closed {
		return nil
	}
	f.closed = true
	defer os.Remove(f.file.Name())
	if f.aborted != nil {
		f.file.Close()
		return f.aborted
	}
	if e := f.file.Sync(); e != nil {
		f.file.Close()
		return e
	}
	if e := f.file.Close(); e != nil {
		return e
	}
	return core.PublishDownloadedFile(f.file.Name(), f.target)
}

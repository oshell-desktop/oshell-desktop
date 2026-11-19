package desktop

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"oshell-desktop/internal/terminal"
	"oshell-desktop/internal/zmodem"
)

func TestTerminalTransferCompletionCountdown(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	p := &pane{}
	p.owner.Store(a)
	var opts terminal.Options
	a.configureTerminalTransfer(p, &opts, true)
	for _, upload := range []bool{false, true} {
		p.transfer = transferNoticeState{active: true, visible: true, upload: upload}
		opts.TransferFinished(nil)
		a.drain()
		start := p.transfer.dismissAt.Add(-15 * time.Second)
		if p.transfer.active || p.transfer.remaining(start) != 15 || p.transfer.fraction() != 1 {
			t.Fatal(p.transfer)
		}
		if p.transfer.remaining(start.Add(14900*time.Millisecond)) != 1 || !p.transfer.visible {
			t.Fatal("dismissed early")
		}
		p.transfer.remaining(start.Add(15 * time.Second))
		if p.transfer.visible {
			t.Fatal("completion did not expire")
		}
		// A later transfer in this pane must survive the previous deadline.
		p.transfer = transferNoticeState{active: true, visible: true, upload: upload}
		p.transfer.remaining(start.Add(time.Minute))
		if !p.transfer.visible {
			t.Fatal("old deadline dismissed new transfer")
		}
		opts.TransferFinished(errors.New("disk full"))
		a.drain()
		p.transfer.remaining(start.Add(time.Hour))
		if !p.transfer.visible || !p.transfer.dismissAt.IsZero() {
			t.Fatal("error was automatically hidden")
		}
	}
}

func TestTerminalTransferDialogExplicitCancel(t *testing.T) {
	for _, upload := range []bool{true, false} {
		t.Run(fmt.Sprint(upload), func(t *testing.T) {
			a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
			p := &pane{id: "transfer", profile: core.Profile{Name: "test"}}
			p.owner.Store(a)
			a.tabs = []*tab{{id: "one", root: &node{pane: p}, active: p}}
			conn, peer := net.Pipe()
			defer peer.Close()
			go io.Copy(io.Discard, peer)
			started := make(chan context.Context, 1)
			opts := terminal.Options{Conn: conn, NoBlink: true}
			a.configureTerminalTransferWithPicker(p, &opts, true, func(ctx context.Context, _ *mygo.Window, direction bool) ([]string, error) {
				if direction != upload {
					return nil, fmt.Errorf("unexpected transfer direction")
				}
				started <- ctx
				<-ctx.Done()
				return nil, ctx.Err()
			})
			term, err := terminal.New(opts)
			if err != nil {
				t.Fatal(err)
			}
			defer term.Close()
			p.term = term
			frame := byte(zmodem.FrameZRINIT)
			if !upload {
				frame = zmodem.FrameZRQINIT
			}
			if _, err := peer.Write(zmodem.BuildZhexHeader(frame, 0)); err != nil {
				t.Fatal(err)
			}
			waitOperator(t, a, func() bool { return len(started) > 0 })
			ctx := <-started
			clicked := false
			tt := ui.NewTester(func(c *ui.Context) {
				ui.Button(c, "后台命令").OnClick(func() { clicked = true })
				a.transferOverlays(c)
			}, 800, 500)
			tt.Key(ui.Ctrl, ui.KeyC)
			tt.Key(0, ui.KeyEscape)
			tt.Type("input")
			tt.ClickAt(5, 5)
			if ctx.Err() != nil || clicked || !tt.HasText("ZMODEM 文件传输") {
				t.Fatal("accidental input dismissed/cancelled transfer")
			}
			if err := tt.Click("取消"); err != nil {
				t.Fatal(err)
			}
			waitOperator(t, a, func() bool { return !p.transfer.active && term.Available() })
			tt.Frame()
			if ctx.Err() == nil || tt.HasText("ZMODEM 文件传输") || p.transfer.status != "ZMODEM · 已取消" {
				t.Fatal("explicit cancellation did not restore input", p.transfer.status)
			}
			if err := tt.Click("后台命令"); err != nil || !clicked {
				t.Fatal("modal still intercepts input", err)
			}
		})
	}
}

func TestTerminalTransferOverlaysAcrossTabs(t *testing.T) {
	for _, theme := range []string{"light", "dark"} {
		t.Run(theme, func(t *testing.T) {
			a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
			a.config.Settings.InterfaceTheme = theme
			upload := &pane{id: "upload", profile: core.Profile{Name: "开发服务器"}, transfer: transferNoticeState{
				active: true, visible: true, upload: true, hasProgress: true, speed: 2 << 20,
				progress: transferNoticeProgress{Name: "release.tar.gz", Bytes: 6 << 20, Total: 10 << 20},
			}}
			download := &pane{id: "download", profile: core.Profile{Name: "测试服务器"}, transfer: transferNoticeState{
				visible: true, status: "ZMODEM · 传输完成", dismissAt: time.Now().Add(15 * time.Second), hasProgress: true,
				progress: transferNoticeProgress{Name: "服务日志.txt", Bytes: 4096, Total: 4096},
			}}
			a.tabs = []*tab{{id: "one", root: &node{pane: upload}, active: upload}, {id: "two", root: &node{pane: download}, active: download}}
			clicked := false
			tt := ui.NewTester(func(c *ui.Context) {
				a.theme(c)
				ui.Column(c).Fill().Background(c.Theme().Surface).Label("终端内容").Children(func() {
					ui.Button(c, "继续操作终端").Width(120).OnClick(func() { clicked = true })
				})
				a.transferOverlays(c)
			}, 900, 500)
			for _, text := range []string{"rz 上传", "sz 下载", "release.tar.gz", "服务日志.txt", "15 秒后自动关闭"} {
				if !tt.HasText(text) {
					t.Fatal("missing", text)
				}
			}
			r, ok := tt.Find("rz 上传")
			if !ok || r.X < 200 || r.X > 500 || r.Y < 80 {
				t.Fatalf("not centered: %+v", r)
			}
			before, _ := tt.Find("终端内容")
			if err := tt.Click("继续操作终端"); err != nil || clicked {
				t.Fatal("modal did not block terminal", err)
			}
			tt.Key(ui.Ctrl, ui.KeyC)
			tt.Key(0, ui.KeyEscape)
			if !tt.HasText("ZMODEM 文件传输") || !upload.transfer.active {
				t.Fatal("keyboard dismissed transfer dialog")
			}
			snapshot(t, tt, "zmodem-overlay-"+theme+".png")
			upload.transfer.active, upload.transfer.visible = false, false
			tt.Frame()
			if tt.HasText("ZMODEM 文件传输") {
				t.Fatal("completed transfer left modal blocking")
			}
			if err := tt.Click("继续操作终端"); err != nil || !clicked {
				t.Fatal("terminal did not resume", err)
			}
			if err := tt.Click("关闭传输进度"); err != nil {
				t.Fatal(err)
			}
			if download.transfer.visible {
				t.Fatal("closed wrong transfer")
			}
			upload.transfer.visible = false
			tt.Frame()
			after, _ := tt.Find("终端内容")
			if before != after {
				t.Fatal("overlay resized terminal", before, after)
			}
		})
	}
}

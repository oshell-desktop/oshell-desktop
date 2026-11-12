package desktop

import (
	"encoding/json"
	"fmt"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"os"
	"oshell-desktop/internal/core"
	"path/filepath"
	"strconv"
	"strings"
)

type windowSize struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

var defaultWindowSize = windowSize{1180, 760}

func (s windowSize) valid() bool {
	return s.Width >= 760 && s.Width <= 10000 && s.Height >= 460 && s.Height <= 10000
}

type windowSizeDraft struct {
	fixed         bool
	width, height string
}

func (d *windowSizeDraft) prepare(s *windowSize) {
	d.fixed = s != nil
	if s == nil {
		s = &defaultWindowSize
	}
	d.width, d.height = strconv.Itoa(s.Width), strconv.Itoa(s.Height)
}
func (d windowSizeDraft) value() (*windowSize, error) {
	if !d.fixed {
		return nil, nil
	}
	w, ew := strconv.Atoi(strings.TrimSpace(d.width))
	h, eh := strconv.Atoi(strings.TrimSpace(d.height))
	s := windowSize{w, h}
	if ew != nil || eh != nil || !s.valid() {
		return nil, fmt.Errorf("启动窗口宽度须为 760–10000、高度须为 460–10000 的整数")
	}
	return &s, nil
}
func (a *App) windowSizeSettings(c *ui.Context) {
	d := &a.windowDraft
	ui.Checkbox(c, &d.fixed, "固定启动窗口大小")
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "宽")
		ui.TextInput(c, &d.width).Width(80).Label("启动窗口宽度").Disabled(!d.fixed)
		ui.Text(c, "高")
		ui.TextInput(c, &d.height).Width(80).Label("启动窗口高度").Disabled(!d.fixed)
		ui.Text(c, "点")
		ui.Button(c, "使用当前窗口大小").Disabled(a.win == nil).OnClick(func() {
			w, h := a.win.ContentSize()
			s := windowSize{min(10000, max(760, w)), min(10000, max(460, h))}
			d.prepare(&s)
		})
	})
	ui.Text(c, "下次启动或新建窗口时生效。关闭固定尺寸会记住普通窗口大小，超过屏幕工作区时自动缩小。").Wrap().TextColor(c.Theme().TextMuted)
}
func (a *App) startupSize(previous *App) windowSize {
	if fixed := a.localUI.StartupWindow; fixed != nil {
		return *fixed
	}
	if previous != nil && previous.normalWindowSize.valid() {
		return previous.normalWindowSize
	}
	data, err := core.ReadBounded(filepath.Join(a.store.Dir, "last-window-size.json"))
	var s windowSize
	if err == nil && json.Unmarshal(data, &s) == nil && s.valid() {
		return s
	}
	return defaultWindowSize
}

// Constrain the outer frame using actual native title-bar and border dimensions.
func fitWorkspaceWindow(bounds, work mygo.Rectangle, content, requested windowSize) mygo.Rectangle {
	bounds.Width = min(work.Width, requested.Width+max(0, bounds.Width-content.Width))
	bounds.Height = min(work.Height, requested.Height+max(0, bounds.Height-content.Height))
	bounds.X = max(work.X, min(bounds.X, work.X+work.Width-bounds.Width))
	bounds.Y = max(work.Y, min(bounds.Y, work.Y+work.Height-bounds.Height))
	return bounds
}
func (a *App) rememberWindowSize() {
	if a.win.IsMinimized() || a.win.IsMaximized() || a.win.IsFullScreen() {
		return
	}
	w, h := a.win.ContentSize()
	size := windowSize{w, h}
	if size.valid() {
		a.normalWindowSize = size
	}
}
func (a *App) saveLastWindowSize() {
	if a.localUI.StartupWindow != nil || !a.normalWindowSize.valid() {
		return
	}
	data, _ := json.Marshal(a.normalWindowSize)
	err := core.AtomicWrite(filepath.Join(a.store.Dir, "last-window-size.json"), data)
	if err != nil {
		fmt.Fprintln(os.Stderr, "保存窗口尺寸失败：", err)
	}
}

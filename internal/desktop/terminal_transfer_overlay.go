package desktop

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

type transferNoticeProgress struct {
	Name         string
	Bytes, Total int64
}
type transferNoticeState struct {
	active      bool
	visible     bool
	status      string
	upload      bool
	progress    transferNoticeProgress
	hasProgress bool
	speed       int64
	dismissAt   time.Time
}

const transferNoticeDismissDelay = 15 * time.Second

// Evaluated on the UI thread. A new transfer replaces the whole state, so an
// earlier completion can never dismiss it. Hidden windows expire on redraw.
func (s *transferNoticeState) remaining(now time.Time) int {
	if !s.visible || s.active || s.dismissAt.IsZero() {
		return 0
	}
	left := s.dismissAt.Sub(now)
	if left <= 0 {
		s.visible = false
		return 0
	}
	return int((left + time.Second - 1) / time.Second)
}

func (s *transferNoticeState) fraction() float64 {
	if !s.active && !s.dismissAt.IsZero() {
		return 1
	}
	if !s.hasProgress {
		return -1
	}
	if s.progress.Total <= 0 {
		return -1
	}
	return min(1, max(0, float64(s.progress.Bytes)/float64(s.progress.Total)))
}

func (a *App) transferOverlays(c *ui.Context) {
	now := time.Now()
	var panes []*pane
	for _, p := range a.allPanes() {
		p.transfer.remaining(now)
		if p.files != nil && p.files.notice != nil {
			p.files.notice.remaining(now)
		}
		if !p.closed && (p.transfer.visible || p.files != nil && p.files.notice != nil && p.files.notice.visible) {
			panes = append(panes, p)
		}
	}
	if len(panes) == 0 {
		return
	}
	w, h := c.Size()
	top := float32(46)
	if a.focusFullscreen.chrome {
		top = 12
	}
	ui.Overlay(c.Key("transfer-notices"), func() {
		ui.Scroll(c).Absolute().Top(top).Right(12).Width(min(360, max(1, w-24))).MaxHeight(max(1, h-top-12)).Children(func() {
			ui.Column(c).Gap(8).Children(func() {
				for _, p := range panes {
					if p.transfer.visible {
						a.terminalTransferCard(c.Key("terminal-"+p.id), p, now)
					}
					if p.files != nil && p.files.notice != nil && p.files.notice.visible {
						a.fileTransferCard(c.Key("files-"+p.id), p, now)
					}
				}
			})
		})
	})
}

func (a *App) terminalTransferCard(c *ui.Context, p *pane, now time.Time) {
	s := &p.transfer
	direction := "sz 下载"
	if s.upload {
		direction = "rz 上传"
	}
	transferNoticeCard(c, s, direction, p.profile.Name, now, func() {
		if s.active {
			ui.Button(c, "取消").Height(24).Disabled(p.term == nil).OnClick(func() {
				if p.term != nil {
					p.term.CancelTransfer()
				}
			})
		}
	})
}

func transferNoticeCard(c *ui.Context, s *transferNoticeState, direction, session string, now time.Time, actions func()) {
	panelSurface(c).Gap(8).Shadow(0, 4, 16, 0, ui.RGBA(0, 0, 0, .16)).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, direction).FontSize(13).FontWeight(600).TextColor(c.Theme().Accent).Shrink(0)
			ui.Text(c, session).SingleLine().Grow(1).MinWidth(0).TextColor(c.Theme().TextMuted).Tooltip(session)
			actions()
			if !s.active {
				rowIconButton(c, "关闭传输进度", closeIcon, c.Theme().TextMuted).OnClick(func() { s.visible = false })
			}
		})
		if s.hasProgress {
			ui.Text(c, s.progress.Name).SingleLine().Tooltip(s.progress.Name)
		}
		fraction := s.fraction()
		// Failed/cancelled transfers must not retain an indeterminate animation.
		if !s.active && fraction < 0 {
			fraction = 0
		}
		ui.Progress(c, fraction).Height(5).Label(direction + "进度")
		if s.active && s.hasProgress {
			ui.Row(c).Children(func() {
				ui.Text(c, fmt.Sprintf("%s / %s", bytesLabel(s.progress.Bytes), bytesLabel(s.progress.Total))).Grow(1).TextColor(c.Theme().TextMuted)
				ui.Text(c, fmt.Sprintf("%s/s", bytesLabel(s.speed))).TextColor(c.Theme().TextMuted)
			})
		} else {
			status := strings.TrimPrefix(s.status, "ZMODEM · ")
			ui.Text(c, status).SingleLine().Tooltip(status)
		}
		if left := s.remaining(now); left > 0 {
			ui.Text(c, fmt.Sprintf("%d 秒后自动关闭", left)).FontSize(11).TextColor(c.Theme().TextMuted)
			c.After(min(time.Second, s.dismissAt.Sub(now)))
		}
	})
}

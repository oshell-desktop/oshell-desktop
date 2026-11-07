package desktop

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"oshell-desktop/internal/terminal"
)

type agentReplayItem struct {
	event *core.AgentAuditEvent
	err   error
	done  bool
}
type agentReplayState struct {
	window                 *mygo.Window
	term                   *terminal.Terminal
	cancel                 context.CancelFunc
	timer                  *time.Timer
	queue                  chan agentReplayItem
	next                   *core.AgentAuditEvent
	start, last            time.Time
	position, target       time.Duration
	paused, ended, seeking bool
	speed, seek, error, id string
}

func (a *App) openAgentReplay(id string) {
	if !mygo.App.IsReady() {
		a.mcp.error = "真实回放窗口待人工验收"
		return
	}
	if old := a.mcp.replay; old != nil && old.window != nil {
		old.window.Close()
	}
	r := &agentReplayState{id: id, speed: "1"}
	a.mcp.replay = r
	if err := a.resetAgentReplay(r, 0); err != nil {
		a.mcp.error = err.Error()
		return
	}
	w := mygo.NewWindow(mygo.WindowOptions{Title: "MCP 只读审计回放", Width: 1050, Height: 680, MinWidth: 650, MinHeight: 400, Content: ui.View(func(c *ui.Context) { a.agentReplayView(c, r) })})
	r.window = w
	w.OnClosed(func() {
		r.cancel()
		if r.timer != nil {
			r.timer.Stop()
		}
		r.term.Close()
		r.window = nil
		if a.mcp.replay == r {
			a.mcp.replay = nil
		}
	})
}
func (a *App) resetAgentReplay(r *agentReplayState, target time.Duration) error {
	if r.cancel != nil {
		r.cancel()
	}
	if r.term != nil {
		r.term.Close()
	}
	if r.timer != nil {
		r.timer.Stop()
	}
	term, err := terminal.NewReplay(terminal.Font{Family: terminalFamily(a.config.Settings.FontFamily), Size: float32(a.config.Settings.FontSize)}, terminalTheme(a.config.Settings.Scheme()))
	if err != nil {
		return err
	}
	r.term = term
	r.queue = make(chan agentReplayItem, 8)
	r.next = nil
	r.start = time.Time{}
	r.last = time.Now()
	r.position = 0
	r.target = target
	r.seeking = target > 0
	r.ended = false
	r.paused = false
	r.error = ""
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	queue := r.queue
	go func() {
		err := core.ReadAgentAudit(ctx, a.store.Dir, r.id, func(e core.AgentAuditEvent) error {
			select {
			case queue <- agentReplayItem{event: &e}:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		select {
		case queue <- agentReplayItem{err: err, done: true}:
		case <-ctx.Done():
		}
	}()
	return nil
}
func (a *App) advanceAgentReplay(r *agentReplayState) {
	now := time.Now()
	speed, _ := strconv.ParseFloat(r.speed, 64)
	if !r.paused && !r.seeking {
		r.position += time.Duration(float64(now.Sub(r.last)) * speed)
	}
	r.last = now
	for n := 0; n < 8 && !r.ended; n++ {
		if r.next == nil {
			select {
			case item := <-r.queue:
				if item.done {
					r.ended = true
					r.seeking = false
					if item.err != nil {
						r.error = item.err.Error()
					}
					return
				}
				r.next = item.event
			default:
				return
			}
		}
		e := r.next
		if r.start.IsZero() {
			r.start = e.Time
		}
		elapsed := e.Time.Sub(r.start)
		if r.seeking {
			r.position = elapsed
			if elapsed >= r.target {
				r.seeking = false
			}
		} else if r.paused || elapsed > r.position {
			return
		}
		switch e.Kind {
		case "baseline", "resize":
			cols, _ := strconv.Atoi(e.Fields["cols"])
			rows, _ := strconv.Atoi(e.Fields["rows"])
			if cols > 0 && cols <= 1000 && rows > 0 && rows <= 1000 {
				r.term.Resize(cols, rows)
			}
			if e.Kind == "baseline" {
				r.term.Feed(e.Bytes)
			}
		case "display":
			r.term.Feed(e.Bytes)
		}
		r.next = nil
	}
}
func (a *App) agentReplayView(c *ui.Context, r *agentReplayState) {
	a.theme(c)
	a.advanceAgentReplay(r)
	if r.timer != nil {
		r.timer.Stop()
	}
	if !r.ended && r.window != nil {
		window := r.window
		r.timer = time.AfterFunc(30*time.Millisecond, window.Invalidate)
	}
	ui.Column(c).Fill().Gap(4).Padding(6).Children(func() {
		ui.Row(c).Gap(8).Children(func() {
			ui.Text(c, "只读回放 · 无终端连接").Bold()
			ui.Spacer(c)
			label := "暂停"
			if r.paused {
				label = "继续"
			}
			ui.Button(c, label).Disabled(r.ended).OnClick(func() { r.paused = !r.paused; r.last = time.Now() })
			selectValue(c, "速度", &r.speed, [][2]string{{"0.5", "0.5×"}, {"1", "1×"}, {"2", "2×"}, {"4", "4×"}, {"8", "8×"}})
			ui.TextInput(c, &r.seek).Placeholder("跳转秒数").Width(100)
			ui.Button(c, "跳过空闲").Disabled(r.next == nil).OnClick(func() {
				if r.next != nil {
					r.position = r.next.Time.Sub(r.start)
					r.paused = false
				}
			})
			ui.Button(c, "跳转").OnClick(func() {
				seconds, err := strconv.ParseFloat(r.seek, 64)
				if err != nil || seconds < 0 || seconds > 365*24*3600 {
					r.error = "请输入有效秒数"
					return
				}
				if err = a.resetAgentReplay(r, time.Duration(seconds*float64(time.Second))); err != nil {
					r.error = err.Error()
				}
			})
		})
		status := fmt.Sprintf("位置 %.1f 秒", r.position.Seconds())
		if r.seeking {
			status += " · 正在顺序定位…"
		}
		if r.ended {
			status += " · 扫描结束"
		}
		ui.Text(c, status).FontSize(11)
		if r.error != "" {
			ui.Text(c, r.error+"；仅展示已认证部分，不能视为完整记录。").TextColor(c.Theme().Danger)
		}
		terminal.View(c, r.term).Fill()
	})
}

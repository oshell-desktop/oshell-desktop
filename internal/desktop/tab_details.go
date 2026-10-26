package desktop

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"os/user"
	"strconv"
	"strings"
	"time"
)

type tabHoverState struct {
	since, last        time.Time
	feedback, identity string
	panel              ui.Handle
	suppressed         bool
}

func (s *tabHoverState) dismiss() { *s = tabHoverState{panel: s.panel, suppressed: true} }

func (a *App) dismissTabDetails() {
	for _, t := range a.tabs {
		t.hover.dismiss()
	}
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *App) observeTabDetails(c *ui.Context) {
	c.ObserveInteractions(func(ev ui.WindowInteraction) {
		for _, t := range a.tabs {
			s := &t.hover
			if ev.Keyboard || ev.Blur || ev.Pointer && !s.panel.Bounds(c).Contains(ev.X, ev.Y) {
				s.dismiss()
			}
		}
	})
}

type tabDetailField struct{ label, value string }

var copyDetailIcon = glyph(`<rect x="7" y="6" width="10" height="11" rx="1.5"/><path d="M13 3H4a1 1 0 0 0-1 1v9"/>`)

func (a *App) paneDetailFields(p *pane) []tabDetailField {
	if p == nil {
		return nil
	}
	peer := ""
	if p.conn != nil {
		peer, _ = p.conn.ObservedPeer()
	}
	name := p.profile.Name
	if saved, ok := a.profileByID(p.profile.ID); ok {
		name = saved.Name
	}
	username := p.profile.Username
	if p.profile.Kind == "local" || p.profile.Kind == "blank" {
		username = ""
		if p.term == nil || !p.term.ToolConnectionActive() {
			if current, err := user.Current(); err == nil {
				username = current.Username
			}
		}
	}
	fields := []tabDetailField{{"会话名称", name}, {"主机名", p.hostObservation.Identity.Hostname}, {"主机 IP", p.hostObservation.Identity.Address}, {"连接 IP", peer}, {"登录用户名", username}}
	if p.profile.Kind == "ssh" || p.profile.Kind == "sftp" {
		fields = append(fields, tabDetailField{"配置主机", p.profile.Host}, tabDetailField{"配置端口", strconv.Itoa(p.profile.Port)})
	}
	return fields
}

// A non-modal hover card: copying a field does not select a tab or focus a button.
func (a *App) tabDetailsHover(c *ui.Context, anchor ui.Element, t *tab) {
	if !c.WindowFocused() || a.modalActive() || a.tabNativeDrag || anchor.Pressed() {
		t.hover.dismiss()
		return
	}
	now := time.Now()
	s := &t.hover
	if s.suppressed {
		if !anchor.Hovered() {
			s.suppressed = false
		}
		return
	}
	identity := t.name + "\x00" + a.tabConnectionDetails(t)
	if t.active != nil {
		identity += "\x00" + t.active.id
		for _, f := range a.paneDetailFields(t.active) {
			identity += "\x00" + f.value
		}
	}
	if s.identity != "" && s.identity != identity {
		s.dismiss()
		return
	}
	s.identity = identity
	if anchor.Hovered() {
		if s.since.IsZero() {
			s.since = now
		}
		s.last = now
		c.After(500 * time.Millisecond)
	}
	if s.since.IsZero() || now.Sub(s.last) > 250*time.Millisecond {
		*s = tabHoverState{panel: s.panel}
		return
	}
	if now.Sub(s.since) < 500*time.Millisecond {
		return
	}
	ui.Overlay(c, func() {
		panel := ui.Column(c.Key("tab-details-"+t.id)).KeepFocusOnClick().AttachTo(anchor, ui.AnchorBottomLeft, ui.AnchorTopLeft).Width(460).MaxHeight(480).Padding(12).Gap(8).Radius(6).Background(c.Theme().Surface).Border(1, c.Theme().Border).Label("标签连接详情")
		panel.Bind(&s.panel)
		panel.Children(func() {
			ui.Text(c, t.name).Bold()
			p := t.active
			for _, field := range a.paneDetailFields(p) {
				f := field
				value := f.value
				if value == "" {
					value = "未获取"
				}
				ui.Row(c).FillWidth().Padding(3, 0).Gap(8).Children(func() {
					ui.Text(c, f.label).Width(86).TextColor(c.Theme().TextMuted)
					ui.Text(c, value).Grow(1).MinWidth(0).SingleLine().Tooltip(value)
					if f.value != "" {
						ui.Row(c).Padding(3, 5).Cursor(ui.CursorPointer).Label("复制" + f.label).OnClick(func() { c.WriteClipboard(f.value); s.feedback = "已复制" + f.label }).Children(func() { ui.Icon(c, copyDetailIcon).FontSize(15).TextColor(c.Theme().Accent) })
					}
				})
			}
			ui.Text(c, "主机 IP 与本机 TCP 对端可能不同；分屏标签显示当前活动分屏。").Wrap().FontSize(11).TextColor(c.Theme().TextMuted)
			ui.Row(c).Gap(12).Children(func() {
				ui.Row(c).Padding(3, 5).Cursor(ui.CursorPointer).Label("复制完整连接详情").OnClick(func() {
					var fields []string
					for _, f := range a.paneDetailFields(p) {
						if f.value != "" {
							fields = append(fields, f.label+"："+f.value)
						}
					}
					c.WriteClipboard(strings.Join(fields, "\n") + "\n\n" + a.tabConnectionDetails(t))
					s.feedback = "已复制全部会话详情"
				}).Children(func() { ui.Text(c, "复制全部详情").TextColor(c.Theme().Accent) })
				ui.Row(c).Padding(3, 5).Cursor(ui.CursorPointer).Label("查看连接详情").OnClick(func() {
					details := a.tabConnectionDetails(t)
					t.hover.dismiss()
					if mygo.App.IsReady() {
						mygo.Dialog.Message(mygo.MessageOptions{Parent: a.win, Title: "会话连接详情", Message: details, Buttons: []string{"关闭"}})
					}
				}).Children(func() { ui.Text(c, "查看连接详情…").TextColor(c.Theme().Accent) })
			})
			feedback := s.feedback
			if feedback == "" {
				feedback = "点击右侧按钮复制对应字段"
			}
			ui.Text(c, feedback).FontSize(11).TextColor(c.Theme().TextMuted)

		})
		if panel.Hovered() {
			s.last = now
		}
		c.After(250 * time.Millisecond)
	})
}

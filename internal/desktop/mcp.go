package desktop

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

type mcpPanelState struct {
	detaching                                                                bool
	sidebarShown                                                             bool
	open, busy, confirmIdle, confirmDelete                                   bool
	window                                                                   *mygo.Window
	poll                                                                     *time.Timer
	clients                                                                  *core.MCPClients
	server                                                                   *core.MCPServer
	error, name, kind, configuration, client, session, permission, sensitive string
	query, filterClient, filterSession, filterTask, from, to                 string
	archives                                                                 []core.AgentAuditIndex
	archiveBytes                                                             int64
	exportPassword, selectedArchive                                          string
	replay                                                                   *agentReplayState
}

func (a *App) agentCoordinator() *core.AgentCoordinator {
	if a.hub != nil && a.hub.agent != nil {
		return a.hub.agent
	}
	return a.agent
}
func (a *App) mcpClients() *core.MCPClients {
	if a.hub != nil {
		return a.hub.mcpClients
	}
	return a.mcp.clients
}
func (a *App) mcpServer() *core.MCPServer {
	if a.hub != nil {
		return a.hub.mcpServer
	}
	return a.mcp.server
}
func (a *App) setMCPServer(server *core.MCPServer) {
	if a.hub != nil {
		a.hub.mcpServer = server
	} else {
		a.mcp.server = server
	}
}
func (a *App) showMCP() {
	if a.workspaceLocked() {
		a.showVault()
		return
	}
	f := &a.mcp
	f.open = true
	f.error = ""
	if f.kind == "" {
		f.kind = "Codex"
	}
	if f.permission == "" {
		f.permission = "ask"
	}
	if a.mcpClients() == nil {
		clients, err := core.OpenMCPClients(a.store.Dir)
		if err != nil {
			f.error = err.Error()
			return
		}
		if a.hub != nil {
			a.hub.mcpClients = clients
		} else {
			f.clients = clients
		}
	}
	a.refreshMCPArchives()
	if f.window != nil {
		f.open = false
		f.window.Show()
		f.window.Focus()
	}
}
func (a *App) closeMCPPanel() {
	f := &a.mcp
	f.open = false
	f.configuration = ""
	f.sensitive = ""
	f.exportPassword = ""
	if f.poll != nil {
		f.poll.Stop()
		f.poll = nil
	}
}
func (a *App) mcpRun(work func() error) {
	if a.mcp.busy {
		return
	}
	a.mcp.busy = true
	a.mcp.error = ""
	go func() {
		err := work()
		a.post(func() {
			a.mcp.busy = false
			if err != nil {
				a.mcp.error = err.Error()
			}
		})
	}()
}
func (a *App) toggleMCP() {
	if a.mcp.busy || a.mcpClients() == nil {
		return
	}
	if server := a.mcpServer(); server != nil {
		a.setMCPServer(nil)
		a.mcpRun(func() error { a.agentCoordinator().RevokeAll(); return server.Close() })
		return
	}
	server, err := core.StartMCPServer(a.store.Dir, a.mcpClients(), a.agentCoordinator().Call, a.agentCoordinator().Disconnected)
	if err != nil {
		a.mcp.error = err.Error()
		return
	}
	a.setMCPServer(server)
}
func (a *App) addMCPClient() {
	clients := a.mcpClients()
	if clients == nil {
		return
	}
	c, token, err := clients.Add(a.mcp.name)
	if err != nil {
		a.mcp.error = err.Error()
		return
	}
	executable, err := os.Executable()
	if err != nil {
		a.mcp.error = err.Error()
		return
	}
	a.mcp.configuration = core.MCPClientConfiguration(executable, a.store.Dir, token, a.mcp.kind)
	a.mcp.client = c.ID
	a.mcp.name = ""
	a.mcp.error = ""
}
func (a *App) mcpWindow() {
	if !mygo.App.IsReady() {
		return
	}
	f := &a.mcp
	f.open = false
	if f.window != nil {
		f.window.Show()
		f.window.Focus()
		return
	}
	w := mygo.NewWindow(mygo.WindowOptions{Title: "MCP 控制中心", Width: 900, Height: 760, MinWidth: 650, MinHeight: 450, Content: ui.View(func(c *ui.Context) {
		a.drain()
		if !c.Valid() {
			return
		}
		a.theme(c)
		ui.Column(c).Fill().Padding(8).Gap(6).Children(func() { a.mcpContents(c) })
	})})
	f.window = w
	w.OnClosed(func() {
		if f.window == w {
			f.window = nil
			a.closeMCPPanel()
		}
	})
}
func (a *App) mcpDialog(c *ui.Context) {
	f := &a.mcp
	if f.sidebarShown {
		return
	}
	desktopModal(c, &f.open, func() { dialogBody(c, 860).Gap(10).Children(func() { a.mcpContents(c) }) })
	if !f.open && f.window == nil && f.poll != nil {
		a.closeMCPPanel()
	}
}
func (a *App) mcpContents(c *ui.Context) {
	f := &a.mcp
	if a.workspaceLocked() {
		ui.Text(c, "请先解锁工作区")
		return
	}
	if f.poll == nil && mygo.App.IsReady() {
		f.poll = time.AfterFunc(500*time.Millisecond, func() { a.post(func() { f.poll = nil }) })
	}
	ui.Row(c).Wrap().Gap(8).Children(func() {
		windowHeading(c, "MCP 控制中心")
		ui.Spacer(c)
		if f.window == nil {
			ui.Button(c, "独立窗口").OnClick(a.mcpWindow)
		}
		ui.Button(c, "关闭面板").OnClick(func() {
			if f.window != nil {
				f.window.Close()
			}
			a.closeMCPPanel()
		})
	})
	ui.Text(c, "外部 Agent 只能访问逐个授权的现有终端；服务默认关闭，配对和审计仅保存在本机。").TextColor(c.Theme().TextMuted)
	ui.Row(c).Wrap().Gap(10).Children(func() {
		label := "开启本机 MCP 服务"
		if a.mcpServer() != nil {
			label = "关闭服务并撤销授权"
		}
		ui.Button(c, label).Disabled(f.busy || a.mcpClients() == nil).OnClick(a.toggleMCP)
		if a.mcpServer() != nil {
			ui.Text(c, "运行中 · 本机私有通道").TextColor(c.Theme().Accent)
		} else {
			ui.Text(c, "服务已关闭")
		}
	})
	if f.error != "" {
		ui.Text(c, f.error).TextColor(c.Theme().Danger)
	}
	ui.Scroll(c).Height(510).MinHeight(100).Shrink(1).Padding(8).Gap(8).Background(panelColor(c)).Border(1, c.Theme().Border).Children(func() {
		ui.Column(c).Gap(7).Children(func() {
			ui.Text(c, "客户端接入").Bold()
			ui.Row(c).Wrap().Gap(8).Children(func() {
				ui.TextInput(c, &f.name).Placeholder("客户端名称").Grow(1)
				selectValue(c, "接入类型", &f.kind, [][2]string{{"Codex", "Codex"}, {"OpenCode", "OpenCode"}})
				ui.Button(c, "添加客户端").Disabled(f.busy || a.mcpClients() == nil).OnClick(a.addMCPClient)
			})
			if f.configuration != "" {
				ui.Text(c, "配对凭据仅此次显示。请复制并合并到客户端配置；OShell 不会改写外部配置。").TextColor(c.Theme().TextMuted)
				ui.TextArea(c, &f.configuration).ReadOnly(true).Height(115)
				ui.Row(c).Wrap().Gap(8).Children(func() {
					ui.Button(c, "复制接入配置").OnClick(func() { c.WriteClipboard(f.configuration) })
					ui.Button(c, "已保存，隐藏凭据").OnClick(func() { f.configuration = "" })
				})
			}
			if clients := a.mcpClients(); clients != nil {
				for _, client := range clients.List() {
					ui.Row(c.Key("client-" + client.ID)).Gap(8).Children(func() {
						ui.Text(c, client.Name+" · "+client.Created.Local().Format("2006-01-02")).Grow(1)
						ui.Button(c, "撤销配对").Disabled(f.busy).OnClick(func() {
							a.mcpRun(func() error {
								if err := clients.Revoke(client.ID); err != nil {
									return err
								}
								a.agentCoordinator().RevokeClient(client.ID)
								return nil
							})
						})
					})
				}
			}
		})
		ui.Column(c).Gap(7).Children(func() {
			ui.Text(c, "会话授权").Bold()
			clients := [][2]string{{"", "选择配对客户端"}}
			if registry := a.mcpClients(); registry != nil {
				for _, client := range registry.List() {
					clients = append(clients, [2]string{client.ID, client.Name})
				}
			}
			selectValue(c, "客户端", &f.client, clients)
			sessions := [][2]string{{"", "选择已连接终端"}}
			for _, s := range a.agentCoordinator().Sessions() {
				v := s.State()
				if v.Connected && !v.Authorized && v.Audit == "" {
					sessions = append(sessions, [2]string{v.ID, v.Title})
				}
			}
			selectValue(c, "待授权终端", &f.session, sessions)
			selectValue(c, "授权权限", &f.permission, [][2]string{{"ask", "请求批准"}, {"assist", "帮我批准（简单查询）"}, {"full", "完全访问（仍审批复杂语法）"}})
			ui.Button(c, "授权此终端").Disabled(f.busy || a.mcpServer() == nil || f.client == "" || f.session == "").OnClick(func() {
				s := a.agentCoordinator().Session(f.session)
				client, permission := f.client, f.permission
				if s != nil {
					a.mcpRun(func() error { return s.Grant(client, permission) })
				}
			})
			for _, s := range a.agentCoordinator().Sessions() {
				v := s.State()
				if v.Audit == "" {
					continue
				}
				a.mcpSessionCard(c.Key(v.ID), s, v)
			}
		})
		a.mcpArchives(c)
	})
	ui.Text(c, "Ctrl+C 或 0.6 秒内双按 Esc 可接管。中断请求、客户端断开和超时均不等于远端命令退出。").FontSize(11).TextColor(c.Theme().TextMuted)
}
func (a *App) mcpSessionCard(c *ui.Context, s *core.AgentSession, v core.AgentSessionState) {
	f := &a.mcp
	panelSurface(c).Children(func() {
		ui.Text(c, v.Title+" · "+v.Permission).Bold()
		if v.Problem != "" {
			ui.Text(c, v.Problem).TextColor(c.Theme().TextMuted)
		}
		ui.Row(c).Wrap().Gap(6).Children(func() {
			ui.Text(c, fmt.Sprintf("连接 %t · 授权 %t · Shell 就绪 %t", v.Connected, v.Authorized, v.ShellReady)).FontSize(11)
			ui.Spacer(c)
			ui.Button(c, "接管").Disabled(f.busy).OnClick(func() { a.mcpRun(func() error { return s.Takeover(context.Background(), false) }) })
			ui.Button(c, "接管并请求 Ctrl+C").Disabled(f.busy).OnClick(func() { a.mcpRun(func() error { return s.Takeover(context.Background(), true) }) })
		})
		ui.Checkbox(c, &f.confirmIdle, "我确认所操作终端是空闲的 Bash/Zsh")
		ui.Row(c).Wrap().Gap(6).Children(func() {
			ui.Button(c, "启用临时 Shell 集成").Disabled(f.busy || !v.Authorized || !v.Connected || !f.confirmIdle).OnClick(func() { f.confirmIdle = false; a.mcpRun(func() error { return s.Integrate(context.Background()) }) })
			ui.MenuButton(c, "调整权限", func(m *ui.Menu) {
				for _, p := range [][2]string{{"ask", "请求批准"}, {"assist", "帮我批准"}, {"full", "完全访问"}} {
					if m.Item(p[1]).Chosen() {
						a.mcpRun(func() error { return s.ChangePermission(p[0]) })
					}
				}
			}).Disabled(f.busy || !v.Authorized)
			ui.Button(c, "撤销授权").Disabled(f.busy || !v.Authorized).OnClick(func() { a.mcpRun(func() error { s.Revoke(); return nil }) })
			ui.Button(c, "结束存档").Disabled(f.busy).OnClick(func() { a.mcpRun(s.EndArchive) })
		})
		if op := v.Operation; op != nil {
			ui.Text(c, "操作 "+op.ID+" · "+op.State).FontSize(11)
			if op.State == "pending_approval" {
				command := op.Command
				ui.TextArea(c, &command).ReadOnly(true).Height(85)
				ui.Row(c).Wrap().Gap(8).Children(func() {
					ui.Button(c, "拒绝此命令").Disabled(f.busy).OnClick(func() { a.mcpRun(func() error { return s.Approve(op.ID, op.Command, false) }) })
					ui.PrimaryButton(c, "批准此命令原文").Disabled(f.busy).OnClick(func() { a.mcpRun(func() error { return s.Approve(op.ID, op.Command, true) }) })
				})
			}
		}
		ui.Row(c).Wrap().Gap(7).Children(func() {
			passwordInput(c, &f.sensitive).Placeholder("当前密码提示的敏感输入").Grow(1)
			ui.Button(c, "接管并发送敏感输入").Disabled(f.busy || !v.Connected).OnClick(func() {
				value := f.sensitive
				f.sensitive = ""
				a.mcpRun(func() error { return s.SensitiveInput(context.Background(), value) })
			})
			if v.Sensitive {
				ui.Button(c, "确认恢复观察").OnClick(func() { s.ResumeObservation() })
			}
		})
	})
}
func (a *App) refreshMCPArchives() {
	f := &a.mcp
	if f.busy {
		return
	}
	f.busy = true
	query, client, session, task, fromText, toText := f.query, f.filterClient, f.filterSession, f.filterTask, f.from, f.to
	go func() {
		var from, to time.Time
		var err error
		if fromText != "" {
			from, err = time.ParseInLocation("2006-01-02", fromText, time.Local)
		}
		if toText != "" && err == nil {
			to, err = time.ParseInLocation("2006-01-02", toText, time.Local)
			to = to.Add(24*time.Hour - time.Nanosecond)
		}
		var list []core.AgentAuditIndex
		var size int64
		if err == nil {
			_, size, err = core.ListAgentAudits(a.store.Dir)
		}
		if err == nil {
			list, err = core.SearchAgentAudits(context.Background(), a.store.Dir, query, client, session, task, from, to)
		}
		a.post(func() {
			f.busy = false
			if err != nil {
				f.error = err.Error()
			} else {
				f.archives = list
				f.archiveBytes = size
			}
		})
	}()
}
func (a *App) mcpArchives(c *ui.Context) {
	f := &a.mcp
	ui.Text(c, "审计与回放").Bold()
	ui.Text(c, fmt.Sprintf("本机存档 %.1f MiB · 无自动删除", float64(f.archiveBytes)/(1<<20))).TextColor(c.Theme().TextMuted)
	if f.archiveBytes >= 10<<30 {
		ui.Text(c, "存档已达到 10 GiB，请按需导出或手动清理。").TextColor(c.Theme().Danger)
	}
	ui.Row(c).Wrap().Gap(6).Children(func() {
		ui.TextInput(c, &f.query).Placeholder("全文搜索，最多 100 条").Grow(1)
		ui.Button(c, "刷新 / 搜索").Disabled(f.busy).OnClick(a.refreshMCPArchives)
	})
	ui.Row(c).Wrap().Gap(6).Children(func() {
		ui.TextInput(c, &f.filterClient).Placeholder("客户端 ID（可选）").Grow(1)
		ui.TextInput(c, &f.filterSession).Placeholder("会话 ID（可选）").Grow(1)
		ui.TextInput(c, &f.filterTask).Placeholder("任务 ID（可选）").Grow(1)
	})
	ui.Row(c).Wrap().Gap(6).Children(func() {
		ui.TextInput(c, &f.from).Placeholder("起始日期 YYYY-MM-DD").Grow(1)
		ui.TextInput(c, &f.to).Placeholder("结束日期 YYYY-MM-DD").Grow(1)
	})
	passwordInput(c, &f.exportPassword).Placeholder("加密导出密码（至少 12 个字符）")
	ui.Checkbox(c, &f.confirmDelete, "确认删除所选本机存档（不删除幂等收据）")
	for _, archive := range f.archives {
		ui.Row(c.Key(archive.ID)).Gap(7).Children(func() {
			status := "未封口"
			if archive.Sealed {
				status = "已封口"
			}
			if archive.Problem != "" {
				status = archive.Problem
			}
			ui.Text(c, archive.Title+" · "+archive.Started.Local().Format("01-02 15:04")+" · "+status).Grow(1)
			ui.Button(c, "回放").Disabled(f.busy).OnClick(func() { a.openAgentReplay(archive.ID) })
			ui.Button(c, "加密导出").Disabled(f.busy || !archive.Sealed || len([]rune(f.exportPassword)) < 12).OnClick(func() { password := f.exportPassword; f.exportPassword = ""; a.exportAgentAudit(archive.ID, password) })
			ui.Button(c, "删除").Disabled(f.busy || !f.confirmDelete).OnClick(func() {
				f.confirmDelete = false
				a.mcpRun(func() error { return a.agentCoordinator().DeleteArchive(archive.ID) })
			})
		})
		if archive.Preview != "" {
			ui.Text(c, archive.Preview).FontSize(11).TextColor(c.Theme().TextMuted).Wrap()
		}
	}
}
func (a *App) exportAgentAudit(id, password string) {
	a.mcpRun(func() error {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: a.mcp.window, Title: "导出加密审计", DefaultPath: "oshell-audit.export"})
		if err != nil || path == "" {
			return err
		}
		return core.ExportAgentAudit(context.Background(), a.store.Dir, id, password, path)
	})
}

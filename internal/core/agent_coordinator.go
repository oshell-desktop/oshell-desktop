package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

type AgentTerminal interface {
	AgentSend(context.Context, []byte, string) error
	AgentSnapshot() (screen []byte, text string, cols, rows int, eligible bool)
}
type AgentOperation struct {
	ID                                              string `json:"operation_id"`
	State                                           string `json:"state"`
	ExitCode                                        *int   `json:"exit_code,omitempty"`
	Command                                         string `json:"-"`
	Task, Client, Key, Digest, Revision, Generation string `json:"-"`
}
type AgentSessionState struct {
	ID                                           string `json:"session_id"`
	Generation                                   string `json:"generation"`
	Title                                        string `json:"title"`
	Client                                       string `json:"client"`
	Permission                                   string `json:"permission"`
	Authorized, Connected, ShellReady, Sensitive bool
	Task, Audit, Problem                         string
	Operation                                    *AgentOperation
}
type AgentCoordinator struct {
	taskCount atomic.Int64
	receiptMu sync.Mutex
	mu        sync.Mutex
	root      string
	sessions  map[string]*AgentSession
}
type AgentSession struct {
	displayRedactor                                   agentRedactor
	size                                              atomic.Uint64
	taskConnection                                    string
	receiptMu                                         *sync.Mutex
	recording                                         atomic.Bool
	integrationPending                                bool
	lastCols, lastRows                                int
	mu                                                sync.Mutex
	root, id, generation, title                       string
	peer                                              AgentTerminal
	client, permission, revision, task, token, ticket string
	authorized, connected, integrated, sensitive      bool
	problem                                           string
	owner                                             atomic.Bool
	audit                                             *AgentAudit
	operations                                        map[string]*AgentOperation
	current                                           *AgentOperation
	redactor                                          agentRedactor
	marker                                            []byte
	lastEscape                                        time.Time
	timer                                             *time.Timer
}

func NewAgentCoordinator(root string) *AgentCoordinator {
	return &AgentCoordinator{root: root, sessions: map[string]*AgentSession{}}
}
func (c *AgentCoordinator) Register(id, title string) *AgentSession {
	c.mu.Lock()
	defer c.mu.Unlock()
	if old := c.sessions[id]; old != nil {
		old.Disconnect()
	}
	s := &AgentSession{receiptMu: &c.receiptMu, root: c.root, id: id, title: title, generation: NewID(), operations: map[string]*AgentOperation{}}
	c.sessions[id] = s
	return s
}
func (c *AgentCoordinator) Sessions() []*AgentSession {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := []*AgentSession{}
	for _, s := range c.sessions {
		out = append(out, s)
	}
	return out
}
func (c *AgentCoordinator) Session(id string) *AgentSession {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sessions[id]
}
func (c *AgentCoordinator) Close() {
	for _, s := range c.Sessions() {
		s.Disconnect()
		_ = s.EndArchive()
	}
}
func (c *AgentCoordinator) RevokeClient(id string) {
	for _, s := range c.Sessions() {
		s.mu.Lock()
		if s.client == id {
			s.revoke("客户端配对已撤销")
		}
		s.mu.Unlock()
	}
}
func (c *AgentCoordinator) RevokeAll() {
	for _, s := range c.Sessions() {
		s.Revoke()
	}
}
func (c *AgentCoordinator) Disconnected(connection string) {
	for _, s := range c.Sessions() {
		s.mu.Lock()
		if s.task != "" && s.taskConnection == connection {
			s.takeover("MCP 客户端连接已断开；远端命令结果仍需确认")
		}
		s.mu.Unlock()
	}
}
func (c *AgentCoordinator) DeleteArchive(id string) error {
	for _, s := range c.Sessions() {
		s.mu.Lock()
		live := s.audit != nil && s.audit.ID() == id
		s.mu.Unlock()
		if live {
			return fmt.Errorf("请先结束该会话的存档")
		}
	}
	return DeleteAgentAudit(c.root, id)
}
func (s *AgentSession) Bind(peer AgentTerminal) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.peer = peer
	s.connected = true
}
func (s *AgentSession) Busy() bool      { return s.owner.Load() }
func (s *AgentSession) Recording() bool { return s.recording.Load() }
func (s *AgentSession) State() AgentSessionState {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := AgentSessionState{ID: s.id, Generation: s.generation, Title: s.title, Client: s.client, Permission: s.permission, Authorized: s.authorized, Connected: s.connected, ShellReady: s.integrated, Sensitive: s.sensitive, Task: s.task, Problem: s.problem}
	if s.audit != nil {
		r.Audit = s.audit.ID()
		if err := s.audit.Err(); err != nil {
			r.Problem = err.Error()
		}
	}
	if s.current != nil {
		copy := *s.current
		r.Operation = &copy
	}
	return r
}
func validAgentPermission(permission string) bool {
	return permission == "ask" || permission == "assist" || permission == "full"
}
func (s *AgentSession) Grant(client, permission string) error {
	if !validAgentID(client) || !validAgentPermission(permission) {
		return fmt.Errorf("客户端或权限无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.connected || s.peer == nil || s.audit != nil {
		return fmt.Errorf("终端未连接，或已有授权/存档，请先结束旧存档")
	}
	screen, _, cols, rows, eligible := s.peer.AgentSnapshot()
	if !eligible {
		return fmt.Errorf("只允许已连接且没有交互工具或文件传输的 Shell")
	}
	audit, err := NewAgentAudit(s.root, AgentAuditIndex{Session: s.id, Client: client, Title: s.title}, []byte(s.redactor.text(string(screen))), cols, rows)
	if err != nil {
		return err
	}
	s.client, s.permission, s.revision, s.authorized, s.audit = client, permission, NewID(), true, audit
	s.recording.Store(true)
	s.lastCols, s.lastRows = cols, rows
	s.NoteResize(cols, rows)
	s.operations = map[string]*AgentOperation{}
	s.problem = "请确认当前为空闲 Bash/Zsh，再启用临时 Shell 集成"
	return nil
}
func (s *AgentSession) event(kind string, data []byte, fields map[string]string) error {
	if s.audit == nil {
		return fmt.Errorf("尚未开启审计")
	}
	op := ""
	if s.current != nil {
		op = s.current.ID
	}
	err := s.audit.Append(AgentAuditEvent{Kind: kind, Operation: op, Bytes: data, Fields: fields})
	if err != nil {
		s.problem = err.Error()
		s.ticket = ""
		s.integrated = false
		s.owner.Store(false)
	}
	return err
}
func (s *AgentSession) takeover(reason string) {
	s.integrationPending = false
	s.ticket = ""
	s.task = ""
	s.owner.Store(false)
	s.revision = NewID()
	s.integrated = false
	s.problem = reason
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	if s.current != nil && s.current.State != "completed" {
		if s.current.State == "pending_approval" || s.current.State == "queued" {
			s.current.State = "cancelled_before_send"
		} else {
			s.current.State = "unknown_after_takeover"
		}
	}
	if s.audit != nil {
		_ = s.event("takeover", nil, map[string]string{"reason": reason})
	}
}
func (s *AgentSession) revoke(reason string) { s.takeover(reason); s.authorized = false }
func (s *AgentSession) Revoke() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revoke("授权已撤销；终端连接和存档保留")
}
func (s *AgentSession) ChangePermission(permission string) error {
	if !validAgentPermission(permission) {
		return fmt.Errorf("权限无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.authorized {
		return fmt.Errorf("终端未授权")
	}
	s.takeover("权限已变化，请重新开始任务和确认 Shell")
	s.permission = permission
	return s.event("permission", nil, map[string]string{"permission": permission})
}
func (s *AgentSession) Disconnect() {
	s.mu.Lock()
	s.connected = false
	s.revoke("连接已结束，旧授权失效")
	s.mu.Unlock()
	_ = s.EndArchive()
}
func (s *AgentSession) ContextChanged(reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.authorized || s.task != "" {
		s.takeover(reason)
	}
}
func (s *AgentSession) EndArchive() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revoke("存档已结束")
	if s.audit == nil {
		return nil
	}
	s.marker = nil
	if b := s.redactor.consume(nil, true); len(b) > 0 {
		_ = s.event("output", b, nil)
	}
	if b := s.displayRedactor.consume(nil, true); len(b) > 0 {
		_ = s.event("display", b, nil)
	}
	err := s.audit.Close()
	s.audit = nil
	s.recording.Store(false)
	return err
}
func (s *AgentSession) Integrate(ctx context.Context) error {
	s.mu.Lock()
	if !s.authorized || !s.connected || s.peer == nil || s.audit == nil || s.sensitive {
		s.mu.Unlock()
		return fmt.Errorf("请先授权已连接的终端并结束敏感输入")
	}
	_, _, _, _, eligible := s.peer.AgentSnapshot()
	if !eligible {
		s.mu.Unlock()
		return fmt.Errorf("当前不是可接管的空闲 Shell")
	}
	if s.current != nil && (s.current.State == "running" || s.current.State == "dispatched") {
		s.mu.Unlock()
		return fmt.Errorf("请先确认现有命令已结束")
	}
	s.current = nil
	s.token = NewID()
	s.ticket = NewID()
	ticket := s.ticket
	s.integrated = false
	s.integrationPending = true
	data := agentIntegrationCommand(s.token)
	if err := s.event("integration_approval", data, nil); err != nil {
		s.mu.Unlock()
		return err
	}
	peer := s.peer
	s.owner.Store(true)
	s.startTimer(10*time.Second, "未收到 Shell 集成确认，请人工检查")
	s.mu.Unlock()
	err := peer.AgentSend(ctx, data, ticket)
	if err != nil {
		s.ContextChanged("Shell 集成未确认写入：" + err.Error())
	}
	return err
}
func (s *AgentSession) startTimer(duration time.Duration, reason string) {
	if s.timer != nil {
		s.timer.Stop()
	}
	revision := s.revision
	s.timer = time.AfterFunc(duration, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.revision == revision {
			s.takeover(reason)
		}
	})
}

// Gate runs at the actual PTY write, on the terminal's serial input worker.
// Revocation and the authorization check serialize with that write, not its UI enqueue.
func (s *AgentSession) Gate(ticket string, data []byte, write func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ticket != "" {
		if ticket != s.ticket || !s.connected || s.audit == nil || s.audit.Err() != nil {
			return fmt.Errorf("自动输入已失效或审计不可用")
		}
		if strings.HasPrefix(ticket, "sensitive:") {
			if !s.sensitive {
				return fmt.Errorf("敏感输入已取消")
			}
		} else if strings.HasPrefix(ticket, "interrupt:") {
			// Explicit user/task_stop interrupt, already recorded without claiming exit.
		} else if !s.authorized {
			return fmt.Errorf("自动输入授权已撤销")
		}
		if err := s.event("input_queued", nil, nil); err != nil {
			return err
		}
		err := write()
		s.ticket = ""
		if err != nil {
			s.takeover("输入写入结果未知，禁止自动重试")
			return err
		}
		return s.event("input_written", nil, nil)
	}
	if s.owner.Load() {
		interrupt := bytes.Equal(data, []byte{3})
		escape := bytes.Equal(data, []byte{27})
		doubleEscape := escape && time.Since(s.lastEscape) <= 600*time.Millisecond
		if escape {
			s.lastEscape = time.Now()
		}
		if !interrupt && !doubleEscape {
			return fmt.Errorf("终端由 MCP 占用，请先接管")
		}
		s.takeover("用户快捷键接管；不能据此确认远端命令结束")
		if doubleEscape {
			return nil
		}
	}
	if s.integrated {
		s.integrated = false
		s.problem = "人工输入后需重新确认空闲 Shell"
	}
	if s.audit != nil {
		if err := s.event("human_input", []byte(s.redactor.text(string(data))), nil); err != nil {
			// A recording failure disables automation, but must preserve manual access.
			s.problem = err.Error()
		}
	}
	return write()
}
func (s *AgentSession) Takeover(ctx context.Context, interrupt bool) error {
	s.mu.Lock()
	s.takeover("人工接管；连接保留，远端命令可能仍在运行")
	if !interrupt || !s.connected || s.peer == nil {
		s.mu.Unlock()
		return nil
	}
	s.ticket = "interrupt:" + NewID()
	ticket := s.ticket
	peer := s.peer
	if err := s.event("interrupt_request", nil, nil); err != nil {
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()
	return peer.AgentSend(ctx, []byte{3}, ticket)
}
func (s *AgentSession) SensitiveInput(ctx context.Context, value string) error {
	if value == "" || len(value) > 4096 || strings.ContainsAny(value, "\x00\r\n") {
		return fmt.Errorf("敏感输入须为不超过 4096 字节的单行")
	}
	s.mu.Lock()
	if !s.connected || s.peer == nil || s.audit == nil || len(s.redactor.values) >= 8 {
		s.mu.Unlock()
		return fmt.Errorf("请先授权并开启存档，每连接最多声明 8 个敏感值")
	}
	s.takeover("敏感输入已接管；完成后请确认恢复观察")
	s.redactor.values = append(s.redactor.values, []byte(value))
	s.displayRedactor.values = append(s.displayRedactor.values, []byte(value))
	s.sensitive = true
	s.ticket = "sensitive:" + NewID()
	ticket := s.ticket
	peer := s.peer
	if err := s.event("sensitive_input", nil, nil); err != nil {
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()
	data := append([]byte(value), '\r')
	defer clear(data)
	return peer.AgentSend(ctx, data, ticket)
}
func (s *AgentSession) ResumeObservation() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sensitive = false
	_ = s.event("sensitive_finished", nil, nil)
}

func (s *AgentSession) Observe(data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.audit == nil {
		return
	}
	size := s.size.Load()
	cols, rows := int(size>>32), int(uint32(size))
	if cols > 0 && rows > 0 && (cols != s.lastCols || rows != s.lastRows) {
		s.lastCols, s.lastRows = cols, rows
		_ = s.event("resize", nil, map[string]string{"cols": strconv.Itoa(cols), "rows": strconv.Itoa(rows)})
	}
	// Persist synchronously on the PTY reader: no unbounded audit queue; slow disk
	// naturally backpressures the existing bounded connection output buffer.
	s.output(data)
	s.marker = append(s.marker, data...)
	for len(s.marker) > 0 {
		start := bytes.Index(s.marker, []byte("\x1b]779;OSHELL;"))
		if start < 0 {
			keep := 0
			prefix := []byte("\x1b]779;OSHELL;")
			for n := 1; n < len(prefix) && n <= len(s.marker); n++ {
				if bytes.Equal(s.marker[len(s.marker)-n:], prefix[:n]) {
					keep = n
				}
			}

			s.marker = append([]byte(nil), s.marker[len(s.marker)-keep:]...)
			break
		}

		s.marker = s.marker[start:]
		end := bytes.IndexByte(s.marker, 7)
		if end < 0 {
			if len(s.marker) > 512 {

				s.marker = s.marker[1:]
				continue
			}
			break
		}
		fields := strings.Split(string(s.marker[len("\x1b]779;OSHELL;"):end]), ";")
		if len(fields) >= 2 && fields[0] == s.token && s.token != "" {
			s.mark(fields[1:])
		}
		s.marker = s.marker[end+1:]
	}
}
func (s *AgentSession) ObserveDisplay(data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.audit == nil {
		return
	}
	for len(data) > 0 {
		n := min(len(data), 16384)
		b := s.displayRedactor.consume(data[:n], false)
		data = data[n:]
		if len(b) > 0 {
			if s.event("display", b, nil) != nil {
				return
			}
		}
	}
}
func (s *AgentSession) output(data []byte) {
	for len(data) > 0 {
		n := min(len(data), 16384)
		b := s.redactor.consume(data[:n], false)
		data = data[n:]
		if len(b) > 0 {
			if err := s.event("output", b, nil); err != nil {
				return
			}
		}
	}
}
func (s *AgentSession) mark(fields []string) {
	switch fields[0] {
	case "ready":
		if !s.integrationPending {
			return
		}
		s.integrationPending = false
		if s.timer != nil {
			s.timer.Stop()
			s.timer = nil
		}
		s.integrated = true
		s.problem = ""
		s.owner.Store(s.task != "")
		_ = s.event("shell_ready", nil, nil)
	case "unsupported":
		s.takeover("当前 Shell 不支持临时集成；请使用 Bash/Zsh")
	case "start":
		if len(fields) == 2 && s.current != nil && fields[1] == s.current.ID && (s.current.State == "dispatched" || s.current.State == "queued") {
			s.current.State = "running"
			s.startTimer(30*time.Minute, "命令未确认结束，已转人工处理")
			_ = s.event("command_start", nil, nil)
		}
	case "end":
		if len(fields) == 3 && s.current != nil && fields[1] == s.current.ID {
			code, err := strconv.Atoi(fields[2])
			if err != nil {
				return
			}
			if b := s.redactor.consume(nil, true); len(b) > 0 {
				_ = s.event("output", b, nil)
			}
			s.current.State = "completed"
			s.current.ExitCode = &code
			if s.timer != nil {
				s.timer.Stop()
				s.timer = nil
			}
			_ = s.event("command_end", nil, map[string]string{"exit_code": fields[2]})
		}
	}
}
func (s *AgentSession) NoteResize(cols, rows int) {
	s.size.Store(uint64(uint32(cols))<<32 | uint64(uint32(rows)))
}
func (s *AgentSession) Resize(cols, rows int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.audit != nil {
		_ = s.event("resize", nil, map[string]string{"cols": strconv.Itoa(cols), "rows": strconv.Itoa(rows)})
	}
}

func (c *AgentCoordinator) Call(ctx context.Context, client, tool string, args json.RawMessage) (any, error) {
	if err := ValidateMCPTool(tool, args); err != nil {
		return nil, err
	}
	var p struct {
		Session    string `json:"session_id"`
		Generation string `json:"generation"`
		Task       string `json:"task_id"`
		Command    string `json:"command"`
		Key        string `json:"idempotency_key"`
		Operation  string `json:"operation_id"`
		Cursor     int64  `json:"cursor"`
		Limit      int    `json:"limit"`
		Interrupt  bool   `json:"interrupt"`
	}
	_ = json.Unmarshal(args, &p)
	if tool == "sessions_list" {
		out := []any{}
		for _, s := range c.Sessions() {
			v := s.State()
			if v.Authorized && v.Connected && v.Client == client {
				out = append(out, map[string]any{"session_id": v.ID, "generation": v.Generation, "title": v.Title, "permission": v.Permission})
			}
		}
		return out, nil
	}
	var session *AgentSession
	for _, s := range c.Sessions() {
		s.mu.Lock()
		match := p.Session != "" && s.id == p.Session || p.Task != "" && s.task == p.Task || p.Operation != "" && s.operations[p.Operation] != nil
		s.mu.Unlock()
		if match {
			session = s
			break
		}
	}
	if session == nil {
		return nil, fmt.Errorf("会话、任务或操作不存在，请在 MCP 控制中心检查")
	}
	s := session
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.authorized || !s.connected || s.client != client || s.audit == nil {
		return nil, fmt.Errorf("会话未授权、已撤销或连接已变化")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if p.Task != "" && s.taskConnection != mcpConnectionID(ctx) {
		return nil, fmt.Errorf("任务属于另一个客户端连接")
	}
	switch tool {
	case "session_state":
		_, text, _, _, _ := s.peer.AgentSnapshot()
		text = s.redactor.text(text)
		if len(text) > 16384 {
			text = text[len(text)-16384:]
			for len(text) > 0 && !utf8.RuneStart(text[0]) {
				text = text[1:]
			}
		}
		if s.sensitive {
			text = ""
		}
		return map[string]any{"generation": s.generation, "occupied": s.task != "", "shell_ready": s.integrated, "screen": text, "sensitive_input_waiting_human": s.sensitive, "output_trust": "untrusted_terminal_data", "audit_error": s.problem}, nil
	case "task_start":
		if p.Generation != s.generation || s.task != "" || s.audit.Err() != nil || len(s.operations) >= 1024 {
			return nil, fmt.Errorf("连接变化、已有任务、审计不可用或达到 1024 操作上限")
		}
		if c.taskCount.Add(1) > 1024 {
			c.taskCount.Add(-1)
			return nil, fmt.Errorf("本次运行已达到 1024 任务上限")
		}
		id := NewID()
		if err := s.audit.Task(id); err != nil {
			c.taskCount.Add(-1)
			return nil, err
		}
		s.task = id
		s.taskConnection = mcpConnectionID(ctx)
		s.owner.Store(true)
		return map[string]any{"task_id": id, "generation": s.generation, "waiting_for_shell_integration": !s.integrated}, nil
	case "task_end":
		if s.current != nil && (s.current.State == "pending_approval" || s.current.State == "queued" || s.current.State == "dispatched" || s.current.State == "running") {
			return nil, fmt.Errorf("当前操作尚未结束，请使用 task_stop；不能据此标记成功")
		}
		s.task = ""
		s.owner.Store(false)
		_ = s.event("task_end", nil, nil)
		return map[string]any{"state": "released", "connection_preserved": true}, nil
	case "task_stop":
		s.mu.Unlock()
		err := s.Takeover(ctx, p.Interrupt)
		s.mu.Lock()
		return map[string]any{"state": "released", "connection_preserved": true}, err
	case "command_submit":
		return s.submit(ctx, client, p.Command, p.Key)
	case "operation_read":
		return s.readOperation(ctx, p.Operation, p.Cursor, p.Limit)
	}
	return nil, fmt.Errorf("未知工具")
}
func (s *AgentSession) submit(ctx context.Context, client, command, key string) (any, error) {
	s.receiptMu.Lock()
	defer s.receiptMu.Unlock()
	if len(key) > 256 {
		return nil, fmt.Errorf("幂等键超过 256 字节")
	}
	digest := sha256.Sum256([]byte(command))
	hash := hex.EncodeToString(digest[:])
	for _, op := range s.operations {
		if op.Key == key && op.Client == client {
			if op.Digest != hash {
				return nil, fmt.Errorf("幂等键已用于不同命令")
			}
			return *op, nil
		}
	}
	if s.current != nil && s.current.State != "completed" && s.current.State != "cancelled_before_send" {
		return nil, fmt.Errorf("已有未完成操作；请人工确认后重新集成 Shell")
	}
	if !s.integrated || s.sensitive || s.audit.Err() != nil {
		return nil, fmt.Errorf("请完成 Shell 集成、敏感输入确认和审计检查")
	}
	decision, err := AgentCommandDecision(command, s.permission)
	if err != nil {
		_ = s.event("request_rejected", nil, map[string]string{"command": s.redactor.text(command), "reason": err.Error()})
		if decision == "human" {
			s.takeover(err.Error())
		}
		return nil, err
	}
	if len(s.operations) >= 1024 {
		return nil, fmt.Errorf("本连接操作达到上限")
	}
	path := filepath.Join(s.root, "agent", agentReceiptName(client, key))
	privateKey, err := loadLocalVaultKey(filepath.Join(s.root, "agent"))
	if err != nil {
		return nil, err
	}
	defer clear(privateKey)
	old, err := readAgentFile(path, 8192)
	if err == nil {
		plain, e := unseal(privateKey, old, "OShell.Desktop.MCP.receipt.v1")
		if e != nil {
			return nil, fmt.Errorf("旧收据无法认证，拒绝执行")
		}
		defer clear(plain)
		var receipt struct{ ID, Digest, Generation string }
		if json.Unmarshal(plain, &receipt) != nil || !validAgentID(receipt.ID) {
			return nil, fmt.Errorf("旧收据无效，拒绝执行")
		}
		if receipt.Digest != hash {
			return nil, fmt.Errorf("幂等键已用于不同命令")
		}
		return map[string]string{"operation_id": receipt.ID, "state": "unknown_prior_intent", "message": "已有持久化收据，禁止自动重试；请检查存档"}, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	op := &AgentOperation{ID: NewID(), State: "pending_approval", Command: command, Task: s.task, Client: client, Key: key, Digest: hash, Revision: s.revision, Generation: s.generation}
	receipt, _ := json.Marshal(map[string]string{"ID": op.ID, "Digest": hash, "Generation": s.generation})
	sealed, err := seal(privateKey, receipt, "OShell.Desktop.MCP.receipt.v1")
	if err != nil {
		return nil, err
	}
	if err = AtomicWrite(path, sealed); err != nil {
		return nil, err
	}
	s.operations[op.ID] = op
	s.current = op
	if err = s.event("request", []byte(s.redactor.text(command)), map[string]string{"task": s.task, "key": key}); err != nil {
		op.State = "audit_failed"
		return nil, err
	}
	if decision == "automatic" {
		if err = s.execute(op, "policy"); err != nil {
			return nil, err
		}
	}
	return *op, nil
}
func (s *AgentSession) execute(op *AgentOperation, authority string) error {
	if !s.authorized || !s.connected || !s.integrated || s.sensitive || s.current != op || s.task != op.Task || s.revision != op.Revision || s.generation != op.Generation {
		return fmt.Errorf("审批快照已失效，请重新开始任务")
	}
	_, _, _, _, eligible := s.peer.AgentSnapshot()
	if !eligible {
		s.takeover("终端进入交互工具或传输状态")
		return fmt.Errorf("当前终端不能自动输入")
	}
	if err := s.event("approval", nil, map[string]string{"authority": authority, "permission": s.permission}); err != nil {
		return err
	}
	op.State = "queued"
	s.ticket = NewID()
	ticket := s.ticket
	peer := s.peer
	data := agentExecutionCommand(s.token, op.ID, op.Command)
	if err := s.event("execution_intent", []byte(s.redactor.text(string(data))), nil); err != nil {
		return err
	}
	s.startTimer(30*time.Second, "30 秒内未确认命令开始，禁止自动重发")
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := peer.AgentSend(ctx, data, ticket)
		s.mu.Lock()
		defer s.mu.Unlock()
		if op.State == "queued" {
			if err != nil {
				op.State = "unknown_write_result"
				s.takeover("命令写入结果未知；请人工检查")
			} else {
				op.State = "dispatched"
			}
		}
	}()
	return nil
}
func (s *AgentSession) Approve(id, command string, approve bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	op := s.current
	if op == nil || op.ID != id || op.Command != command || op.State != "pending_approval" {
		return fmt.Errorf("待审批命令已经变化")
	}
	if !approve {
		op.State = "cancelled_before_send"
		return s.event("approval_rejected", nil, nil)
	}
	return s.execute(op, "user")
}
func (s *AgentSession) readOperation(ctx context.Context, id string, cursor int64, limit int) (any, error) {
	if s.sensitive {
		return nil, fmt.Errorf("敏感输入期间暂不提供终端内容")
	}
	op := s.operations[id]
	if op == nil || op.Client != s.client || op.Generation != s.generation {
		return nil, fmt.Errorf("操作不可访问")
	}
	if limit == 0 {
		limit = 16384
	}
	if cursor < 0 || limit < 1 || limit > 65536 {
		return nil, fmt.Errorf("游标或读取上限无效")
	}
	position := int64(0)
	output := make([]byte, 0, limit)
	stop := errors.New("page complete")
	err := ReadAgentAudit(ctx, s.root, s.audit.ID(), func(e AgentAuditEvent) error {
		if e.Kind != "output" || e.Operation != id {
			return nil
		}
		end := position + int64(len(e.Bytes))
		if end > cursor {
			start := max(int64(0), cursor-position)
			n := min(limit-len(output), len(e.Bytes)-int(start))
			output = append(output, e.Bytes[start:int(start)+n]...)
		}
		position = end
		if len(output) == limit {
			return stop
		}
		return nil
	})
	if err != nil && !errors.Is(err, ErrAgentAuditUnsealed) && err != stop {
		s.takeover("审计读取失败")
		return nil, err
	}
	if cursor > position {
		return nil, fmt.Errorf("游标超出已持久化输出")
	}
	var text any
	if len(output) <= 16384 && utf8.Valid(output) {
		text = string(output)
	}
	return map[string]any{"operation": *op, "output_base64": base64.StdEncoding.EncodeToString(output), "output_text": text, "stream": "merged_pty", "next_cursor": cursor + int64(len(output)), "output_trust": "untrusted_terminal_data"}, nil
}

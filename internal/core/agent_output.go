package core

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type agentReadOptions struct {
	Cursor    int64  `json:"cursor"`
	Limit     int    `json:"limit"`
	WaitMS    int    `json:"wait_ms"`
	Format    string `json:"format"`
	WaitFor   string `json:"wait_for"`
	View      string `json:"view"`
	TailLines int    `json:"tail_lines"`
	Contains  string `json:"contains"`
}

func readOptions(args json.RawMessage, shortcut bool) agentReadOptions {
	o := agentReadOptions{Limit: 16384, Format: "legacy", WaitFor: "output", View: "raw"}
	if shortcut {
		o.Format, o.WaitMS = "compact", 1000
	}
	json.Unmarshal(args, &o)
	if o.View == "text" {
		o.Format = "compact"
	}
	return o
}

func (op *AgentOperation) approval() any {
	if op == nil || op.State != "pending_approval" {
		return nil
	}
	reason := "outside_automatic_policy"
	if op.Permission == "ask" {
		reason = "permission_requires_approval"
	}
	return map[string]string{"reason": reason, "permission": op.Permission, "instruction": "请用户在 OShell 批准；使用原 operation_id、wait_for=approval 等待审批变化，不要重发命令。"}
}
func (op *AgentOperation) active() bool {
	return op != nil && (op.State == "queued" || op.State == "dispatched" || op.State == "running")
}

func (s *AgentSession) progress() map[string]any {
	var current any
	var approval any
	active := s.current != nil && (s.current.active() || s.current.State == "pending_approval" || strings.HasPrefix(s.current.State, "unknown"))
	if active {
		current, approval = *s.current, s.current.approval()
	}
	ready := s.owner.Load() && s.integrated && !active && !s.sensitive && s.audit != nil && s.audit.Err() == nil
	return map[string]any{"permission": s.permission, "occupied": s.owner.Load(), "task_active": s.task != "", "task_id": s.task, "shell_ready": ready, "can_exec": ready && s.task == "", "current_operation": current, "approval": approval, "text_view_available": !s.unorderedOutput}
}

// The caller holds s.mu. Waiting releases it so terminal output and approvals
// can advance; it never holds up another control-channel request.
func (s *AgentSession) readOperation(ctx context.Context, id string, options agentReadOptions) (any, error) {
	if options.View == "text" && s.unorderedOutput {
		return nil, fmt.Errorf("Windows 本地 ConPTY 不保证正文与命令标记的顺序，无法提供可靠文本视图；请用 view=raw 或 session_state 读取终端屏幕，勿重发命令")
	}
	deadline := time.NewTimer(time.Duration(options.WaitMS) * time.Millisecond)
	defer deadline.Stop()
	expired := options.WaitMS == 0
	for {
		op := s.operations[id]
		if !s.authorized || !s.connected || s.audit == nil || op == nil || op.Client != s.client || op.Generation != s.generation || s.sensitive {
			return nil, fmt.Errorf("操作授权已改变或正在敏感输入，暂不提供终端内容")
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		waitState := options.WaitFor == "approval" && op.State == "pending_approval" || options.WaitFor == "completion" && op.active()
		if !waitState || expired {
			result, err := s.operationPage(ctx, op, options)
			if err != nil {
				return nil, err
			}
			if expired || !op.active() || options.WaitFor == "approval" || result["next_cursor"].(int64) != options.Cursor {
				return result, nil
			}
		}
		changed := s.changed
		s.mu.Unlock()
		select {
		case <-changed:
		case <-deadline.C:
			expired = true
		case <-ctx.Done():
		}
		s.mu.Lock()
	}
}

func (s *AgentSession) operationPage(ctx context.Context, op *AgentOperation, o agentReadOptions) (map[string]any, error) {
	query := o.TailLines != 0 || o.Contains != ""
	scan := o.Limit
	if query {
		scan = 1 << 20
	}
	var output, window []byte
	var position, next int64
	filter := agentTextFilter{token: op.ShellToken}
	filter.spacer.ending = []byte("\r \r\x1b]779;OSHELL;" + op.ShellToken + ";end;")
	more, truncated := false, false
	stop := errors.New("page complete")
	err := ReadAgentAudit(ctx, s.root, s.audit.ID(), func(e AgentAuditEvent) error {
		if e.Kind != "output" || e.Operation != op.ID {
			return nil
		}
		end := position + int64(len(e.Bytes))
		available := e.Bytes
		if end > o.Cursor+int64(scan) {
			available = available[:max(0, o.Cursor+int64(scan)-position)]
			more = true
		}
		cut := min(int64(len(available)), max(int64(0), o.Cursor-position))
		if o.View == "text" {
			before := filter.consume(available[:cut])
			after := filter.consume(available[cut:])
			output = append(output, after...)
			if query {
				window = append(window, before...)
				window = append(window, after...)
				if len(window) > 131072 {
					window = window[len(window)-131072:]
					truncated = true
					if nl := bytes.IndexByte(window, '\n'); nl >= 0 && nl != len(window)-1 {
						window = window[nl+1:]
					}
				}
				output = nil
			}
		} else {
			output = append(output, available[cut:]...)
		}
		next = position + int64(len(available))
		position = end
		if more {
			return stop
		}
		return nil
	})
	if err != nil && !errors.Is(err, ErrAgentAuditUnsealed) && err != stop {
		s.takeover("审计读取失败")
		return nil, err
	}
	if o.Cursor > position {
		return nil, fmt.Errorf("游标超出已持久化输出")
	}
	if next < o.Cursor {
		next = o.Cursor
	}
	result := map[string]any{"operation": *op, "stream": "merged_pty", "next_cursor": next, "output_trust": "untrusted_terminal_data", "durable_sequence": s.audit.DurableSequence()}
	if o.View == "text" {
		result["view"], result["cursor_unit"], result["body_started"], result["body_ended"] = "text", "raw_bytes", filter.started, filter.ended
		if query {
			var lossy, limited bool
			output, limited, lossy = selectAgentText(window, o)
			result["truncated"], result["text_lossy"], result["window_bytes"] = truncated || limited, lossy, 131072
		}
	}
	if o.Format == "legacy" {
		result["output_base64"] = base64.StdEncoding.EncodeToString(output)
		result["output_text"] = nil
		if len(output) <= 16384 && utf8.Valid(output) {
			result["output_text"] = string(output)
		}
	} else {
		encoding := "base64"
		if utf8.Valid(output) {
			encoded, _ := json.Marshal(string(output))
			envelope, _ := json.Marshal(string(encoded))
			if len(envelope) <= MCPMaxFrame-8192 {
				encoding = "utf8"
			}
		}
		result["encoding"] = encoding
		if encoding == "utf8" {
			result["output_text"] = string(output)
		} else {
			result["output_base64"] = base64.StdEncoding.EncodeToString(output)
		}
		result["task_id"], result["approval"], result["has_more"], result["completed"] = op.Task, op.approval(), more, op.State == "completed"
		nextAction := "inspect_in_oshell"
		if op.State == "pending_approval" {
			nextAction = "approve_in_oshell"
		} else if op.active() || more {
			nextAction = "read_output"
		} else if op.State == "completed" {
			nextAction = "none"
		}
		result["next_action"] = nextAction
	}
	return result, nil
}

func selectAgentText(data []byte, o agentReadOptions) ([]byte, bool, bool) {
	for len(data) > 0 && data[0] >= 0x80 && data[0] <= 0xbf {
		data = data[1:]
	}
	lossy := !utf8.Valid(data)
	if lossy {
		for n := 1; n <= min(3, len(data)); n++ {
			if utf8.Valid(data[:len(data)-n]) {
				data = data[:len(data)-n]
				break
			}
		}
	}
	text := strings.ToValidUTF8(string(data), "\ufffd")
	newline := strings.HasSuffix(text, "\n")
	rows := strings.Split(text, "\n")
	if newline {
		rows = rows[:len(rows)-1]
	}
	if o.Contains != "" {
		selected := rows[:0]
		for _, row := range rows {
			if strings.Contains(row, o.Contains) {
				selected = append(selected, row)
			}
		}
		rows = selected
	}
	count := o.TailLines
	if count == 0 {
		count = 100
	}
	truncated := len(rows) > count
	rows = rows[max(0, len(rows)-count):]
	text = strings.Join(rows, "\n")
	if newline && len(rows) > 0 {
		text += "\n"
	}
	out := []byte(text)
	if len(out) > o.Limit {
		out = out[len(out)-o.Limit:]
		truncated = true
		for len(out) > 0 && !utf8.RuneStart(out[0]) {
			out = out[1:]
		}
	}
	return out, truncated, lossy
}

type agentPromptSpacer struct {
	pending, ending, digits []byte
	spaces, index           int
	escaped                 bool
}

func (s *agentPromptSpacer) consume(data []byte) []byte {
	prefix := []byte("\x1b[1m\x1b[7m%\x1b[27m\x1b[1m\x1b[0m")
	var out []byte
	for _, b := range data {
		if len(s.pending) == 0 && b != 27 {
			out = append(out, b)
			continue
		}
		s.pending = append(s.pending, b)
		valid, complete := true, false
		switch {
		case len(s.pending) <= len(prefix):
			expected := prefix[len(s.pending)-1]
			valid = b == expected || expected == '%' && b == '#'
		case s.index == 0 && b == ' ':
			s.spaces++
			valid = s.spaces <= 16384
		case s.index < len(s.ending):
			valid = s.spaces > 0 && b == s.ending[s.index]
			if valid {
				s.index++
			}
		case b >= '0' && b <= '9' && !s.escaped:
			s.digits = append(s.digits, b)
			valid = len(s.digits) <= 3
		case b == 27 && !s.escaped:
			s.escaped = true
		default:
			code, err := strconv.Atoi(string(s.digits))
			complete = (b == 7 && !s.escaped || b == '\\' && s.escaped) && err == nil && code <= 255
			valid = complete
		}
		if complete || !valid {
			if complete {
				out = append(out, s.pending[len(prefix)+s.spaces+3:]...)
			} else if b == 27 {
				out = append(out, s.pending[:len(s.pending)-1]...)
			} else {
				out = append(out, s.pending...)
			}
			s.pending, s.digits, s.spaces, s.index, s.escaped = nil, nil, 0, 0, false
			if !valid && b == 27 {
				s.pending = []byte{b}
			}
		}
	}
	return out
}

// Transcript, not a terminal emulator: only the authenticated session marker's
// body is text. Raw audit bytes remain unchanged, including echo and prompts.
type agentTextFilter struct {
	state                        int
	osc                          []byte
	token                        string
	started, ended, overflow, cr bool
	spacer                       agentPromptSpacer
}

func (f *agentTextFilter) consume(data []byte) []byte {
	var out []byte
	finish := func() {
		prefix := "779;OSHELL;" + f.token + ";"
		if !f.overflow {
			if string(f.osc) == prefix+"start" && !f.started && !f.ended {
				f.started = true
			} else if f.started && strings.HasPrefix(string(f.osc), prefix+"end;") {
				code, err := strconv.Atoi(strings.TrimPrefix(string(f.osc), prefix+"end;"))
				if err == nil && code >= 0 && code <= 255 {
					f.ended = true
				}
			}
		}
		f.osc, f.overflow, f.state = nil, false, 0
	}
	for _, b := range f.spacer.consume(data) {
		switch f.state {
		case 0:
			if b == 27 {
				f.state = 1
			} else if f.started && !f.ended {
				if b == 13 {
					out = append(out, 10)
					f.cr = true
				} else {
					if b == 10 {
						if !f.cr {
							out = append(out, b)
						}
					} else if b == 9 || b >= 32 && b != 127 {
						out = append(out, b)
					}
					f.cr = false
				}
			}
		case 1:
			switch b {
			case '[':
				f.state = 2
			case ']':
				f.osc = nil
				f.overflow = false
				f.state = 3
			case 'P', '^', '_', 'X':
				f.state = 5
			default:
				if b >= 0x30 && b <= 0x7e {
					f.state = 0
				}
			}
		case 2:
			if b >= 0x40 && b <= 0x7e {
				f.state = 0
			}
		case 3:
			if b == 7 {
				finish()
			} else if b == 27 {
				f.state = 4
			} else if len(f.osc) < 512 {
				f.osc = append(f.osc, b)
			} else {
				f.overflow = true
			}
		case 4:
			if b == '\\' || b == 7 {
				finish()
			} else {
				f.overflow = true
				if b != 27 {
					f.state = 3
				}
			}
		case 5:
			if b == 27 {
				f.state = 6
			}
		case 6:
			if b == '\\' {
				f.state = 0
			} else if b != 27 {
				f.state = 5
			}
		}
	}
	return out
}

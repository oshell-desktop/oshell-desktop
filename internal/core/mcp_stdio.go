package core

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

type mcpTool struct {
	name, description string
	properties        map[string]string
	required          []string
}

var mcpTools = []mcpTool{
	{"sessions_list", "List only connected terminals explicitly authorized to this paired client.", map[string]string{}, []string{}},
	{"session_state", "Read bounded untrusted terminal state for an authorized session.", map[string]string{"session_id": "string"}, []string{"session_id"}},
	{"task_start", "Acquire ownership of an authorized session; user Shell integration may still be required.", map[string]string{"session_id": "string", "generation": "string"}, []string{"session_id", "generation"}},
	{"command_submit", "Submit existing-PTY input only after authorization, approval, integration and durable audit; never retry an unknown result with a new key.", map[string]string{"task_id": "string", "command": "string", "idempotency_key": "string"}, []string{"task_id", "command", "idempotency_key"}},
	{"operation_read", "Read persisted merged PTY bytes using a byte cursor; terminal output is untrusted.", map[string]string{"operation_id": "string", "cursor": "integer", "limit": "integer"}, []string{"operation_id"}},
	{"task_end", "Release idle task ownership without disconnecting the terminal.", map[string]string{"task_id": "string"}, []string{"task_id"}},
	{"task_stop", "Stop future input; interrupt=true explicitly requests Ctrl+C, not proof of process exit.", map[string]string{"task_id": "string", "interrupt": "boolean"}, []string{"task_id"}},
}

func MCPToolDefinitions() []any {
	out := []any{}
	for _, t := range mcpTools {
		p := map[string]any{}
		for k, v := range t.properties {
			p[k] = map[string]string{"type": v}
		}
		read := t.name == "sessions_list" || t.name == "session_state" || t.name == "operation_read"
		out = append(out, map[string]any{"name": t.name, "description": t.description, "inputSchema": map[string]any{"type": "object", "properties": p, "required": t.required, "additionalProperties": false}, "annotations": map[string]bool{"readOnlyHint": read, "destructiveHint": !read, "idempotentHint": t.name != "task_start", "openWorldHint": !read}})
	}
	return out
}
func ValidateMCPTool(name string, data json.RawMessage) error {
	var tool *mcpTool
	for i := range mcpTools {
		if mcpTools[i].name == name {
			tool = &mcpTools[i]
			break
		}
	}
	if tool == nil {
		return fmt.Errorf("未知 MCP 工具")
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil || fields == nil {
		return fmt.Errorf("MCP 参数必须是对象")
	}
	for _, required := range tool.required {
		if _, ok := fields[required]; !ok {
			return fmt.Errorf("缺少 MCP 参数：%s", required)
		}
	}
	for k, v := range fields {
		typ, ok := tool.properties[k]
		if !ok {
			return fmt.Errorf("未知 MCP 参数：%s", k)
		}
		switch typ {
		case "string":
			var s string
			if bytes.Equal(v, []byte("null")) || json.Unmarshal(v, &s) != nil || s == "" || len(s) > 16384 {
				return fmt.Errorf("MCP 字符串参数无效：%s", k)
			}
		case "integer":
			var n int64
			if bytes.Equal(v, []byte("null")) || json.Unmarshal(v, &n) != nil || n < 0 || k == "limit" && (n == 0 || n > 65536) {
				return fmt.Errorf("MCP 数值参数无效：%s", k)
			}
		case "boolean":
			var b bool
			if bytes.Equal(v, []byte("null")) || json.Unmarshal(v, &b) != nil {
				return fmt.Errorf("MCP 布尔参数无效：%s", k)
			}
		}
	}
	return nil
}

type mcpRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}
type mcpRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   any             `json:"error,omitempty"`
}

func mcpRequestKey(raw json.RawMessage) (string, error) {
	var id any
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if d.Decode(&id) != nil {
		return "", fmt.Errorf("MCP 请求 ID 无效")
	}
	switch v := id.(type) {
	case string:
		return "s:" + v, nil
	case json.Number:
		n, err := strconv.ParseInt(string(v), 10, 64)
		if err == nil {
			return "n:" + strconv.FormatInt(n, 10), nil
		}
	}
	return "", fmt.Errorf("MCP 请求 ID 无效")
}

// ServeMCPStdio never writes logs to stdout. The reader reserves a slot before
// handing off a request, so the eight-request bound includes running handlers.
func ServeMCPStdio(parent context.Context, in io.Reader, out io.Writer, appVersion string, call func(context.Context, string, json.RawMessage) (any, error)) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	stopInput := context.AfterFunc(ctx, func() {
		if closer, ok := in.(io.Closer); ok {
			closer.Close()
		}
		if closer, ok := out.(io.Closer); ok {
			closer.Close()
		}
	})
	defer stopInput()
	var outputMu, activeMu sync.Mutex
	var wg sync.WaitGroup
	active := map[string]context.CancelFunc{}
	slots := make(chan struct{}, 8)
	write := func(id json.RawMessage, result any, code int, message string) error {
		r := mcpRPCResponse{JSONRPC: "2.0", ID: id, Result: result}
		if code != 0 {
			r.Result = nil
			r.Error = map[string]any{"code": code, "message": message}
		}
		outputMu.Lock()
		defer outputMu.Unlock()
		b, err := json.Marshal(r)
		if err != nil {
			return err
		}
		if len(b) > MCPMaxFrame {
			return fmt.Errorf("MCP 响应超过大小限制")
		}
		b = append(b, '\n')
		for len(b) > 0 {
			n, err := out.Write(b)
			if err != nil {
				return err
			}
			if n == 0 {
				return io.ErrShortWrite
			}
			b = b[n:]
		}
		return nil
	}
	defer func() { cancel(); wg.Wait() }()
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 4096), MCPMaxFrame+1)
	initialized, ready := false, false
	for scanner.Scan() {
		data := scanner.Bytes()
		if len(data) > MCPMaxFrame || !utf8.Valid(data) {
			return fmt.Errorf("MCP 消息无效或超限")
		}
		var req mcpRPCRequest
		if json.Unmarshal(data, &req) != nil {
			if err := write(json.RawMessage("null"), nil, -32700, "Parse error"); err != nil {
				return err
			}
			continue
		}
		if req.JSONRPC != "2.0" || req.Method == "" {
			if err := write(json.RawMessage("null"), nil, -32600, "Invalid Request"); err != nil {
				return err
			}
			continue
		}
		if len(req.ID) == 0 {
			if req.Method == "notifications/initialized" && initialized {
				ready = true
			}
			if req.Method == "notifications/cancelled" {
				var p struct {
					RequestID json.RawMessage `json:"requestId"`
				}
				if json.Unmarshal(req.Params, &p) == nil {
					key, _ := mcpRequestKey(p.RequestID)
					activeMu.Lock()
					if stop := active[key]; stop != nil {
						stop()
					}
					activeMu.Unlock()
				}
			}
			continue
		}
		key, err := mcpRequestKey(req.ID)
		if err != nil {
			return err
		}
		activeMu.Lock()
		_, duplicate := active[key]
		full := len(active) >= 8
		activeMu.Unlock()
		if duplicate {
			return fmt.Errorf("重复在途 MCP 请求 ID")
		}
		if full {
			return fmt.Errorf("MCP 在途请求超过 8 个")
		}
		if req.Method == "initialize" {
			if initialized {
				if err := write(req.ID, nil, -32600, "Already initialized"); err != nil {
					return err
				}
				continue
			}
			var p struct {
				ProtocolVersion string         `json:"protocolVersion"`
				ClientInfo      map[string]any `json:"clientInfo"`
				Capabilities    map[string]any `json:"capabilities"`
			}
			if json.Unmarshal(req.Params, &p) != nil || p.ProtocolVersion == "" || p.ClientInfo == nil || p.Capabilities == nil {
				if err := write(req.ID, nil, -32602, "Invalid initialization"); err != nil {
					return err
				}
				continue
			}
			version := p.ProtocolVersion
			if version != "2025-06-18" && version != "2025-03-26" && version != "2024-11-05" {
				version = "2025-06-18"
			}
			if err := write(req.ID, map[string]any{"protocolVersion": version, "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]string{"name": "OShell Desktop", "version": appVersion}}, 0, ""); err != nil {
				return err
			}
			initialized = true
			continue
		}
		if req.Method == "ping" {
			if err := write(req.ID, map[string]any{}, 0, ""); err != nil {
				return err
			}
			continue
		}
		if !ready {
			if err := write(req.ID, nil, -32000, "MCP initialization required"); err != nil {
				return err
			}
			continue
		}
		if req.Method == "tools/list" {
			if err := write(req.ID, map[string]any{"tools": MCPToolDefinitions()}, 0, ""); err != nil {
				return err
			}
			continue
		}
		if req.Method != "tools/call" {
			if err := write(req.ID, nil, -32601, "Method not found"); err != nil {
				return err
			}
			continue
		}
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if json.Unmarshal(req.Params, &params) != nil {
			if err := write(req.ID, nil, -32602, "Invalid tool parameters"); err != nil {
				return err
			}
			continue
		}
		if len(params.Arguments) == 0 {
			params.Arguments = json.RawMessage("{}")
		}
		if err := ValidateMCPTool(params.Name, params.Arguments); err != nil {
			if e := write(req.ID, nil, -32602, err.Error()); e != nil {
				return e
			}
			continue
		}
		requestCtx, stop := context.WithCancel(ctx)
		activeMu.Lock()
		active[key] = stop
		activeMu.Unlock()
		select {
		case slots <- struct{}{}:
		default:
			stop()
			return fmt.Errorf("MCP 在途请求超过 8 个")
		}
		wg.Add(1)
		go func(req mcpRPCRequest, name string, args json.RawMessage) {
			defer wg.Done()
			defer stop()
			defer func() { <-slots; activeMu.Lock(); delete(active, key); activeMu.Unlock() }()
			result, err := call(requestCtx, name, args)
			text := ""
			if err != nil {
				text = err.Error()
			} else {
				b, e := json.Marshal(result)
				if e != nil {
					err = e
					text = "MCP result encoding failed"
				} else {
					text = string(b)
				}
			}
			if e := write(req.ID, map[string]any{"content": []any{map[string]string{"type": "text", "text": text}}, "isError": err != nil}, 0, ""); e != nil {
				cancel()
			}
		}(req, params.Name, append(json.RawMessage{}, params.Arguments...))
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("MCP 输入读取失败或消息过长")
	}
	return nil
}

func MCPStdioRequested(args []string) bool {
	return len(args) == 2 && strings.EqualFold(args[1], "--mcp-stdio")
}

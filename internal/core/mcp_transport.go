package core

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"sync"
	"time"
)

const MCPMaxFrame = 262144

type MCPRequest struct {
	Credential string          `json:"credential"`
	Tool       string          `json:"tool"`
	Arguments  json.RawMessage `json:"arguments"`
}
type MCPResponse struct {
	Result any    `json:"result"`
	Error  string `json:"error,omitempty"`
}
type MCPHandler func(context.Context, string, string, json.RawMessage) (any, error)
type mcpConnectionContextKey struct{}

func mcpConnectionID(ctx context.Context) string {
	id, _ := ctx.Value(mcpConnectionContextKey{}).(string)
	return id
}

type MCPServer struct {
	disconnected func(string)
	listener     net.Listener
	release      func()
	ctx          context.Context
	cancel       context.CancelFunc
	once         sync.Once
	slots        chan struct{}
	wg           sync.WaitGroup
	clients      *MCPClients
	handler      MCPHandler
}

// Only the tested same-user transport primitives are shared with launch IPC.
// A distinct endpoint and authenticated request protocol cannot launch sessions.
func mcpEndpoint(root string) string { return filepath.Join(root, "agent", "mcp-control") }
func StartMCPServer(root string, clients *MCPClients, handler MCPHandler, disconnected ...func(string)) (*MCPServer, error) {
	if clients == nil || handler == nil {
		return nil, fmt.Errorf("MCP 服务尚未初始化")
	}
	l, release, err := listenLaunch(mcpEndpoint(root))
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &MCPServer{listener: l, release: release, ctx: ctx, cancel: cancel, slots: make(chan struct{}, 8), clients: clients, handler: handler}
	if len(disconnected) > 0 {
		s.disconnected = disconnected[0]
	}
	s.wg.Add(1)
	go s.accept()
	return s, nil
}
func (s *MCPServer) Close() error {
	var err error
	s.once.Do(func() {
		s.cancel()
		err = s.listener.Close()
		s.wg.Wait()
		if s.release != nil {
			s.release()
		}
	})
	return err
}
func (s *MCPServer) accept() {
	defer s.wg.Done()
	for {
		c, err := s.listener.Accept()
		if err != nil {
			return
		}
		select {
		case s.slots <- struct{}{}:
			s.wg.Add(1)
			go s.serve(c)
		default:
			c.Close()
		}
	}
}
func (s *MCPServer) serve(c net.Conn) {
	defer s.wg.Done()
	defer c.Close()
	defer func() { <-s.slots }()
	stop := context.AfterFunc(s.ctx, func() { c.Close() })
	defer stop()
	if verifyLaunchConnection(c, false) != nil {
		return
	}
	connectionID := NewID()
	if s.disconnected != nil {
		defer s.disconnected(connectionID)
	}
	connectionContext := context.WithValue(s.ctx, mcpConnectionContextKey{}, connectionID)
	for {
		c.SetReadDeadline(time.Now().Add(5 * time.Minute))
		var req MCPRequest
		if readMCPFrame(c, &req) != nil {
			return
		}
		id, err := s.clients.Authenticate(req.Credential)
		req.Credential = ""
		reply := MCPResponse{}
		if err == nil {
			if err = ValidateMCPTool(req.Tool, req.Arguments); err == nil {
				ctx, cancel := context.WithTimeout(connectionContext, 30*time.Second)
				reply.Result, err = s.handler(ctx, id, req.Tool, req.Arguments)
				cancel()
			}
		}
		if err != nil {
			reply.Error = err.Error()
		}
		c.SetWriteDeadline(time.Now().Add(30 * time.Second))
		if writeMCPFrame(c, reply) != nil {
			return
		}
	}
}
func readMCPFrame(r io.Reader, v any) error {
	var size [4]byte
	if _, err := io.ReadFull(r, size[:]); err != nil {
		return err
	}
	n := binary.BigEndian.Uint32(size[:])
	if n == 0 || n > MCPMaxFrame {
		return fmt.Errorf("MCP 消息长度无效")
	}
	b := make([]byte, n)
	defer clear(b)
	if _, err := io.ReadFull(r, b); err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
func writeMCPFrame(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	defer clear(b)
	if len(b) > MCPMaxFrame {
		return fmt.Errorf("MCP 响应过长")
	}
	packet := make([]byte, 4, len(b)+4)
	binary.BigEndian.PutUint32(packet, uint32(len(b)))
	packet = append(packet, b...)
	defer clear(packet)
	for len(packet) > 0 {
		n, err := w.Write(packet)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		packet = packet[n:]
	}
	return nil
}

type MCPBridge struct {
	root, token string
	conn        net.Conn
	mu          sync.Mutex
}

func NewMCPBridge(root, token string) *MCPBridge { return &MCPBridge{root: root, token: token} }
func (b *MCPBridge) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.conn != nil {
		err := b.conn.Close()
		b.conn = nil
		return err
	}
	return nil
}
func (b *MCPBridge) request(ctx context.Context, tool string, args json.RawMessage) (any, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.conn == nil {
		c, err := dialLaunch(ctx, mcpEndpoint(b.root))
		if err != nil {
			return nil, &MCPTransportError{message: "无法连接 OShell MCP 服务；请手动开启，后续请求会自动重连，无需重新配对"}
		}
		if err = verifyLaunchConnection(c, true); err != nil {
			c.Close()
			return nil, err
		}
		b.conn = c
	}
	c := b.conn
	stop := context.AfterFunc(ctx, func() { c.Close() })
	defer stop()
	deadline := time.Now().Add(30 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	c.SetDeadline(deadline)
	err := writeMCPFrame(c, MCPRequest{b.token, tool, args})
	var response MCPResponse
	if err == nil {
		err = readMCPFrame(c, &response)
	}
	if err != nil {
		c.Close()
		b.conn = nil
		var network net.Error
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, net.ErrClosed) || errors.As(err, &network) {
			return nil, &MCPTransportError{message: "MCP 通道中断，执行结果未知；不得用新幂等键重发命令"}
		}
		return nil, err
	}
	if response.Error != "" {
		return nil, fmt.Errorf("%s", response.Error)
	}
	return response.Result, nil
}

// Only transport failures can retry a read. Protocol, identity and application
// failures, and every execution request, retain their original result.
type MCPTransportError struct{ message string }

func (e *MCPTransportError) Error() string { return e.message }
func (b *MCPBridge) requestWithRetry(ctx context.Context, tool string, args json.RawMessage) (any, error) {
	result, err := b.request(ctx, tool, args)
	var failure *MCPTransportError
	readOnly := tool == "sessions_list" || tool == "session_state" || tool == "operation_read"
	if readOnly && errors.As(err, &failure) {
		timer := time.NewTimer(250 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
		}
		return b.request(ctx, tool, args)
	}
	return result, err
}
func (b *MCPBridge) Call(ctx context.Context, tool string, args json.RawMessage) (any, error) {
	if err := ValidateMCPTool(tool, args); err != nil {
		return nil, err
	}
	options := readOptions(args, tool == "session_exec")
	if tool == "operation_read" && options.WaitMS > 0 {
		read := NewMCPBridge(b.root, b.token)
		defer read.Close()
		return read.requestWithRetry(ctx, tool, args)
	}
	result, err := b.requestWithRetry(ctx, tool, args)
	if err != nil || tool != "session_exec" {
		return result, err
	}
	receipt, ok := result.(map[string]any)
	if !ok {
		return result, nil
	}
	id, ok := receipt["operation_id"].(string)
	if !ok {
		return result, nil
	}
	params := map[string]any{"operation_id": id, "format": "compact", "wait_for": "completion", "wait_ms": options.WaitMS, "limit": options.Limit, "view": options.View}
	if options.TailLines > 0 {
		params["tail_lines"] = options.TailLines
	}
	if options.Contains != "" {
		params["contains"] = options.Contains
	}
	data, _ := json.Marshal(params)
	read := NewMCPBridge(b.root, b.token)
	defer read.Close()
	output, err := read.requestWithRetry(ctx, "operation_read", data)
	if err != nil {
		receipt["read_error"], receipt["next_action"] = err.Error(), "read_output"
		return receipt, nil
	}
	return output, nil
}

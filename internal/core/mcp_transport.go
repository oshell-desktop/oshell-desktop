package core

import (
	"context"
	"encoding/binary"
	"encoding/json"
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
func (b *MCPBridge) Call(ctx context.Context, tool string, args json.RawMessage) (any, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.conn == nil {
		c, err := dialLaunch(ctx, mcpEndpoint(b.root))
		if err != nil {
			return nil, fmt.Errorf("请先在 OShell 中手动开启 MCP 服务")
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
		return nil, fmt.Errorf("MCP 通道中断，结果未知；未自动重试")
	}
	if response.Error != "" {
		return nil, fmt.Errorf("%s", response.Error)
	}
	return response.Result, nil
}

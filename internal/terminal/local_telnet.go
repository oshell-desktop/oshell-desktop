package terminal

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	tnSE   = 240
	tnSB   = 250
	tnWILL = 251
	tnWONT = 252
	tnDO   = 253
	tnDONT = 254
	tnIAC  = 255
)

// An interactive NVT client. The reader owns option negotiation. Writes from
// negotiation and from the keyboard share a lock so IAC sequences stay whole.
type localTelnet struct {
	mu         sync.Mutex
	conn       net.Conn
	cancel     context.CancelFunc
	closed     bool
	sendMu     sync.Mutex
	remoteEcho atomic.Bool
	output     io.Writer
}

func (s *localTelnet) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	s.cancel()
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}
func (s *localTelnet) Resize(int, int, int, int) error { return nil }

func (s *localTelnet) send(conn net.Conn, data []byte) error {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_, err := conn.Write(data)
	return err
}

func (s *localTelnet) Write(b []byte) (int, error) {
	s.mu.Lock()
	conn := s.conn
	s.mu.Unlock()
	if bytes.IndexByte(b, 0x1d) >= 0 || conn == nil && bytes.IndexByte(b, 3) >= 0 {
		s.Close()
		return len(b), nil
	}
	if conn == nil {
		return len(b), nil
	}
	var data []byte
	for i := 0; i < len(b); i++ {
		switch b[i] {
		case tnIAC:
			data = append(data, tnIAC, tnIAC)
		case '\r':
			data = append(data, '\r', '\n')
			if i+1 < len(b) && b[i+1] == '\n' {
				i++
			}
		default:
			data = append(data, b[i])
		}
	}
	if err := s.send(conn, data); err != nil {
		return 0, err
	}
	if !s.remoteEcho.Load() {
		_, _ = s.output.Write(bytes.ReplaceAll(b, []byte{'\r'}, []byte{'\r', '\n'}))
	}
	return len(b), nil
}

func (s *localTelnet) run(ctx context.Context, address string, out io.Writer) error {
	dialer := net.Dialer{Timeout: 15 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return err
	}
	defer conn.Close()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return context.Canceled
	}
	s.conn = conn
	s.mu.Unlock()
	fmt.Fprintf(out, "已连接 %s，Ctrl+] 断开。\r\n", address)
	return s.receive(conn, out)
}

func (s *localTelnet) receive(conn net.Conn, out io.Writer) error {
	r := bufio.NewReader(conn)
	var remote [256]bool
	var localSGA bool
	var afterCR bool
	var text bytes.Buffer
	for {
		if r.Buffered() == 0 || text.Len() >= 4096 {
			if _, err := text.WriteTo(out); err != nil {
				return err
			}
		}
		b, err := r.ReadByte()
		if err != nil {
			return err
		}
		if b == tnIAC {
			cmd, err := r.ReadByte()
			if err != nil {
				return err
			}
			switch cmd {
			case tnIAC: // A doubled IAC is literal data.
			case tnSB:
				// Discard subnegotiation for unimplemented options without buffering it.
				for {
					b, err = r.ReadByte()
					if err != nil {
						return err
					}
					if b == tnIAC {
						b, err = r.ReadByte()
						if err != nil {
							return err
						}
						if b == tnSE {
							break
						}
					}
				}
				continue
			case tnWILL, tnWONT, tnDO, tnDONT:
				opt, err := r.ReadByte()
				if err != nil {
					return err
				}
				var reply byte
				switch cmd {
				case tnWILL:
					if opt != 1 && opt != 3 {
						reply = tnDONT
					} else if !remote[opt] {
						remote[opt] = true
						reply = tnDO
					}
				case tnWONT:
					if remote[opt] {
						remote[opt] = false
						reply = tnDONT
					}
				case tnDO:
					if opt != 3 {
						reply = tnWONT
					} else if !localSGA {
						localSGA = true
						reply = tnWILL
					}
				case tnDONT:
					if opt == 3 && localSGA {
						localSGA = false
						reply = tnWONT
					}
				}
				if reply != 0 {
					if opt == 1 && (cmd == tnWILL || cmd == tnWONT) {
						s.remoteEcho.Store(remote[opt])
					}
					if err = s.send(conn, []byte{tnIAC, reply, opt}); err != nil {
						return err
					}
				}
				continue
			default:
				continue
			}
		}
		if !afterCR || b != 0 {
			text.WriteByte(b)
		}
		afterCR = b == '\r'
	}
}

func (c *ToolsConsole) runTelnet(args []string) error {
	if len(args) < 1 || len(args) > 2 || strings.TrimSpace(args[0]) == "" {
		return fmt.Errorf("用法：telnet 主机 [端口]")
	}
	port := 23
	if len(args) == 2 {
		port, _ = strconv.Atoi(args[1])
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("端口范围为 1–65535")
	}
	address := net.JoinHostPort(strings.Trim(args[0], "[]"), strconv.Itoa(port))
	c.editor.suspend()
	c.editor.SetBracketedPasteMode(false)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &localTelnet{cancel: cancel, output: c.writer}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return io.ErrClosedPipe
	}
	c.process, c.processName = s, "telnet"
	c.epoch.Add(1)
	c.managed.Store(false)
	changed := c.stateChanged
	c.mu.Unlock()
	if changed != nil {
		changed()
	}
	fmt.Fprintf(c.writer, "正在连接 %s…\r\n", address)
	err := s.run(ctx, address, c.writer)
	c.mu.Lock()
	c.process, c.processName = nil, ""
	c.epoch.Add(1)
	c.managed.Store(!c.closed)
	changed = c.stateChanged
	c.mu.Unlock()
	if changed != nil {
		changed()
	}
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, context.Canceled) && !errors.Is(err, net.ErrClosed) {
		return err
	}
	io.WriteString(c.writer, "\r\nTelnet 已断开。\r\n")
	return nil
}

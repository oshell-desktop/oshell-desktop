// SPDX-License-Identifier: GPL-3.0-only
package core

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

type commandConn struct {
	reader  io.ReadCloser
	writer  io.WriteCloser
	command *exec.Cmd
	peer    net.Addr
	done    chan struct{}
	once    sync.Once
}

func (c *commandConn) Read(p []byte) (int, error)  { return c.reader.Read(p) }
func (c *commandConn) Write(p []byte) (int, error) { return c.writer.Write(p) }
func (c *commandConn) Close() error {
	c.once.Do(func() {
		c.writer.Close()
		c.reader.Close()
		select {
		case <-c.done:
		default:
			stopCommand(c.command)
		}
	})
	return nil
}
func (c *commandConn) LocalAddr() net.Addr              { return proxyAddr("local proxy command") }
func (c *commandConn) RemoteAddr() net.Addr             { return c.peer }
func (c *commandConn) SetDeadline(time.Time) error      { return nil }
func (c *commandConn) SetReadDeadline(time.Time) error  { return nil }
func (c *commandConn) SetWriteDeadline(time.Time) error { return nil }

type proxyAddr string

func (proxyAddr) Network() string  { return "proxy-command" }
func (p proxyAddr) String() string { return string(p) }

// The command originates only from the user's OpenSSH configuration, never
// from imported session fields or remote terminal output.
func dialProxyCommand(ctx context.Context, template string, p Profile, originalHost ...string) (net.Conn, error) {
	if strings.ContainsAny(template, "\x00\r\n") {
		return nil, fmt.Errorf("ProxyCommand 包含不支持的字符")
	}
	command, e := expandProxyCommand(template, p, originalHost...)
	if e != nil {
		return nil, e
	}
	cmd := proxyShellCommand(ctx, command)
	prepareCommand(cmd)
	inRead, input, e := os.Pipe()
	if e != nil {
		return nil, e
	}
	output, outWrite, e := os.Pipe()
	if e != nil {
		inRead.Close()
		input.Close()
		return nil, e
	}
	cmd.Stdin = inRead
	cmd.Stdout = outWrite
	cmd.Stderr = io.Discard
	// Host-key verification needs a valid host:port even for a pipe transport.
	// Keep its network type distinct: this is not an observed TCP endpoint.
	c := &commandConn{reader: output, writer: input, command: cmd, peer: proxyAddr(p.Address()), done: make(chan struct{})}
	// Killing just the shell leaves its child holding the pipes open. Cancel
	// the connection and process tree together so blocked SSH reads wake up.
	cmd.Cancel = func() error { return c.Close() }
	if e = cmd.Start(); e != nil {
		inRead.Close()
		input.Close()
		output.Close()
		outWrite.Close()
		return nil, e
	}
	inRead.Close()
	outWrite.Close()
	go func() { cmd.Wait(); close(c.done) }()
	return c, nil
}

func expandProxyCommand(template string, p Profile, originalHost ...string) (string, error) {
	original := p.Host
	if len(originalHost) > 0 && originalHost[0] != "" {
		original = originalHost[0]
	}
	values := map[byte]string{'h': p.Host, 'n': original, 'p': strconv.Itoa(p.Port), 'r': p.Username}
	if runtime.GOOS == "windows" {
		for _, v := range values {
			if strings.ContainsAny(v, "&|<>^%!\"\r\n") {
				return "", fmt.Errorf("ProxyCommand 身份字段含有不安全的命令字符")
			}
		}
		var out strings.Builder
		for i := 0; i < len(template); i++ {
			if template[i] != '%' {
				out.WriteByte(template[i])
				continue
			}
			i++
			if i == len(template) {
				return "", fmt.Errorf("ProxyCommand 变量不完整")
			}
			if template[i] == '%' {
				out.WriteByte('%')
				continue
			}
			value, ok := values[template[i]]
			if !ok {
				return "", fmt.Errorf("不支持的 ProxyCommand 变量 %%%c", template[i])
			}
			out.WriteString(value)
		}
		return out.String(), nil
	}
	var out strings.Builder
	quote := byte(0)
	for i := 0; i < len(template); i++ {
		b := template[i]
		if b == '\\' && quote != '\'' && i+1 < len(template) {
			out.WriteByte(b)
			i++
			out.WriteByte(template[i])
			continue
		}
		if b == '\'' && quote != '"' {
			if quote == '\'' {
				quote = 0
			} else {
				quote = '\''
			}
			out.WriteByte(b)
			continue
		}
		if b == '"' && quote != '\'' {
			if quote == '"' {
				quote = 0
			} else {
				quote = '"'
			}
			out.WriteByte(b)
			continue
		}
		if b == '%' {
			if i+1 == len(template) {
				return "", fmt.Errorf("ProxyCommand 变量不完整")
			}
			i++
			code := template[i]
			if code == '%' {
				out.WriteByte('%')
				continue
			}
			v, ok := values[code]
			if !ok {
				return "", fmt.Errorf("不支持的 ProxyCommand 变量 %%%c", code)
			}
			switch quote {
			case '\'':
				out.WriteString(strings.ReplaceAll(v, "'", `'\''`))
			case '"':
				out.WriteString(strings.NewReplacer(`\`, `\\`, "$", `\$`, "`", "\\`", `"`, `\"`).Replace(v))
			default:
				out.WriteString(shellQuote(v))
			}
			continue
		}
		out.WriteByte(b)
	}
	if quote != 0 {
		return "", fmt.Errorf("ProxyCommand 引号不完整")
	}
	return out.String(), nil
}

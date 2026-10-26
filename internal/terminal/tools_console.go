// SPDX-License-Identifier: GPL-3.0-only
package terminal

import (
	"fmt"
	"io"
	"os"
	"oshell-desktop/internal/core"
	"oshell-desktop/internal/terminal/internal/pty"
	"strings"
	"sync"
	"sync/atomic"
)

type ToolsConsole struct {
	epoch        atomic.Uint64
	mu           sync.Mutex
	process      *pty.PTY
	processName  string
	starting     bool
	stateChanged func()
	output       *io.PipeReader
	writer       *io.PipeWriter
	editor       *promptEditor
	cols, rows   int
	closed       bool
	once         sync.Once
	managed      atomic.Bool
}

func NewToolsConsole() *ToolsConsole {
	c := &ToolsConsole{cols: 80, rows: 24}
	c.output, c.writer = io.Pipe()
	c.editor = newPromptEditor(c.writer)
	c.managed.Store(true)
	go c.run()
	return c
}
func (c *ToolsConsole) Read(b []byte) (int, error) { return c.output.Read(b) }
func (c *ToolsConsole) Write(b []byte) (int, error) {
	c.mu.Lock()
	p, closed := c.process, c.closed
	c.mu.Unlock()
	if closed {
		return 0, io.ErrClosedPipe
	}
	if p != nil {
		return p.Write(b)
	}
	if _, e := c.editor.Write(b); e != nil {
		return 0, e
	}
	return len(b), nil
}
func (c *ToolsConsole) Resize(cols, rows int) error {
	c.mu.Lock()
	c.cols, c.rows = cols, rows
	p := c.process
	c.mu.Unlock()
	if p != nil {
		return p.Resize(cols, rows, 0, 0)
	}
	return c.editor.SetSize(cols, rows)
}
func (c *ToolsConsole) ManagedInputAllowed() bool { return c.managed.Load() }
func (c *ToolsConsole) Close() error {
	c.once.Do(func() {
		c.mu.Lock()
		c.closed = true
		p := c.process
		c.mu.Unlock()
		c.managed.Store(false)
		c.editor.Close()
		c.writer.Close()
		c.output.Close()
		if p != nil {
			p.Close()
		}
	})
	return nil
}
func (c *ToolsConsole) run() {
	defer c.Close()
	io.WriteString(c.writer, "OShell 本机网络工具\r\nhelp 查看用法，tools 查看安装状态，exit 关闭。\r\n")
	for {
		c.editor.SetBracketedPasteMode(true)
		line, e := c.editor.ReadLine()
		if e != nil {
			return
		}
		line = strings.TrimSpace(line)
		switch line {
		case "":
			continue
		case "exit", "quit":
			return
		case "help", "?":
			io.WriteString(c.writer, strings.Join(core.LocalTools, "、")+"\r\n支持参数和引号，不解析管道、重定向或变量；Ctrl+C 中断工具。\r\n")
			continue
		case "tools":
			io.WriteString(c.writer, core.ToolAvailability()+"\r\n")
			continue
		}
		command, e := core.ParseToolCommand(line)
		if e != nil {
			io.WriteString(c.writer, e.Error()+"\r\n")
			continue
		}
		c.editor.suspend()
		c.editor.SetBracketedPasteMode(false)
		// Bytes following an external command belong to its interactive phase,
		// never to the next local prompt or its history.
		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			return
		}
		cols, rows := c.cols, c.rows
		c.starting = true
		c.epoch.Add(1)
		c.mu.Unlock()
		dir, _ := os.UserHomeDir()
		proc, e := pty.Start(pty.Options{Path: command.Path, Args: append([]string{command.Path}, command.Arguments...), Dir: dir, Env: environment(nil), Cols: cols, Rows: rows})
		if e != nil {
			c.mu.Lock()
			c.starting = false
			c.mu.Unlock()
			io.WriteString(c.writer, e.Error()+"\r\n")
			continue
		}
		c.mu.Lock()
		c.starting = false
		if c.closed {
			c.mu.Unlock()
			proc.Close()
			return
		}
		c.epoch.Add(1)
		c.process = proc
		c.processName = strings.ToLower(command.Name)
		changed := c.stateChanged
		c.mu.Unlock()
		if changed != nil {
			changed()
		}
		c.managed.Store(command.ManagedInputAllowed())
		code := drainToolProcess(c.writer, proc)
		c.mu.Lock()
		c.epoch.Add(1)
		c.process = nil
		c.processName = ""
		changed = c.stateChanged
		closed := c.closed
		c.mu.Unlock()
		if changed != nil {
			changed()
		}
		if closed {
			return
		}
		c.managed.Store(true)
		fmt.Fprintf(c.writer, "\r\n[工具退出：%d]\r\n", code)
	}
}

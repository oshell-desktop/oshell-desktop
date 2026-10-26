package terminal

import (
	"io"
	"oshell-desktop/internal/terminal/internal/pty"
	"sync"
	"sync/atomic"
)

// A single VT survives the SSH/local process and the subsequent tools console.
// Input packets carry an epoch, so queued remote commands cannot cross into the
// local console when the transport disconnects.
type sessionFallback struct {
	mu             sync.Mutex
	current        io.ReadWriteCloser
	tools          *ToolsConsole
	closed         bool
	mode           atomic.Bool
	epoch          atomic.Uint64
	cols, rows     int
	before         func()
	changed        func()
	processChanged func()
	prefix         []byte
}

func (f *sessionFallback) Read(out []byte) (int, error) {
	for {
		f.mu.Lock()
		if f.closed {
			f.mu.Unlock()
			return 0, io.EOF
		}
		if len(f.prefix) > 0 {
			n := copy(out, f.prefix)
			f.prefix = f.prefix[n:]
			f.mu.Unlock()
			return n, nil
		}
		current, local := f.current, f.tools != nil
		f.mu.Unlock()
		n, e := current.Read(out)
		if n > 0 {
			return n, nil
		}
		if e == nil {
			return n, e
		}
		if local {
			return n, e
		}
		_ = current.Close()
		if f.before != nil {
			f.before()
		}
		f.mu.Lock()
		if f.closed {
			f.mu.Unlock()
			return 0, io.EOF
		}
		tools := NewToolsConsole()
		tools.SetStateHandler(f.processChanged)
		f.tools = tools
		f.current = tools
		f.prefix = []byte("\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[?2004l\x1b[0m\r\n[会话已结束 · 本机工具模式]\r\n")
		f.epoch.Add(1)
		f.mode.Store(true)
		cols, rows := f.cols, f.rows
		f.mu.Unlock()
		// Resize is asynchronous because the line editor may be waiting for its
		// first greeting to be consumed through this same reader.
		go tools.Resize(cols, rows)
		if f.changed != nil {
			f.changed()
		}
	}
}
func (f *sessionFallback) Write(data []byte) (int, error) {
	return f.WriteGeneration(data, f.epoch.Load())
}
func (f *sessionFallback) WriteGeneration(data []byte, epoch uint64) (int, error) {
	f.mu.Lock()
	current, closed, local := f.current, f.closed, f.tools != nil
	if epoch != f.epoch.Load() {
		f.mu.Unlock()
		return len(data), nil
	}
	f.mu.Unlock()
	if closed {
		return 0, io.ErrClosedPipe
	}
	n, e := current.Write(data)
	if e != nil && !local {
		return len(data), nil
	} // EOF drives the transition on reader.
	return n, e
}
func (f *sessionFallback) Close() error {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return nil
	}
	f.closed = true
	current := f.current
	f.mu.Unlock()
	return current.Close()
}
func (f *sessionFallback) Resize(cols, rows int) error {
	f.mu.Lock()
	f.cols, f.rows = cols, rows
	current := f.current
	f.mu.Unlock()
	if p, ok := current.(*pty.PTY); ok {
		return p.Resize(cols, rows, 0, 0)
	}
	if sized, ok := current.(interface{ Resize(int, int) error }); ok {
		return sized.Resize(cols, rows)
	}
	return nil
}
func (f *sessionFallback) ManagedInputAllowed() bool {
	f.mu.Lock()
	tools := f.tools
	closed := f.closed
	f.mu.Unlock()
	return !closed && (tools == nil || tools.ManagedInputAllowed())
}
func (t *Terminal) InLocalToolsMode() bool { f := t.fallback.Load(); return f != nil && f.mode.Load() }
func (t *Terminal) ToolsProcessActive() bool {
	if console, ok := t.opts.Conn.(*ToolsConsole); ok && console.ProcessActive() {
		return true
	}
	f := t.fallback.Load()
	if f == nil {
		return false
	}
	f.mu.Lock()
	tools := f.tools
	f.mu.Unlock()
	return tools != nil && tools.ProcessActive()
}
func (c *ToolsConsole) ProcessActive() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.closed && (c.starting || c.process != nil)
}

// HasActiveProcess excludes an idle built-in tools prompt from exit warnings.
func (t *Terminal) HasActiveProcess() bool {
	select {
	case <-t.done:
		return false
	default:
	}
	if t.InLocalToolsMode() {
		return t.ToolsProcessActive()
	}
	if console, ok := t.opts.Conn.(interface{ ProcessActive() bool }); ok {
		return console.ProcessActive()
	}
	return true
}

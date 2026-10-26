package terminal

import "fmt"

// Opaque runtime identity; never persisted or sent to a remote host.
func (t *Terminal) PasteContextToken() string {
	t.mu.Lock()
	proc := t.proc
	t.mu.Unlock()
	tools, _ := t.opts.Conn.(*ToolsConsole)
	if f := t.fallback.Load(); f != nil {
		f.mu.Lock()
		if f.tools != nil {
			tools = f.tools
		}
		f.mu.Unlock()
	}
	var epoch uint64
	if tools != nil {
		epoch = tools.epoch.Load()
	}
	return fmt.Sprintf("%p:%p:%d:%t", proc, tools, epoch, t.InLocalToolsMode())
}

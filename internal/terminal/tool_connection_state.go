package terminal

func (c *ToolsConsole) SetStateHandler(fn func()) { c.mu.Lock(); c.stateChanged = fn; c.mu.Unlock() }
func (c *ToolsConsole) ConnectionActive() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.closed && c.process != nil && (c.processName == "ssh" || c.processName == "telnet")
}

func (t *Terminal) LocalImagePasteAllowed() bool {
	tools, _ := t.opts.Conn.(*ToolsConsole)
	if f := t.fallback.Load(); f != nil {
		f.mu.Lock()
		if f.tools != nil {
			tools = f.tools
		}
		f.mu.Unlock()
	}
	if tools == nil {
		return t.Available()
	}
	tools.mu.Lock()
	defer tools.mu.Unlock()
	return !tools.closed && !tools.starting && tools.process != nil && tools.processName != "ssh" && tools.processName != "telnet"
}
func (t *Terminal) ToolConnectionActive() bool {
	if f := t.fallback.Load(); f != nil {
		f.mu.Lock()
		tools := f.tools
		f.mu.Unlock()
		if tools != nil {
			return tools.ConnectionActive()
		}
	}
	if tools, ok := t.opts.Conn.(*ToolsConsole); ok {
		return tools.ConnectionActive()
	}
	return false
}

func (t *Terminal) LocalProcessRunning() bool {
	if !t.HasActiveProcess() || t.InLocalToolsMode() {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.proc != nil || t.opts.Conn != nil
}

package terminal

import (
	"bytes"
	"oshell-desktop/internal/core"
)

func (t *Terminal) noteIdleInput(data []byte) {
	if len(data) == 0 {
		return
	}
	if bytes.LastIndexAny(data, "\r\n\x03\x15") == len(data)-1 {
		t.idleEditing.Store(false)
		return
	}
	t.idleEditing.Store(true)
}

// IdleInputSafe is called by the connection timer; never consults UI-owned state.
func (t *Terminal) IdleInputSafe() bool {
	if !t.Available() || t.idleEditing.Load() {
		return false
	}
	line, ok := t.HostPrompt()
	return ok && !core.IsAuthenticationPrompt(line)
}

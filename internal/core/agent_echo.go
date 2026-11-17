package core

// Match only the bootstrap's complete echo, allowing a late prompt to precede
// it. Unmatched and partial output is always returned to the terminal.
type AgentIntegrationEcho struct {
	command   string
	candidate *HostProbeEcho
	consumed  int
	finished  bool
}

func NewAgentIntegrationEcho(command string) *AgentIntegrationEcho {
	return &AgentIntegrationEcho{command: command}
}
func (e *AgentIntegrationEcho) Flush() []byte {
	e.finished = true
	if e.candidate == nil {
		return nil
	}
	out := e.candidate.Flush()
	e.candidate = nil
	return out
}
func (e *AgentIntegrationEcho) Consume(data []byte) []byte {
	var out []byte
	for _, b := range data {
		if e.finished {
			out = append(out, b)
			continue
		}
		e.consumed++
		if e.consumed > 16384 {
			out = append(out, e.Flush()...)
			out = append(out, b)
			continue
		}
		if e.candidate == nil && b != e.command[0] {
			out = append(out, b)
			continue
		}
		if e.candidate == nil {
			e.candidate = NewHostProbeEcho(e.command)
			e.candidate.shellEditing = true
		}
		visible := e.candidate.Consume([]byte{b})
		if e.candidate.finished {
			matched := e.candidate.matched
			e.candidate = nil
			if matched {
				e.finished = true
			} else if b == e.command[0] && len(visible) > 0 && visible[len(visible)-1] == b {
				out = append(out, visible[:len(visible)-1]...)
				e.candidate = NewHostProbeEcho(e.command)
				e.candidate.shellEditing = true
				visible = e.candidate.Consume([]byte{b})
			}
		}
		out = append(out, visible...)
	}
	return out
}
func (s *AgentSession) FilterIntegrationEcho(data []byte) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.integrationEcho == nil {
		return data
	}
	out := s.integrationEcho.Consume(data)
	if !s.integrationPending {
		out = append(out, s.integrationEcho.Flush()...)
		s.integrationEcho = nil
	}
	return out
}

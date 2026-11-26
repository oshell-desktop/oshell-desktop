// OShell extensions to the MIT-licensed mygo terminal adapter.
package terminal

import (
	"fmt"
	"io"

	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"oshell-desktop/internal/terminal/internal/vt"
)

// ReplayUserInput applies a semantic event through this terminal's own encoder.
// Device replies, mouse reports and IME preedit are never synchronized.
// Call on the UI thread, as with View.
func (t *Terminal) ReplayUserInput(ev ui.InputEvent) {
	if !t.ManagedInputAvailable() {
		return
	}
	if ev.Kind != ui.InputKeyDown && ev.Kind != ui.InputKeyUp && ev.Kind != ui.InputText {
		return
	}
	if t.v == nil {
		t.v = &view{t: t, blinkStart: time.Now()}
		t.v.shaped.init(4096)
	}
	t.v.input(ev)
}
func (t *Terminal) RequestPaste(text string) {
	if text == "" || !t.Available() {
		return
	}
	if t.opts.BeforePaste != nil && !t.opts.BeforePaste(text) {
		return
	}
	t.ConfirmedPaste(text)
}
func (t *Terminal) ConfirmedPaste(text string) {
	if !t.paste(text) {
		return
	}
	if t.opts.OnUserPaste != nil {
		t.opts.OnUserPaste(text)
	}
}
func (t *Terminal) cleanCopy(s string) string {
	if t.opts.TrimCopyLeading {
		s = strings.TrimLeftFunc(s, unicode.IsSpace)
	}
	if t.opts.TrimCopyTrailing {
		s = strings.TrimRightFunc(s, unicode.IsSpace)
	}
	return s
}
func (t *Terminal) SetInputPolicy(copyOnSelect, rightPaste, trimLeading, trimTrailing bool) {
	t.opts.CopyOnSelect = copyOnSelect
	t.opts.RightClickPaste = rightPaste
	t.opts.TrimCopyLeading = trimLeading
	t.opts.TrimCopyTrailing = trimTrailing
}

func (t *Terminal) SetSelectionPolicy(separators string, shiftWhitespace bool) {
	t.opts.CustomWordSelection = true
	t.opts.WordSeparators = separators
	t.opts.ShiftWhitespace = shiftWhitespace
}

func (t *Terminal) SetMiddleClickPaste(enabled bool) { t.opts.MiddleClickPaste = enabled }

type HighlightRule struct {
	Pattern                    string
	Regex, CaseSensitive, Bold bool
	Foreground, Background     ui.Color
}
type compiledHighlight struct {
	pattern *core.Pattern
	rule    HighlightRule
}

func (t *Terminal) SetHighlights(rules []HighlightRule) error {
	compiled := make([]compiledHighlight, 0, len(rules))
	rules = rules[:min(len(rules), 32)]
	for _, r := range rules {
		if r.Pattern == "" {
			continue
		}
		p := r.Pattern
		if !r.Regex {
			p = regexp.QuoteMeta(p)
		}
		if !r.CaseSensitive {
			p = "(?i)" + p
		}
		re, e := core.CompilePattern(p, 2*time.Millisecond)
		if e != nil {
			return e
		}
		compiled = append(compiled, compiledHighlight{re, r})
	}
	t.mu.Lock()
	old := t.highlights
	t.highlights = compiled
	t.highlightVersion++
	t.mu.Unlock()
	for _, rule := range old {
		rule.pattern.Close()
	}
	t.redraw()
	return nil
}
func (v *view) rowHighlights(rs *vt.RenderState, cells []vt.Cell) map[int]HighlightRule {
	if len(v.rules) == 0 {
		return nil
	}
	var text []rune
	units := 0
	var columns []int
	var buf []rune
	for x, cell := range cells {
		if cell.Wide == vt.SpacerHead || cell.Wide == vt.SpacerTail {
			continue
		}
		runes := []rune{rune(cell.Codepoint)}
		if cell.Codepoint == 0 {
			runes[0] = ' '
		}
		if cell.Grapheme {
			buf = rs.CellGraphemes(x, buf[:0])
			runes = buf
		}
		for _, r := range runes {
			units++
			if r > 0xffff {
				units++
			}
			text = append(text, r)
			columns = append(columns, x)
		}
	}
	if units > 4096 {
		return nil
	}
	raw := string(text)
	remaining := 256
	out := map[int]HighlightRule{}
	deadline := time.Now().Add(2 * time.Millisecond)
	for _, h := range v.rules {
		if time.Now().After(deadline) {
			break
		}
		matches, e := h.pattern.FindAllStringIndex(raw, remaining)
		if e != nil {
			break
		}
		for _, m := range matches {
			if m[0] == m[1] {
				continue
			}
			remaining--
			start := utf8.RuneCountInString(raw[:m[0]])
			end := start + utf8.RuneCountInString(raw[m[0]:m[1]])
			for i := start; i < end && i < len(columns); i++ {
				if _, taken := out[columns[i]]; !taken {
					out[columns[i]] = h.rule
				}
			}
			if remaining == 0 {
				return out
			}
		}
	}
	return out
}

func (t *Terminal) SetHistoryLimit(lines int) error {
	if lines < 500 || lines > 100000 {
		return fmt.Errorf("历史行数范围为 500–100000")
	}
	t.mu.Lock()
	if t.term == nil {
		t.mu.Unlock()
		return nil
	}
	if t.historyLines == lines {
		t.mu.Unlock()
		return nil
	}
	err := t.term.SetHistoryLines(lines)
	if err == nil {
		t.historyLines = lines
		t.term.ClampHistory(lines)
		t.revision++
	}
	t.mu.Unlock()
	t.redraw()
	return err
}

// SendManagedText batches a composer dispatch into one input packet and never
// echoes it through the synchronized-keyboard callback. Unlike clipboard paste,
// reviewed composer text is sent as command lines even in bracketed-paste mode.
func (t *Terminal) SendManagedText(text string, appendReturn bool) {
	if !t.Available() {
		return
	}
	data := core.ManagedInput(text, appendReturn)
	t.mu.Lock()
	if t.term == nil {
		t.mu.Unlock()
		return
	}
	t.term.ScrollToBottom()
	t.mu.Unlock()
	t.Send(data)
	t.redraw()
}

func (t *Terminal) ManagedInputAvailable() bool {
	if f := t.fallback.Load(); f != nil && !f.ManagedInputAllowed() {
		return false
	}
	return t.Available() && (t.opts.ManagedInputAllowed == nil || t.opts.ManagedInputAllowed())
}

// Edit routes the native Edit menu through this view, on the UI thread.
func (t *Terminal) Edit(command string) {
	if t.v != nil {
		t.v.input(ui.InputEvent{Kind: ui.InputCommand, Text: command})
	}
}

func (t *Terminal) SetVisible(visible bool) {
	was := t.hidden.Swap(!visible)
	if !visible {
		t.stopScrollbarFade()
	}
	if !visible && !was {
		t.visibilityEpoch.Add(1)
		// A hidden view will not build another frame to report losing focus.
		t.mu.Lock()
		t.resumeSelectionLocked()
		t.rmu.Lock()
		// Row shapes and decoded display bitmaps can be rebuilt on return.
		// During synchronized output the previous complete frame is still needed.
		if v := t.v; v != nil && !t.held {
			v.shaped.clear()
			v.lines = nil
			v.bidiView = bidiView{}
			v.graphicBitmaps = nil
			v.graphics = nil
			v.graphicCell = [2]int{}
			if t.term != nil {
				t.term.ReleaseDisplayCaches()
			}
		}
		t.rmu.Unlock()
		if t.term != nil {
			t.term.SetFocus(false)
		}
		t.mu.Unlock()
	}
	if visible && was {
		t.redraw()
	}
}

func (t *Terminal) resumeSelectionLocked() {
	if t.selectionHold != nil {
		close(t.selectionHold)
		t.selectionHold = nil
	}
}

// HostPrompt reads the active screen rather than the scrolled viewport.
func (t *Terminal) HostPrompt() (string, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.term == nil || t.term.AltScreen() {
		return "", false
	}
	return t.term.CursorLine(), true
}

// Match only a fresh channel's opening dialogue. Screen contents alone can be
// redrawn later by a shell or restored from a previous connection.
type initialTargetLogin struct {
	account, prefix             string
	ready, checkingRetry, retry bool
}

func (s *initialTargetLogin) observe(data []byte) {
	if s.account == "" || len(data) == 0 {
		return
	}
	if s.ready || s.retry {
		for len(data) > 0 && data[0] == ' ' {
			data = data[1:]
		}
		if len(data) == 0 {
			return
		}
		if s.retry {
			*s = initialTargetLogin{}
			return
		}
		s.ready, s.checkingRetry, s.prefix = false, true, ""
	}
	if s.checkingRetry {
		if len(s.prefix)+len(data) <= 128 {
			s.prefix += string(data)
			response := strings.ReplaceAll(s.prefix, "\r\n", "\n")
			for _, message := range []string{"\nPassword:", "\nPermission denied, please try again.\nPassword:", "\nLogin incorrect\nPassword:", "\nAccess denied\nPassword:"} {
				if strings.TrimRight(response, " ") == message {
					s.retry = true
					return
				}
				if strings.HasPrefix(message, strings.TrimSuffix(response, "\r")) {
					return
				}
			}
		}
		*s = initialTargetLogin{}
		return
	}
	want := "Login: " + s.account + "\nPassword:"
	if len(s.prefix)+len(data) <= len(want)+32 {
		s.prefix += string(data)
		text := strings.TrimLeft(strings.ReplaceAll(s.prefix, "\r\n", "\n"), "\r\n")
		s.ready = strings.TrimRight(text, " ") == want
		if s.ready || strings.HasPrefix(want, strings.TrimSuffix(text, "\r")) {
			return
		}
	}
	*s = initialTargetLogin{}
}

func (t *Terminal) targetLoginPromptLocked(account string) bool {
	if !t.initialLogin.ready || t.initialLogin.account != account || t.term == nil || t.term.AltScreen() || strings.TrimSpace(t.term.CursorLine()) != "Password:" {
		return false
	}
	fields := strings.Fields(t.term.PreviousCursorLine())
	return len(fields) == 2 && fields[0] == "Login:" && fields[1] == account
}

func (t *Terminal) TargetLoginPrompt(account string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.targetLoginPromptLocked(account)
}

func (t *Terminal) TargetLoginRetry(account string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.initialLogin.retry && t.initialLogin.account == account && t.term != nil &&
		!t.term.AltScreen() && strings.TrimSpace(t.term.CursorLine()) == "Password:"
}

func (t *Terminal) FillServerPassword(conn *core.Connection, prompt, password, submitAccount string) (<-chan error, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if conn == nil || t.opts.Conn != conn || t.opts.Replay || t.InLocalToolsMode() ||
		t.term == nil || t.term.AltScreen() || prompt == "" || t.term.CursorLine() != prompt ||
		password == "" || len(password) > 4096 || strings.ContainsFunc(password, unicode.IsControl) ||
		submitAccount != "" && !t.targetLoginPromptLocked(submitAccount) {
		return nil, false
	}
	result := make(chan error, 1)
	packet := inputPacket{data: []byte(password), result: result}
	packet.writePassword = func(data []byte) error {
		if line, ok := t.HostPrompt(); !ok || line != prompt || !t.Available() || t.InLocalToolsMode() || !conn.Alive() ||
			submitAccount != "" && !t.TargetLoginPrompt(submitAccount) {
			return fmt.Errorf("密码提示或终端连接已变化")
		}
		// Deliver only to this SSH channel, even if the view has fallen back to a local console.
		write := func() error {
			if submitAccount != "" {
				line := make([]byte, len(data)+1)
				copy(line, data)
				line[len(data)] = '\r'
				defer clear(line)
				return conn.WriteInitialTargetLogin(line)
			}
			n, err := conn.Write(data)
			if err == nil && n != len(data) {
				return io.ErrShortWrite
			}
			return err
		}
		if t.opts.Agent != nil {
			return t.opts.Agent.GatePasswordInput(data, write)
		}
		return write()
	}
	if !t.in.pushInput(packet, nil) {
		clear(packet.data)
		return nil, false
	}
	return result, true
}

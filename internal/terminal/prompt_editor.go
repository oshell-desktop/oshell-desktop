package terminal

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/go-text/typesetting/segmenter"
	"golang.org/x/text/width"
	"oshell-desktop/internal/core"
)

// promptEditor is used ONLY while the local tools prompt owns input. Running
// tools (including ssh and password prompts) receive bytes directly via PTY.
type promptEditor struct {
	done               chan struct{}
	once               sync.Once
	gate               sync.Mutex
	reading            *promptRead
	changed            chan struct{}
	suspended          bool
	generation         uint64
	cols               atomic.Int32
	pending            []byte
	out                io.Writer
	line               []string
	pos                int
	kill               string
	history            []string
	historyIndex       int
	draft              string
	search             bool
	query, searchDraft string
	searchPos, match   int
	paste              bool
}
type promptRead struct {
	input chan []byte
	done  chan struct{}
}

func newPromptEditor(out io.Writer) *promptEditor {
	e := &promptEditor{done: make(chan struct{}), changed: make(chan struct{}), out: out, historyIndex: -1, match: -1}
	e.cols.Store(80)
	return e
}
func (e *promptEditor) Write(b []byte) (int, error) {
	e.gate.Lock()
	generation := e.generation
	e.gate.Unlock()
	for {
		e.gate.Lock()
		reading, changed := e.reading, e.changed
		discard := e.suspended || generation != e.generation
		e.gate.Unlock()
		if discard {
			return len(b), nil
		}
		if reading == nil {
			select {
			case <-changed:
				continue
			case <-e.done:
				return 0, io.ErrClosedPipe
			}
		}
		select {
		case reading.input <- append([]byte(nil), b...):
			return len(b), nil
		case <-reading.done:
			continue
		case <-e.done:
			return 0, io.ErrClosedPipe
		}
	}
}
func (e *promptEditor) suspend() {
	e.gate.Lock()
	e.suspended = true
	e.generation++
	close(e.changed)
	e.changed = make(chan struct{})
	e.gate.Unlock()
	e.pending = nil
}
func (e *promptEditor) Close()                       { e.once.Do(func() { close(e.done) }) }
func (e *promptEditor) SetSize(cols, rows int) error { e.cols.Store(int32(max(cols, 10))); return nil }
func (e *promptEditor) SetBracketedPasteMode(on bool) {
	if on {
		io.WriteString(e.out, "\x1b[?2004h")
	} else {
		io.WriteString(e.out, "\x1b[?2004l")
	}
}
func clusters(text string) []string {
	var s segmenter.Segmenter
	s.Init([]rune(text))
	it := s.GraphemeIterator()
	var out []string
	for it.Next() {
		out = append(out, string(it.Grapheme().Text))
	}
	return out
}
func (e *promptEditor) text() string    { return strings.Join(e.line, "") }
func (e *promptEditor) set(text string) { e.line = clusters(text); e.pos = len(e.line) }
func (e *promptEditor) insert(text string) {
	if len(e.text())+len(text) > 8192 {
		return
	}
	prefix := strings.Join(e.line[:e.pos], "") + text
	e.line = clusters(prefix + strings.Join(e.line[e.pos:], ""))
	e.pos = min(len(clusters(prefix)), len(e.line))
}
func (e *promptEditor) previousWord() int {
	p := e.pos
	for p > 0 && strings.TrimSpace(e.line[p-1]) == "" {
		p--
	}
	for p > 0 && strings.TrimSpace(e.line[p-1]) != "" {
		p--
	}
	return p
}
func (e *promptEditor) nextWord() int {
	p := e.pos
	for p < len(e.line) && strings.TrimSpace(e.line[p]) != "" {
		p++
	}
	for p < len(e.line) && strings.TrimSpace(e.line[p]) == "" {
		p++
	}
	return p
}
func (e *promptEditor) erase(from, to int) {
	e.kill = strings.Join(e.line[from:to], "")
	e.line = append(e.line[:from], e.line[to:]...)
	e.pos = from
}
func (e *promptEditor) historyMove(delta int) {
	if e.historyIndex < 0 {
		e.draft = e.text()
	}
	next := e.historyIndex + delta
	if next < 0 {
		e.historyIndex = -1
		e.set(e.draft)
		return
	}
	if next >= len(e.history) {
		return
	}
	e.historyIndex = next
	e.set(e.history[len(e.history)-1-next])
}
func (e *promptEditor) findHistory(direction int, repeat bool) {
	start := len(e.history) - 1
	if repeat {
		start = e.match + direction
		if e.match < 0 {
			start = len(e.history) - 1
		}
	}
	for i := start; i >= 0 && i < len(e.history); i += direction {
		if strings.Contains(e.history[i], e.query) {
			e.match = i
			e.set(e.history[i])
			return
		}
	}
	e.match = -1
	e.set("")
}
func (e *promptEditor) complete() {
	prefix := strings.Join(e.line[:e.pos], "")
	if strings.ContainsAny(prefix, " \t") {
		return
	}
	names := append([]string{"help", "tools", "exit", "quit"}, core.LocalTools...)
	var matches []string
	for _, n := range names {
		if strings.HasPrefix(n, prefix) {
			matches = append(matches, n)
		}
	}
	if len(matches) == 0 {
		return
	}
	common := matches[0]
	for _, n := range matches[1:] {
		for !strings.HasPrefix(n, common) {
			_, size := utf8.DecodeLastRuneInString(common)
			common = common[:len(common)-size]
		}
	}
	if len(matches) == 1 {
		common += " "
	} else if common == prefix {
		fmt.Fprint(e.out, "\r\n", strings.Join(matches, "  "), "\r\n")
	}
	e.line = append(clusters(common), e.line[e.pos:]...)
	e.pos = len(clusters(common))
}

// key returns a completed command, or EOF. Cursor positions are graphemes,
// so combining accents, CJK and joined Emoji are never split by editing.
func (e *promptEditor) key(key string) (string, bool, bool) {
	if key == "\x1b[200~" {
		e.paste = true
		return "", false, false
	}
	if key == "\x1b[201~" {
		e.paste = false
		return "", false, false
	}
	if e.paste {
		if key == "\r" || key == "\n" || key == "\t" {
			e.insert(" ")
		} else if len(key) > 0 && []rune(key)[0] >= 32 {
			e.insert(key)
		}
		return "", false, false
	}
	if e.search {
		switch key {
		case "\x12":
			e.findHistory(-1, true)
		case "\x13":
			e.findHistory(1, true)
		case "\x07", "\x03":
			e.search = false
			e.set(e.searchDraft)
			e.pos = min(e.searchPos, len(e.line))
		case "\x1b":
			e.search = false
			if e.match < 0 {
				e.set(e.searchDraft)
				e.pos = min(e.searchPos, len(e.line))
			}
		case "\r", "\n":
			if e.match < 0 {
				return "", false, false
			}
			e.search = false
			return e.key("\r")
		case "\x7f", "\b":
			q := clusters(e.query)
			if len(q) > 0 {
				e.query = strings.Join(q[:len(q)-1], "")
			}
			e.findHistory(-1, false)
		default:
			if len(e.query)+len(key) <= 64<<10 && len(key) > 0 && []rune(key)[0] >= 32 && !strings.HasPrefix(key, "\x1b") {
				e.query += key
				e.findHistory(-1, false)
			}
		}
		return "", false, false
	}
	switch key {
	case "\r", "\n":
		line := e.text()
		if strings.TrimSpace(line) != "" {
			if len(e.history) == 0 || e.history[len(e.history)-1] != line {
				e.history = append(e.history, line)
				if len(e.history) > 100 {
					e.history = append([]string(nil), e.history[len(e.history)-100:]...)
				}
			}
		}
		e.set("")
		e.historyIndex = -1
		return line, true, false
	case "\x04":
		if len(e.line) == 0 {
			return "", false, true
		}
		if e.pos < len(e.line) {
			e.line = append(e.line[:e.pos], e.line[e.pos+1:]...)
		}
	case "\x03":
		e.set("")
		e.historyIndex = -1
	case "\x01", "\x1b[H", "\x1bOH", "\x1b[1~", "\x1b[7~":
		e.pos = 0
	case "\x05", "\x1b[F", "\x1bOF", "\x1b[4~", "\x1b[8~":
		e.pos = len(e.line)
	case "\x02", "\x1b[D", "\x1bOD":
		e.pos = max(0, e.pos-1)
	case "\x06", "\x1b[C", "\x1bOC":
		e.pos = min(len(e.line), e.pos+1)
	case "\x1bb", "\x1b[1;5D", "\x1b[1;3D":
		e.pos = e.previousWord()
	case "\x1bf", "\x1b[1;5C", "\x1b[1;3C":
		e.pos = e.nextWord()
	case "\x10", "\x1b[A", "\x1bOA":
		e.historyMove(1)
	case "\x0e", "\x1b[B", "\x1bOB":
		e.historyMove(-1)
	case "\x7f", "\b":
		if e.pos > 0 {
			e.line = append(e.line[:e.pos-1], e.line[e.pos:]...)
			e.pos--
		}
	case "\x1b[3~":
		if e.pos < len(e.line) {
			e.line = append(e.line[:e.pos], e.line[e.pos+1:]...)
		}
	case "\x15":
		e.erase(0, e.pos)
	case "\x0b":
		e.erase(e.pos, len(e.line))
	case "\x17", "\x1b\x7f":
		e.erase(e.previousWord(), e.pos)
	case "\x19":
		e.insert(e.kill)
	case "\x14":
		if e.pos > 0 && len(e.line) > 1 {
			p := min(e.pos, len(e.line)-1)
			e.line[p-1], e.line[p] = e.line[p], e.line[p-1]
			e.pos = p + 1
		}
	case "\x0c":
		io.WriteString(e.out, "\x1b[2J\x1b[H")
	case "\x12", "\x13":
		e.search = true
		e.searchDraft = e.text()
		e.searchPos = e.pos
		e.query = ""
		e.match = -1
		e.findHistory(-1, false)
	case "\t":
		e.complete()
	default:
		if len(key) > 0 && []rune(key)[0] >= 32 && !strings.HasPrefix(key, "\x1b") {
			e.insert(key)
		}
	}
	return "", false, false
}
func clusterWidth(s string) int {
	w := 0
	for _, r := range s {
		if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || r == 0x200d {
			continue
		}
		v := 1
		k := width.LookupRune(r).Kind()
		if k == width.EastAsianWide || k == width.EastAsianFullwidth || r >= 0x1f000 && r <= 0x1ffff {
			v = 2
		}
		w = max(w, v)
	}
	if strings.ContainsRune(s, 0xfe0f) {
		w = max(w, 2)
	}
	return w
}
func (e *promptEditor) render() {
	prompt := "oshell> "
	if e.search {
		prompt = "搜索 [" + e.query + "]: "
		if e.match < 0 {
			prompt = "无匹配 [" + e.query + "]: "
		}
	}
	budget := max(int(e.cols.Load())-1, 9)
	p := clusters(prompt)
	for len(p) > 0 {
		w := 0
		for _, g := range p {
			w += clusterWidth(g)
		}
		if w < budget/2 {
			break
		}
		p = p[1:]
	}
	prompt = strings.Join(p, "")
	available := budget
	for _, g := range p {
		available -= clusterWidth(g)
	}
	start, posWidth := e.pos, 0
	for start > 0 && posWidth+clusterWidth(e.line[start-1]) < available {
		start--
		posWidth += clusterWidth(e.line[start])
	}
	end, w := start, 0
	for end < len(e.line) && w+clusterWidth(e.line[end]) <= available {
		w += clusterWidth(e.line[end])
		end++
	}
	fmt.Fprint(e.out, "\r\x1b[2K", prompt, strings.Join(e.line[start:end], ""))
	if w > posWidth {
		fmt.Fprintf(e.out, "\x1b[%dD", w-posWidth)
	}
}
func (e *promptEditor) ReadLine() (string, error) {
	e.gate.Lock()
	reading := &promptRead{make(chan []byte), make(chan struct{})}
	e.reading = reading
	e.suspended = false
	close(e.changed)
	e.changed = make(chan struct{})
	e.gate.Unlock()
	defer func() { e.gate.Lock(); e.reading = nil; close(reading.done); e.gate.Unlock() }()
	e.render()
	pending := e.pending
	e.pending = nil
	for {
		for len(pending) > 0 {
			n := 1
			if pending[0] == 27 {
				if len(pending) < 2 {
					break
				}
				n = 2
				if pending[1] == '[' || pending[1] == 'O' {
					n = 0
					for i := 2; i < len(pending); i++ {
						if pending[i] >= 0x40 && pending[i] <= 0x7e {
							n = i + 1
							break
						}
					}
					if n == 0 {
						break
					}
				}
			} else {
				if !utf8.FullRune(pending) {
					break
				}
				_, n = utf8.DecodeRune(pending)
			}
			key := string(pending[:n])
			pending = pending[n:]
			line, ready, eof := e.key(key)
			if eof {
				return "", io.EOF
			}
			if ready {
				e.pending = append([]byte(nil), pending...)
				fmt.Fprint(e.out, "\r\x1b[2Koshell> ", line, "\r\n")
				return line, nil
			}
		}
		e.render()
		var timeout <-chan time.Time
		if len(pending) > 0 && pending[0] == 27 {
			timeout = time.After(35 * time.Millisecond)
		}
		select {
		case <-e.done:
			return "", io.EOF
		case <-timeout:
			pending = nil
			e.key("\x1b")
		case b := <-reading.input:
			if len(pending)+len(b) <= 128<<10 {
				pending = append(pending, b...)
			} else {
				pending = nil
			}
		}
	}
}

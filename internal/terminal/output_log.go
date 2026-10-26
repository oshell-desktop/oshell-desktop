package terminal

import (
	"fmt"
	"io"
	"os"
	"sync"
)

// SF-I06: the fixed OShell PlainTextFilter state machine preserves UTF-8
// bytes and CR/LF/TAB. This is an output stream, not a screen transcription.
type plainOutputFilter struct{ state byte }

func (f *plainOutputFilter) consume(in []byte) []byte {
	out := make([]byte, 0, len(in))
	for _, b := range in {
		switch f.state {
		case 0:
			if b == 27 {
				f.state = 1
			} else if b == 10 || b == 13 || b == 9 || b >= 32 {
				out = append(out, b)
			}
		case 1:
			switch b {
			case '[':
				f.state = 2
			case ']', 'P', '^', '_':
				f.state = 3
			default:
				f.state = 0
			}
		case 2:
			if b >= 0x40 && b <= 0x7e {
				f.state = 0
			}
		case 3:
			if b == 7 {
				f.state = 0
			} else if b == 27 {
				f.state = 4
			}
		case 4:
			if b == '\\' {
				f.state = 0
			} else {
				f.state = 3
			}
		}
	}
	return out
}

const outputLogLimit = 4 << 20

type outputLog struct {
	mu      sync.Mutex
	wake    chan struct{}
	done    chan struct{}
	writer  io.WriteCloser
	queue   [][]byte
	pending int // Includes the write currently in flight.
	stopped bool
	err     error
	changed func()
}

func newOutputLog(w io.WriteCloser, changed func()) *outputLog {
	l := &outputLog{writer: w, wake: make(chan struct{}, 1), done: make(chan struct{}), changed: changed}
	go l.run()
	return l
}
func (l *outputLog) signal() {
	select {
	case l.wake <- struct{}{}:
	default:
	}
}
func (l *outputLog) append(b []byte) {
	l.mu.Lock()
	if l.stopped {
		l.mu.Unlock()
		return
	}
	overflow := len(b) > outputLogLimit-l.pending
	if overflow {
		l.stopped = true
		l.err = fmt.Errorf("日志积压超过 4 MiB，已停止记录")
	} else {
		l.queue = append(l.queue, append([]byte(nil), b...))
		l.pending += len(b)
	}
	l.mu.Unlock()
	l.signal()
	if overflow && l.changed != nil {
		l.changed()
	}
}
func (l *outputLog) stop()                { l.mu.Lock(); l.stopped = true; l.mu.Unlock(); l.signal() }
func (l *outputLog) state() (bool, error) { l.mu.Lock(); defer l.mu.Unlock(); return !l.stopped, l.err }
func (l *outputLog) run() {
	defer close(l.done)
	defer func() {
		if err := l.writer.Close(); err != nil {
			l.mu.Lock()
			if l.err == nil {
				l.err = err
			}
			l.mu.Unlock()
			if l.changed != nil {
				l.changed()
			}
		}
	}()
	var filter plainOutputFilter
	for {
		l.mu.Lock()
		if len(l.queue) == 0 {
			stop := l.stopped
			l.mu.Unlock()
			if stop {
				return
			}
			<-l.wake
			continue
		}
		b := l.queue[0]
		l.queue[0] = nil
		l.queue = l.queue[1:]
		l.mu.Unlock()
		text := filter.consume(b)
		n, err := l.writer.Write(text)
		if err == nil && n != len(text) {
			err = io.ErrShortWrite
		}
		l.mu.Lock()
		l.pending -= len(b)
		if err != nil {
			l.stopped = true
			l.err = err
			l.queue = nil
			l.pending = 0
		}
		l.mu.Unlock()
		if err != nil {
			if l.changed != nil {
				l.changed()
			}
			return
		}
	}
}
func (t *Terminal) observeOutput(b []byte) {
	t.logMu.Lock()
	l := t.logFile
	t.logMu.Unlock()
	if l != nil {
		l.append(b)
	}
	if t.opts.OnOutput != nil {
		t.opts.OnOutput(b)
	}
}
func (t *Terminal) StartLog(path string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	t.logMu.Lock()
	old := t.logFile
	t.logFile = newOutputLog(f, t.redraw)
	t.logPath = path
	t.logMu.Unlock()
	if old != nil {
		old.stop()
	}
	return nil
}

// StopLog never waits for disk I/O on the UI/terminal input thread.
func (t *Terminal) StopLog() {
	t.logMu.Lock()
	l := t.logFile
	t.logMu.Unlock()
	if l != nil {
		l.stop()
	}
}

// WaitLog is for shutdown verification and callers that explicitly need a flush.
func (t *Terminal) WaitLog() {
	t.logMu.Lock()
	l := t.logFile
	t.logMu.Unlock()
	if l != nil {
		<-l.done
	}
}
func (t *Terminal) LogState() (string, bool, error) {
	t.logMu.Lock()
	l, path := t.logFile, t.logPath
	t.logMu.Unlock()
	if l == nil {
		return path, false, nil
	}
	active, err := l.state()
	return path, active, err
}

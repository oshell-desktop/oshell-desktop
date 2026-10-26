// OShell's bounded terminal/protocol multiplexer. Terminal bytes remain in
// order; protocol sessions cannot close or consume the underlying shell.
package terminal

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"oshell-desktop/internal/zmodem"
	"sync"
	"sync/atomic"
	"time"
)

type transferController struct {
	mu       sync.Mutex
	term     *Terminal
	detector transferDetector
	active   *protocolSession
	// Cancellation must not wait behind feed's bounded-buffer backpressure.
	cancellable atomic.Pointer[protocolSession]
}
type protocolSession struct {
	ctx    context.Context
	cancel context.CancelFunc
	pipe   *protocolPipe
	done   chan struct{}
	err    error
	canRun int // input reader only; includes runs split across network reads
}

func newTransferController(t *Terminal) *transferController { return &transferController{term: t} }
func (t *Terminal) Available() bool {
	open, paused := t.inputStatus()
	return open && !paused
}

// inputStatus distinguishes a live view whose transport is temporarily owned
// by a transfer from a connection that can no longer take input at all.
func (t *Terminal) inputStatus() (open, paused bool) {
	select {
	case <-t.done:
		return false, false
	default:
	}
	t.in.mu.Lock()
	defer t.in.mu.Unlock()
	return !t.in.closed, t.in.paused
}
func (t *Terminal) CancelTransfer() {
	if t.transfers != nil {
		t.transfers.cancel()
	}
}
func (t *Terminal) SetAutomaticTransfers(enabled bool) { t.transferEnabled.Store(enabled) }
func (c *transferController) cancel() {
	active := c.cancellable.Load()
	if active != nil {
		active.cancel()
	}
}
func (c *transferController) finishInput() {
	c.cancel()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active != nil {
		c.active.cancel()
	}
	c.term.write(c.detector.flush())
}
func (c *transferController) feed(data []byte) { c.mu.Lock(); defer c.mu.Unlock(); c.process(data) }
func (c *transferController) process(data []byte) {
	for c.active != nil {
		session := c.active
		select {
		case <-session.done:
			c.complete(session)
			// Returned bytes can contain the next transfer's header. Route
			// this input to that new session instead of the text detector.
			continue
		default:
		}
		n, ok := session.feed(data)
		if ok {
			return
		}
		<-session.done
		c.complete(session)
		data = data[n:]
	}
	if !c.term.transferEnabled.Load() {
		c.term.write(c.detector.flush())
		c.term.write(data)
		return
	}
	text, upload, protocol := c.detector.consume(data)
	c.term.write(text)
	if protocol != nil {
		c.start(upload, protocol)
	}
}
func (c *transferController) start(upload bool, prefix []byte) {
	ctx, cancel := context.WithCancel(context.Background())
	session := &protocolSession{ctx: ctx, cancel: cancel, done: make(chan struct{})}
	session.pipe = newProtocolPipe(ctx, session.done, func(p []byte) (int, error) { return c.term.in.raw(ctx, p) })
	c.active = session
	c.cancellable.Store(session)
	c.term.in.pauseWithCancel(true, session.cancel)
	go func() {
		session.err = c.term.opts.TransferHandler(ctx, upload, session.pipe)
		session.pipe.finish()
		close(session.done)
		c.mu.Lock()
		defer c.mu.Unlock()
		c.complete(session)
	}()
	n, ok := session.feed(prefix)
	if !ok {
		<-session.done
		c.complete(session)
		if n < len(prefix) {
			c.process(prefix[n:])
		}
	}
}
func (c *transferController) complete(session *protocolSession) {
	if c.active != session {
		return
	}
	select {
	case <-session.done:
	default:
		return
	}
	c.active = nil
	c.cancellable.CompareAndSwap(session, nil)
	tail := session.pipe.takeTail()
	finishedBatch := session.err == nil || errors.Is(session.err, zmodem.ErrSkipped)
	if !finishedBatch {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		c.term.in.raw(ctx, []byte{0x18, 0x18, 0x18, 0x18, 0x18, 0x18, 0x18, 0x18, 8, 8, 8, 8, 8, 8, 8, 8})
		cancel()
	}
	session.cancel()
	c.term.in.pause(false)
	// Publish completion before a buffered header starts another handler;
	// otherwise the old completion can overwrite the new transfer's UI state.
	if c.term.opts.TransferFinished != nil {
		c.term.opts.TransferFinished(session.err)
	}
	if len(tail) > 0 && finishedBatch {
		c.process(tail)
	}
}

func (s *protocolSession) feed(data []byte) (int, bool) {
	// Escaped file data cannot contain five consecutive raw CAN bytes.
	// Observe aborts even while the handler is waiting for a file picker.
	for _, b := range data {
		if b == 0x18 {
			s.canRun++
			if s.canRun >= 5 {
				s.cancel()
			}
		} else {
			s.canRun = 0
		}
	}
	return s.pipe.feed(data)
}

type protocolPipe struct {
	ctx          context.Context
	sessionDone  <-chan struct{}
	write        func([]byte) (int, error)
	mu           sync.Mutex
	buffer       []byte
	returned     []byte
	changed      chan struct{}
	readDeadline time.Time
	finished     bool
}

func newProtocolPipe(ctx context.Context, done <-chan struct{}, write func([]byte) (int, error)) *protocolPipe {
	return &protocolPipe{ctx: ctx, sessionDone: done, write: write, changed: make(chan struct{})}
}
func (p *protocolPipe) notify() { close(p.changed); p.changed = make(chan struct{}) }
func (p *protocolPipe) feed(data []byte) (int, bool) {
	consumed := 0
	for len(data) > 0 {
		p.mu.Lock()
		if p.finished {
			p.mu.Unlock()
			return consumed, false
		}
		room := (256 << 10) - len(p.buffer)
		if room > 0 {
			n := min(room, len(data))
			p.buffer = append(p.buffer, data[:n]...)
			data = data[n:]
			consumed += n
			p.notify()
			p.mu.Unlock()
			continue
		}
		changed := p.changed
		p.mu.Unlock()
		select {
		case <-changed:
		case <-p.sessionDone:
			return consumed, false
		case <-p.ctx.Done():
			return consumed, false
		}
	}
	return consumed, true
}
func (p *protocolPipe) Read(out []byte) (int, error) {
	for {
		p.mu.Lock()
		if len(p.buffer) > 0 {
			n := copy(out, p.buffer)
			p.buffer = p.buffer[n:]
			p.notify()
			p.mu.Unlock()
			return n, nil
		}
		if p.finished {
			p.mu.Unlock()
			return 0, io.EOF
		}
		deadline, changed := p.readDeadline, p.changed
		p.mu.Unlock()
		var timer *time.Timer
		var timeout <-chan time.Time
		if !deadline.IsZero() {
			duration := time.Until(deadline)
			if duration <= 0 {
				return 0, os.ErrDeadlineExceeded
			}
			timer = time.NewTimer(duration)
			timeout = timer.C
		}
		select {
		case <-changed:
			if timer != nil {
				timer.Stop()
			}
		case <-timeout:
			return 0, os.ErrDeadlineExceeded
		case <-p.ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return 0, p.ctx.Err()
		}
	}
}
func (p *protocolPipe) Write(b []byte) (int, error)   { return p.write(b) }
func (p *protocolPipe) Close() error                  { return nil }
func (p *protocolPipe) LocalAddr() net.Addr           { return terminalAddr("terminal") }
func (p *protocolPipe) RemoteAddr() net.Addr          { return terminalAddr("zmodem") }
func (p *protocolPipe) SetDeadline(t time.Time) error { return p.SetReadDeadline(t) }
func (p *protocolPipe) SetReadDeadline(t time.Time) error {
	p.mu.Lock()
	p.readDeadline = t
	p.notify()
	p.mu.Unlock()
	return nil
}
func (p *protocolPipe) SetWriteDeadline(time.Time) error { return nil }
func (p *protocolPipe) ReturnBuffered(b []byte) {
	p.mu.Lock()
	p.returned = append(p.returned, b...)
	p.mu.Unlock()
}
func (p *protocolPipe) DiscardBuffered() { p.mu.Lock(); p.buffer = nil; p.notify(); p.mu.Unlock() }
func (p *protocolPipe) finish()          { p.mu.Lock(); p.finished = true; p.notify(); p.mu.Unlock() }
func (p *protocolPipe) takeTail() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := append(append([]byte(nil), p.returned...), p.buffer...)
	p.returned = nil
	p.buffer = nil
	return out
}

type terminalAddr string

func (terminalAddr) Network() string  { return "terminal" }
func (a terminalAddr) String() string { return string(a) }

// Drain a cancelled transfer before switching the same terminal to local tools.
// Protocol abort bytes must still target the old transport epoch.
func (c *transferController) endSession() {
	c.cancel()
	c.mu.Lock()
	active := c.active
	if active != nil {
		active.cancel()
	}
	c.mu.Unlock()
	if active != nil {
		<-active.done
		c.mu.Lock()
		c.complete(active)
		c.mu.Unlock()
	}
	c.finishInput()
}

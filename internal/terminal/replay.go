package terminal

import (
	"io"
	"sync"
)

// A replay has an emulator, never a PTY, process, network connection or host effects.
type replaySource struct {
	done chan struct{}
	once sync.Once
}

func (r *replaySource) Read([]byte) (int, error)    { <-r.done; return 0, io.EOF }
func (r *replaySource) Write(p []byte) (int, error) { return len(p), nil }
func (r *replaySource) Close() error                { r.once.Do(func() { close(r.done) }); return nil }
func NewReplay(font Font, theme *Theme) (*Terminal, error) {
	return New(Options{Conn: &replaySource{done: make(chan struct{})}, Replay: true, Font: font, Theme: theme, NoBlink: true, Scrollback: 4 << 20, ManagedClipboardBindings: true})
}

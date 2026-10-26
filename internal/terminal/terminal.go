// Package terminal is a terminal for MyGo's native UI: a view that runs a
// shell, or any program, in a pseudo-terminal, with the terminal emulator
// of Ghostty, libghostty-vt, and draws it with package ui.
//
//	term, err := terminal.New(terminal.Options{})
//	...
//	mygo.NewWindow(mygo.WindowOptions{Title: "Terminal", Content: ui.View(func(c ui.Frame) {
//		terminal.View(c, term).Fill()
//	})})
//
// It does what terminals do: colors, styles and Unicode (wide characters,
// emoji, grapheme clusters), the scrollback with reflow on resize, the
// alternate screen of full-screen programs, the keyboard as legacy
// sequences or the Kitty keyboard protocol, mouse reporting, bracketed
// paste, focus reports, synchronized output, hyperlinks (OSC 8), titles,
// the working directory (OSC 7). Remote clipboard access (OSC 52) is denied.
// Selecting with
// the pointer copies as Ghostty does: a drag selects cells, a double click
// words and a triple click lines; Option (macOS) or Alt drags a rectangle.
//
// libghostty-vt is a native library loaded at run time, with no cgo: `mygo
// build` and `mygo dev` put the build this package binds into the app, and
// programs run without them, as with `go run` and `go test`, download it
// once into the user's cache. See LibraryPath.
package terminal

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"oshell-desktop/internal/terminal/internal/pty"
	"oshell-desktop/internal/terminal/internal/vt"
)

// Options configure a terminal.
type Options struct {
	BeforeClipboardPaste func() bool
	CustomWordSelection  bool
	// Default enables OSC 133 clicks with no modifiers, as SwiftTerm does.
	SemanticClickBehavior  SemanticClickBehavior
	SemanticClickModifiers ui.Modifiers
	WordSeparators         string
	ShiftWhitespace        bool
	LocalToolsOnExit       bool
	OnSessionEnd           func()
	OnShellIdentity        func(core.HostIdentity)
	OnDirectory            func(string)
	PreferColorEmoji       bool
	// App-managed shortcuts suppress the widget default accelerators.
	ManagedClipboardBindings   bool
	AmbiguousCharactersAreWide bool
	ManagedInputAllowed        func() bool
	AutomaticTransfers         bool
	TransferHandler            func(context.Context, bool, net.Conn) error
	TransferFinished           func(error)

	// Input callbacks run on the UI thread; output callbacks on the reader.
	OnUserInput func(ui.InputEvent)
	// KeyAliases add alternative keys before the terminal protocol encoder.
	KeyAliases                                                       []KeyAlias
	OnUserPaste                                                      func(string)
	BeforePaste                                                      func(string) bool
	OnOutput                                                         func([]byte)
	OnActivity                                                       func()
	OnProcessState                                                   func()
	CopyOnSelect, RightClickPaste, TrimCopyLeading, TrimCopyTrailing bool

	// Command is the program the terminal runs and its arguments. Empty
	// runs the user's shell as a login shell, as terminal apps do: $SHELL,
	// else /bin/zsh on macOS and /bin/sh elsewhere; PowerShell on Windows.
	Command []string
	// Dir is the working directory of the program: the user's home
	// directory when empty.
	Dir string
	// Env adds to the environment of the program, as "KEY=value". The
	// terminal sets TERM=xterm-256color and COLORTERM=truecolor, and LANG
	// to a UTF-8 locale when no locale is set, as when the app starts from
	// the Finder.
	Env []string
	// Conn connects the terminal to something other than a program, such
	// as a remote shell: what the terminal reads from it shows, and what
	// is typed is written to it. Command, Dir and Env are then ignored. A
	// Conn with a method Resize(cols, rows int) error is told the size.
	Conn io.ReadWriteCloser

	// Font is the font of the text: the system's monospaced font at 13
	// DIPs when zero.
	Font Font
	// Theme sets the colors, and DarkTheme those in dark windows when set,
	// as Ghostty's theme = light:…,dark:… does (GhosttyTheme returns
	// Ghostty's themes). Without either, the terminal follows the window's
	// light or dark appearance, with LightTheme and DarkTheme.
	Theme, DarkTheme *Theme
	// Cursor is the shape of the cursor, which programs may change, and
	// NoBlink keeps it from blinking unless a program asks it to.
	Cursor  CursorStyle
	NoBlink bool
	// Scrollback is about how many bytes of output the terminal keeps above
	// the screen: 10 MB when zero, none when negative.
	Scrollback int
	// OptionAsAlt makes Option on macOS the Alt key of programs (Meta),
	// rather than the key that types accented letters and symbols.
	OptionAsAlt bool
	// Transparent leaves the terminal's background undrawn, so that what
	// is under the view shows through, as the window's material or an
	// element's translucent background, as Ghostty's background-opacity
	// does: cells with a background color of their own still draw it.
	// Give the theme the background the view shows on, which the text
	// under a block cursor takes.
	Transparent bool

	// OnTitle, OnExit, OnBell and OnNotify run on a goroutine of the
	// terminal when a program sets the title, when the program (or Conn)
	// ends, when it rings the bell, and when it asks for a desktop
	// notification (OSC 9, OSC 777). Calls to the terminal are safe in
	// them.
	OnTitle  func(title string)
	OnExit   func(code int)
	OnBell   func()
	OnNotify func(title, body string)
}

// CursorStyle is the shape of the cursor.
type CursorStyle int

const (
	CursorBlock CursorStyle = iota
	CursorBar
	CursorUnderline
)

// Terminal is a terminal emulator and the program it runs. Its methods are
// safe from any goroutine.
type Terminal struct {
	progress              terminalProgress
	outputActivity        core.OutputActivity
	idleEditing           atomic.Bool
	fallback              atomic.Pointer[sessionFallback]
	probeEcho             *core.HostProbeEcho
	integration           integrationScanner
	hidden                atomic.Bool
	visibilityEpoch       atomic.Uint64
	scrollTimerMu         sync.Mutex
	scrollTimer           *time.Timer
	scrollTimerGeneration uint64
	transfers             *transferController
	transferEnabled       atomic.Bool

	historyLines     int
	revision         uint64
	highlights       []compiledHighlight
	highlightVersion uint64
	logMu            sync.Mutex
	logFile          *outputLog
	logPath          string

	opts Options
	in   inputQueue

	// mu guards the emulator, which the reader changes and frames read.
	mu      sync.Mutex
	term    *vt.Terminal
	size    gridSize
	events  events
	closed  bool
	clipOut []string // what programs copied, for the view to write
	// src is the program's pseudo-terminal, or Conn: nil until the
	// program starts, from launch, its options.
	src    io.ReadWriteCloser
	proc   *pty.PTY
	launch *pty.Options
	timer  *time.Timer // starts the program when no view does

	srcOnce sync.Once
	srcErr  error
	// connMu guards the size Conn is told, from a goroutine at a time.
	connMu   sync.Mutex
	connSize gridSize
	connBusy bool

	// rmu guards what frames draw from: the render state, which a
	// program's synchronized update (mode 2026) holds.
	rmu       sync.Mutex
	rs        *vt.RenderState
	held      bool
	heldSince time.Time

	draw     atomic.Pointer[func()] // asks for a frame of the view
	done     chan struct{}
	doneOnce sync.Once
	code     atomic.Int64

	// curTheme is the view's theme, for the program's questions; t.mu
	// guards it. v is the view's state, on the main thread.
	curTheme *Theme
	v        *view
}

// gridSize is the size of the screen in cells and of a cell in pixels.
type gridSize struct{ cols, rows, cellW, cellH int }

// events are what a program did while the reader wrote its output, for
// the callbacks, which run once the lock is released.
type events struct {
	directory  bool
	identities []core.HostIdentity
	title      bool
	bells      int
	notify     [][2]string
}

// New makes a terminal running opts.Command, or connected to opts.Conn.
// The program starts once a view shows the terminal, at the size of the
// view, or half a second later at 80×24 when none does.
func New(opts Options) (*Terminal, error) {
	if err := Load(); err != nil {
		return nil, err
	}
	opts.Font.Features = append([]string(nil), opts.Font.Features...)
	opts.KeyAliases = append([]KeyAlias(nil), opts.KeyAliases...)
	t := &Terminal{opts: opts, done: make(chan struct{})}
	if opts.TransferHandler != nil {
		t.transfers = newTransferController(t)
		t.transferEnabled.Store(opts.AutomaticTransfers)
	}
	t.size = gridSize{80, 24, 8, 16}
	var err error
	t.term, err = vt.NewTerminal(80, 24, vt.Effects{
		WritePTY:     func(p []byte) { t.in.push(append([]byte(nil), p...)) },
		Bell:         func() { t.events.bells++ },
		Progress:     t.setProgress,
		TitleChanged: func() { t.events.title = true },
		PwdChanged:   func() { t.events.directory = true },
		RenderHold:   t.renderHold,
		// SF-I27: remote OSC52 never owns the host clipboard. User-initiated
		// copy/paste uses the view services independently of this callback.
		Clipboard: func(string, bool) bool { return false },
		Size:      func() (int, int, int, int) { return t.size.cols, t.size.rows, t.size.cellW, t.size.cellH },
		Dark:      func() bool { return t.curTheme == nil || t.curTheme.dark() },
		Notify: func(title, body string) {
			t.events.notify = append(t.events.notify, [2]string{title, body})
		},
	})
	if err != nil {
		return nil, err
	}
	// Until a host view is focused, a connection must not be told it has focus.
	t.term.SetFocus(false)
	// Keep native pixel geometry in sync with the initial grid before a fast
	// connection can send an inline image or query, even before the first view.
	if err := t.term.Resize(t.size.cols, t.size.rows, t.size.cellW, t.size.cellH); err != nil {
		t.term.Free()
		return nil, err
	}
	if err := t.term.SetAmbiguousWidth(opts.AmbiguousCharactersAreWide); err != nil {
		t.term.Free()
		return nil, err
	}
	// Desktop supplies the user's terminal theme before connecting. Install it
	// before the reader can process OSC colors or queries; the first frame must
	// not reapply it and erase colors already set by the remote program.
	if opts.Theme != nil {
		opts.Theme.apply(t.term)
		t.curTheme = opts.Theme
	}
	t.term.SetTerminfoName("xterm-256color")
	// Grapheme clusters take the cells of their width, as in Ghostty:
	// emoji with modifiers and joined emoji take two.
	t.term.SetModeDefault(vt.ModeGraphemes, true)
	switch {
	case opts.Scrollback < 0:
		t.term.SetScrollback(0)
	case opts.Scrollback > 0:
		t.term.SetScrollback(opts.Scrollback)
	default:
		t.term.SetScrollback(10 << 20)
	}
	t.term.SetDefaultCursor(vtCursor(opts.Cursor), !opts.NoBlink)
	if t.rs, err = vt.NewRenderState(); err != nil {
		t.term.Free()
		return nil, err
	}
	if opts.Conn != nil {
		t.src = opts.Conn
		if console, ok := opts.Conn.(*ToolsConsole); ok {
			console.SetStateHandler(opts.OnProcessState)
		}
		t.run()
		return t, nil
	}
	path, args, err := command(opts.Command)
	if err != nil {
		t.rs.Free()
		t.term.Free()
		return nil, err
	}
	dir := opts.Dir
	if dir == "" {
		dir, _ = os.UserHomeDir()
	}
	t.launch = &pty.Options{Path: path, Args: args, Dir: dir, Env: environment(opts.Env)}
	if runtime.GOOS == "windows" {
		// ConPTY keeps a screen of its own, which a resize pulling rows
		// back from the scrollback would disagree with.
		t.term.SetResizePullBack(false)
	}
	// The program starts once a view knows the size of the screen, so
	// that it draws its first screen at that size; a terminal no view
	// shows starts after half a second, at 80×24.
	t.mu.Lock()
	t.timer = time.AfterFunc(500*time.Millisecond, func() {
		t.mu.Lock()
		o := t.launch
		t.launch = nil
		t.mu.Unlock()
		t.start(o)
	})
	t.mu.Unlock()
	return t, nil
}

// run reads the output and writes the input of the program, or Conn.
func (t *Terminal) run() {
	t.mu.Lock()
	if t.opts.LocalToolsOnExit {
		f := &sessionFallback{current: t.src, cols: t.size.cols, rows: t.size.rows, before: func() {
			if t.transfers != nil {
				t.transfers.endSession()
			}
			t.StopLog()
			t.clearProgress()
			t.mu.Lock()
			alternate := t.term != nil && t.term.AltScreen()
			t.mu.Unlock()
			if alternate {
				t.write([]byte("\x1b[?1049l"))
			}
		}, changed: t.opts.OnSessionEnd, processChanged: t.opts.OnProcessState}
		t.src = f
		t.fallback.Store(f)
		t.in.mu.Lock()
		t.in.generation = f.epoch.Load
		t.in.mu.Unlock()
	}
	src := t.src
	t.mu.Unlock()
	go t.read()
	go t.in.run(src)
}

// start starts the program of launch, which the caller took, in a
// pseudo-terminal of the screen's size.
func (t *Terminal) start(o *pty.Options) {
	if o == nil {
		return
	}
	t.mu.Lock()
	s := t.size
	t.mu.Unlock()
	o.Cols, o.Rows, o.Width, o.Height = s.cols, s.rows, s.cols*s.cellW, s.rows*s.cellH
	proc, err := pty.Start(*o)
	if err != nil {
		t.write([]byte("\x1b[31m" + err.Error() + "\x1b[0m"))
		t.code.Store(-1)
		t.finish()
		return
	}
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		proc.Close()
		t.finish()
		return
	}
	t.proc, t.src = proc, proc
	if t.size != s {
		// Resized while it started.
		proc.Resize(t.size.cols, t.size.rows, t.size.cols*t.size.cellW, t.size.rows*t.size.cellH)
	}
	t.mu.Unlock()
	go func() {
		code, _ := proc.Wait()
		t.code.Store(int64(code))
		// What the program wrote before exiting is still to be read; then
		// hang up whatever else has the terminal, as terminal apps do.
		select {
		case <-t.done:
		case <-time.After(100 * time.Millisecond):
			proc.Close()
		}
	}()
	t.run()
}

// command returns the program to run and its arguments, with args[0] its
// name: the user's shell as a login shell ("-zsh") when cmd is empty.
func command(cmd []string) (path string, args []string, err error) {
	if len(cmd) > 0 {
		path, err = exec.LookPath(cmd[0])
		if err != nil {
			return "", nil, err
		}
		return path, cmd, nil
	}
	if runtime.GOOS == "windows" {
		for _, name := range []string{"pwsh.exe", "powershell.exe", os.Getenv("COMSPEC"), "cmd.exe"} {
			if p, err := exec.LookPath(name); name != "" && err == nil {
				return p, []string{name}, nil
			}
		}
		return "", nil, errors.New("terminal: no shell found")
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
		if runtime.GOOS == "darwin" {
			shell = "/bin/zsh"
		}
	}
	return shell, []string{"-" + filepath.Base(shell)}, nil
}

// environment returns the environment of the program: this process's with
// the terminal's variables and extra.
func environment(extra []string) []string {
	env := os.Environ()
	set := func(kv string) {
		k, _, _ := strings.Cut(kv, "=")
		for i, e := range env {
			key, _, _ := strings.Cut(e, "=")
			if key == k || runtime.GOOS == "windows" && strings.EqualFold(key, k) {
				env[i] = kv
				return
			}
		}
		env = append(env, kv)
	}
	set("TERM=xterm-256color")
	set("COLORTERM=truecolor")
	if runtime.GOOS != "windows" && os.Getenv("LANG") == "" && os.Getenv("LC_ALL") == "" && os.Getenv("LC_CTYPE") == "" {
		set("LANG=en_US.UTF-8")
	}
	for _, kv := range extra {
		set(kv)
	}
	return env
}

// read processes the program's output until it ends.
func (t *Terminal) read() {
	buf := make([]byte, 64<<10)
	for {
		n, err := t.src.Read(buf)
		if n > 0 {
			if t.transfers != nil {
				t.transfers.feed(buf[:n])
			} else {
				t.write(buf[:n])
			}
		}
		if err != nil {
			break
		}
	}
	if t.transfers != nil {
		t.transfers.finishInput()
	}
	t.ended()
}

// write processes output, then runs the callbacks of what it did.
func (t *Terminal) write(p []byte) {
	t.mu.Lock()
	var ev events
	if t.probeEcho != nil {
		p = t.probeEcho.Consume(p)
	}
	var title, directory string
	visibleOutput := t.outputActivity.Consume(p)
	if t.term != nil {
		t.integration.scan(p, t.term.Write, func(identity core.HostIdentity) {
			if !t.term.AltScreen() {
				t.events.identities = append(t.events.identities, identity)
			}
		})
		t.revision++
		ev, t.events = t.events, events{}
		if ev.directory {
			directory = t.term.Pwd()
		}
		if ev.title {
			title = t.term.Title()
		}
	}
	t.mu.Unlock()
	t.observeOutput(p)
	if visibleOutput && t.opts.OnActivity != nil {
		t.opts.OnActivity()
	}
	if ev.directory && t.opts.OnDirectory != nil {
		t.opts.OnDirectory(directory)
	}
	t.fire(ev, title)
	t.redraw()
}

func (t *Terminal) fire(ev events, title string) {
	o := &t.opts
	if o.OnShellIdentity != nil {
		for _, identity := range ev.identities {
			o.OnShellIdentity(identity)
		}
	}
	if ev.title && o.OnTitle != nil {
		o.OnTitle(title)
	}
	for range ev.bells {
		if o.OnBell != nil {
			o.OnBell()
		}
	}
	for _, n := range ev.notify {
		if o.OnNotify != nil {
			o.OnNotify(n[0], n[1])
		}
	}
}

// ended marks the program ended, once its output did.
func (t *Terminal) ended() {
	t.clearProgress()
	t.closeSrc()
	if t.proc != nil {
		code, _ := t.proc.Wait()
		t.code.Store(int64(code))
	}
	t.mu.Lock()
	if t.term != nil && !t.closed {
		msg := "[Process exited]"
		if code := t.code.Load(); code != 0 {
			msg = fmt.Sprintf("[Process exited with code %d]", code)
		}
		if t.opts.Conn != nil {
			msg = "[Disconnected]"
		}
		if x, _ := t.term.Cursor(); x > 0 {
			msg = "\r\n" + msg
		}
		t.term.Write([]byte("\x1b[0;2m" + msg + "\x1b[0m"))
	}
	t.mu.Unlock()
	t.finish()
}

// finish closes Done and calls OnExit, once.
func (t *Terminal) finish() {
	t.doneOnce.Do(func() {
		close(t.done)
		t.in.close()
		if t.opts.OnExit != nil {
			t.opts.OnExit(int(t.code.Load()))
		}
		t.redraw()
	})
}

// Done is closed once the program exited (or Conn ended).
func (t *Terminal) Done() <-chan struct{} { return t.done }

// ExitCode returns the exit code of the program once it exited: 128 plus
// the signal's number for a program a signal killed, -1 for one that
// could not start.
func (t *Terminal) ExitCode() int { return int(t.code.Load()) }

// Title returns the title the program set (OSC 0, OSC 2).
func (t *Terminal) Title() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.term == nil {
		return ""
	}
	return t.term.Title()
}

// Dir returns the working directory the shell reported (OSC 7, which the
// shells of macOS report), or "".
func (t *Terminal) Dir() string {
	t.mu.Lock()
	pwd := ""
	if t.term != nil {
		pwd = t.term.Pwd()
	}
	t.mu.Unlock()
	return fileURLPath(pwd)
}

// SetFont changes the font, as Options.Font.
// set them, as for zooming.
func (t *Terminal) SetFont(f Font) {
	f.Features = append([]string(nil), f.Features...)
	t.mu.Lock()
	t.opts.Font = f
	t.mu.Unlock()
	t.redraw()
}

// SetTheme changes the colors, as Options.Theme and DarkTheme.
func (t *Terminal) SetTheme(theme, dark *Theme) {
	t.mu.Lock()
	t.opts.Theme, t.opts.DarkTheme = theme, dark
	t.mu.Unlock()
	t.redraw()
}

// Size returns the size of the screen in cells.
func (t *Terminal) Size() (cols, rows int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.size.cols, t.size.rows
}

// Text returns the text of the screen and its scrollback, with the lines a
// program's long output wrapped joined.
func (t *Terminal) Text() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.term == nil {
		return ""
	}
	return t.term.FormattedTextFrom(t.term.HistoryStart(t.historyLines), true)
}

// Resize sets the size of the screen in cells, for a terminal no view
// shows, as one a server keeps for a session while no window does: a view
// sets the size of the terminal it shows.
func (t *Terminal) Resize(cols, rows int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.resize(gridSize{max(cols, 1), max(rows, 1), t.size.cellW, t.size.cellH})
}

// Snapshot returns what the terminal shows as escape sequences: its
// scrollback and screen with their styles, the cursor and the modes
// programs set. Fed to a new terminal of the same size, it shows the same,
// as a terminal attaching again to a session a server keeps.
func (t *Terminal) Snapshot() []byte {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.term == nil {
		return nil
	}
	return t.term.VT()
}

// Send sends data to the program as if typed.
func (t *Terminal) Send(data []byte) {
	if len(data) > 0 {
		t.mu.Lock()
		if t.sendInputLocked(data) {
			t.noteIdleInput(data)
		}
		t.mu.Unlock()
	}
}

// sendInputLocked serializes the submission scanner with PTY output parsing.
// Register accepted input before the writer can deliver it to the program.
func (t *Terminal) sendInputLocked(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	return t.in.pushBefore(append([]byte(nil), data...), func() {
		if t.term != nil {
			t.term.RegisterUserInput(data)
		}
	})
}

// Paste pastes text, bracketed when the program asks (mode 2004), so that
// it does not run as commands as it would typed.
func (t *Terminal) Paste(text string) { t.paste(text) }

func (t *Terminal) paste(text string) bool {
	if text == "" || len(text) > 1<<20 || strings.ContainsRune(text, 0) || !t.Available() {
		return false
	}
	t.mu.Lock()
	if t.term == nil {
		t.mu.Unlock()
		return false
	}
	bracketed := t.term.Mode(vt.ModeBracketedPaste)
	data := vt.EncodePaste(text, bracketed)
	t.term.ScrollToBottom()
	t.mu.Unlock()
	t.Send(data)
	if bracketed {
		t.idleEditing.Store(true)
	}
	t.redraw()
	return true
}

// Feed shows data as if the program wrote it: text and escape sequences.
func (t *Terminal) Feed(data []byte) { t.write(data) }

// Close closes the terminal: the program is hung up (SIGHUP), or Conn
// closed, and the terminal frees its memory. A view of it then shows
// nothing.
func (t *Terminal) Close() error {
	t.stopScrollbarFade()
	t.StopLog()
	if t.transfers != nil {
		t.transfers.cancel()
	}
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	t.closed = true
	if t.v != nil {
		t.v.cancelSemanticClick()
	}
	src, proc, waiting := t.src, t.proc, t.launch != nil
	t.launch = nil
	if t.timer != nil {
		t.timer.Stop()
	}
	t.mu.Unlock()
	var err error
	switch {
	case src != nil:
		err = t.closeSrc()
	case waiting:
		// The program never started; one starting finishes itself.
		t.code.Store(-1)
		t.finish()
	}
	if proc != nil {
		// A program that ignores the hangup is killed after a moment.
		go func() {
			select {
			case <-proc.Exited():
			case <-time.After(2 * time.Second):
				proc.Kill()
			}
		}()
	}
	t.in.close()
	// The reader may be writing to the emulator, and a frame drawing it:
	// free it under the locks.
	t.mu.Lock()
	t.rmu.Lock()
	t.term.Free()
	t.term = nil
	t.rs.Free()
	t.rs = nil
	if t.v != nil {
		t.v.graphics = nil
		t.v.graphicBitmaps = nil
	}
	t.rmu.Unlock()
	t.mu.Unlock()
	t.redraw()
	return err
}

// closeSrc closes the program's pseudo-terminal, or Conn, once.
func (t *Terminal) closeSrc() error {
	t.srcOnce.Do(func() { t.srcErr = t.src.Close() })
	return t.srcErr
}

// resizeConn tells Conn the size of the screen, from a goroutine, the
// latest size only once it told the one before.
func (t *Terminal) resizeConn(c interface{ Resize(cols, rows int) error }, s gridSize) {
	t.connMu.Lock()
	defer t.connMu.Unlock()
	t.connSize = s
	if t.connBusy {
		return
	}
	t.connBusy = true
	go func() {
		sent := gridSize{}
		for {
			t.connMu.Lock()
			s := t.connSize
			if s == sent {
				t.connBusy = false
				t.connMu.Unlock()
				return
			}
			t.connMu.Unlock()
			c.Resize(s.cols, s.rows)
			sent = s
		}
	}()
}

// resize sets the size of the screen, from the view; t.mu is held.
func (t *Terminal) resize(s gridSize) {
	if t.term == nil {
		return
	}
	if o := t.launch; o != nil {
		t.launch = nil
		t.timer.Stop()
		defer func() { go t.start(o) }() // at this size
	}
	if s == t.size {
		return
	}
	t.size = s
	t.term.Resize(s.cols, s.rows, s.cellW, s.cellH)
	t.revision++
	switch src := t.src.(type) {
	case *pty.PTY:
		src.Resize(s.cols, s.rows, s.cols*s.cellW, s.rows*s.cellH)
	case interface{ Resize(cols, rows int) error }:
		t.resizeConn(src, s)
	}
}

// renderHold starts or ends a program's synchronized update, during which
// frames show the screen as it was when it started; t.mu is held.
func (t *Terminal) renderHold(held bool) {
	t.rmu.Lock()
	if held && t.rs != nil {
		t.rs.Update(t.term)
		t.heldSince = time.Now()
	}
	t.held = held
	t.rmu.Unlock()
}

// redraw asks the view for a frame.
func (t *Terminal) redraw() {
	if t.hidden.Load() {
		return
	}
	if f := t.draw.Load(); f != nil {
		(*f)()
	}
}

// fileURLPath returns the path of a file URL, as shells report their
// directory (file://host/path), or s when it is not one.
func fileURLPath(s string) string {
	rest, ok := strings.CutPrefix(s, "file://")
	if !ok {
		return s
	}
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		rest = rest[i:]
	}
	return unescapePath(rest)
}

func unescapePath(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			if v, ok := unhex(s[i+1]); ok {
				if w, ok := unhex(s[i+2]); ok {
					b.WriteByte(v<<4 | w)
					i += 2
					continue
				}
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func unhex(c byte) (byte, bool) {
	switch {
	case '0' <= c && c <= '9':
		return c - '0', true
	case 'a' <= c && c <= 'f':
		return c - 'a' + 10, true
	case 'A' <= c && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

// inputQueue writes what is typed to the program in order, from a
// goroutine of its own: a program that does not read its input must not
// block the reader, which answers its queries.
type inputPacket struct {
	generation uint64
	data       []byte
	result     chan error
	raw        bool
	ctx        context.Context
}
type inputQueue struct {
	generation     func() uint64
	mu             sync.Mutex
	cond           *sync.Cond
	queue          []inputPacket
	closed, paused bool
	cancelPaused   context.CancelFunc
}

func (q *inputQueue) init() {
	if q.cond == nil {
		q.cond = sync.NewCond(&q.mu)
	}
}
func (q *inputQueue) push(p []byte) {
	q.pushBefore(p, nil)
}
func (q *inputQueue) pushBefore(p []byte, before func()) bool {
	var cancel context.CancelFunc
	q.mu.Lock()
	defer func() {
		q.mu.Unlock()
		// Cancel the specific session that rejected this input, outside the
		// queue lock. Do not enter transferController.mu while t.mu is held:
		// PTY output parsing takes those locks in the opposite order.
		if cancel != nil {
			cancel()
		}
	}()
	q.init()
	if !q.closed && q.paused && bytes.IndexByte(p, 3) >= 0 {
		cancel = q.cancelPaused
	}
	if !q.closed && !q.paused {
		if before != nil {
			before()
		}
		epoch := uint64(0)
		if q.generation != nil {
			epoch = q.generation()
		}
		q.queue = append(q.queue, inputPacket{data: p, generation: epoch})
		q.cond.Signal()
		return true
	}
	return false
}
func (q *inputQueue) pause(paused bool) {
	q.pauseWithCancel(paused, nil)
}

func (q *inputQueue) pauseWithCancel(paused bool, cancel context.CancelFunc) {
	q.mu.Lock()
	q.init()
	q.paused = paused
	q.cancelPaused = nil
	if paused {
		q.cancelPaused = cancel
		next := q.queue[:0]
		for _, packet := range q.queue {
			if packet.raw {
				next = append(next, packet)
			}
		}
		q.queue = next
	}
	q.mu.Unlock()
}
func (q *inputQueue) raw(ctx context.Context, p []byte) (int, error) {
	result := make(chan error, 1)
	q.mu.Lock()
	q.init()
	if q.closed {
		q.mu.Unlock()
		return 0, io.ErrClosedPipe
	}
	epoch := uint64(0)
	if q.generation != nil {
		epoch = q.generation()
	}
	q.queue = append(q.queue, inputPacket{data: append([]byte(nil), p...), result: result, raw: true, ctx: ctx, generation: epoch})
	q.cond.Signal()
	q.mu.Unlock()
	select {
	case e := <-result:
		if e != nil {
			return 0, e
		}
		return len(p), nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}
func (q *inputQueue) close() {
	q.mu.Lock()
	q.init()
	q.closed = true
	q.cancelPaused = nil
	for _, packet := range q.queue {
		if packet.result != nil {
			packet.result <- io.ErrClosedPipe
		}
	}
	q.queue = nil
	q.cond.Broadcast()
	q.mu.Unlock()
}
func (q *inputQueue) run(w io.Writer) {
	for {
		q.mu.Lock()
		q.init()
		for len(q.queue) == 0 && !q.closed {
			q.cond.Wait()
		}
		if q.closed {
			q.mu.Unlock()
			return
		}
		packet := q.queue[0]
		q.queue = q.queue[1:]
		paused := q.paused
		q.mu.Unlock()
		if !packet.raw && paused {
			continue
		}
		var err error
		if packet.ctx != nil {
			err = packet.ctx.Err()
		}
		if err == nil {
			var n int
			if writer, ok := w.(interface {
				WriteGeneration([]byte, uint64) (int, error)
			}); ok {
				n, err = writer.WriteGeneration(packet.data, packet.generation)
			} else {
				n, err = w.Write(packet.data)
			}
			if err == nil && n != len(packet.data) {
				err = io.ErrShortWrite
			}
		}
		if packet.result != nil {
			packet.result <- err
		}
		if err != nil && packet.ctx == nil {
			q.close()
			return
		}
	}
}

func vtCursor(s CursorStyle) vt.CursorStyle {
	switch s {
	case CursorBar:
		return vt.CursorBar
	case CursorUnderline:
		return vt.CursorUnderline
	}
	return vt.CursorBlock
}

// SetPreferColorEmoji invalidates rendering without changing terminal geometry.
func (t *Terminal) SetPreferColorEmoji(prefer bool) {
	t.mu.Lock()
	changed := t.opts.PreferColorEmoji != prefer
	t.opts.PreferColorEmoji = prefer
	t.mu.Unlock()
	if changed {
		t.redraw()
	}
}

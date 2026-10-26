package terminal

import (
	"math"
	"runtime"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/terminal/internal/bidilayout"
	"oshell-desktop/internal/terminal/internal/vt"
)

// View shows the terminal in a window of native UI, and takes its keyboard
// input once it has the focus, which a click gives it. Size it like any
// element, as with Fill or Grow; its size sets the terminal's, in cells of
// the font. A terminal shows in one view at a time.
//
// Copy and paste are Command+C and Command+V on macOS, which the Edit
// menu's roles also send, and Control+Shift+C and Control+Shift+V
// elsewhere. Unmodified Page Up/Down scroll a full page in legacy mode,
// outside the alternate screen and application cursor mode. Modified keys
// and Kitty keyboard mode go to the program. The wheel scrolls unless the
// program takes the mouse. Hold
// Shift to select while a program takes the mouse, and Command (Control
// elsewhere) to open a hyperlink (OSC 8) on click.
func View(c *ui.Context, t *Terminal) ui.Element {
	v := t.viewOf(c)
	pointer := ui.CursorText
	if runtime.GOOS == "windows" {
		// The thin I-beam disappears among terminal glyphs. The system arrow
		// is easier to locate and respects Windows pointer size/color settings.
		pointer = ui.CursorDefault
	}
	e := ui.Box(c).Focusable().FocusRing(false).Cursor(pointer).Clip().Label("Terminal")
	v.build(c, e)
	e.HandleInput(v.handleInput)
	e.TextCaret(v.caret()).FunctionalInput(true)
	e.Draw(v.paint)
	t.progressView(c, e)
	e.ContextMenu(v.menu)
	return e
}

// handleInput is the window input route, including availability and local actions.
func (v *view) handleInput(ev ui.InputEvent) bool {
	t := v.t
	// Lifecycle cleanup still belongs to this view while a transfer owns
	// input, or after the connection has closed. It must not be swallowed
	// by the user-input gate (nor broadcast as keyboard input below).
	if ev.Kind == ui.InputCancel || ev.Kind == ui.InputPointerCancel {
		return v.input(ev)
	}
	if ev.Kind == ui.InputPointerDown {
		t.mu.Lock()
		v.cancelSemanticClick()
		t.mu.Unlock()
	}
	// Swift gates transport bytes in send(), after local selection, scrolling,
	// clipboard and composition handling. Keep those interactions alive while
	// the queue owns protocol input, without broadcasting rejected keys.
	open, paused := t.inputStatus()
	if !open && ev.Kind != ui.InputPointerUp {
		return true
	}
	if ev.Kind == ui.InputPointerDown && ev.Button == 1 && ev.Mods&ui.Shift == 0 && t.opts.RightClickPaste {
		v.wordClicks.seed = nil
		v.paste()
		return true
	}
	wasSelecting := v.selecting
	handled := v.input(ev)
	if handled && ev.Kind == ui.InputPointerUp && ev.Button == 0 && wasSelecting && t.opts.CopyOnSelect {
		v.copy()
	}
	if handled && open && !paused && t.Available() && t.opts.OnUserInput != nil && (ev.Kind == ui.InputKeyDown || ev.Kind == ui.InputKeyUp || ev.Kind == ui.InputText) {
		t.opts.OnUserInput(ev)
	}
	return handled
}

// viewOf returns the terminal's view state, made on the first frame
// showing it.
func (t *Terminal) viewOf(c *ui.Context) *view {
	t.mu.Lock() // Close may concurrently release the view's image snapshots.
	if t.v == nil {
		t.v = &view{t: t, blinkStart: time.Now(), applied: t.curTheme}
		t.v.shaped.init(4096)
	}
	v := t.v
	t.mu.Unlock()
	if v.services != c.Services() {
		v.services = c.Services()
		draw := v.services.Invalidate
		t.draw.Store(&draw)
	}
	return v
}

// view is what a frame of the terminal needs from frame to frame. Main
// thread only.
type view struct {
	bidiView
	preferColorEmoji bool
	rules            []compiledHighlight
	highlightVersion uint64

	t        *Terminal
	services ui.Services

	theme   *Theme
	applied *Theme // the theme the emulator has

	// The grid, in device pixels: the cells, the baseline in a cell, and
	// the origin of the grid relative to the element; scale is device
	// pixels per DIP.
	font                   fontKey
	fonts                  [4]ui.Font
	scale                  float32
	cellW, cellH, baseline int
	ox, oy                 int
	cols, rows             int

	colors         vt.Colors
	cursor         vt.Cursor
	scrollbar      vt.Scrollbar
	overlay        overlayScrollbar
	lines          []rowCache
	shaped         lru
	graphics       []vt.Graphic
	graphicBitmaps map[graphicBitmapKey]*ui.Bitmap

	keys                                        *vt.KeyEncoder
	mouse                                       *vt.MouseEncoder
	gesture                                     *vt.Gesture
	gestureStart                                time.Time
	gestureTimestampSource, gestureTimestampSet bool
	wordClicks                                  wordClickState
	semanticPress                               semanticPress
	pendingSemantic                             *pendingSemanticClick // protected by t.mu

	// pending is a key that types text, until the text comes; preedit
	// is an input method's composition and its caret.
	pending        *pendingKey
	pressedAliases map[keyIdentity]KeyAlias
	composedKeys   map[keyIdentity]bool // printable AltGr presses sent without key events
	preedit        string
	preeditCaret   int
	preeditShape   preeditLayout

	focused    bool
	blinkStart time.Time
	caretCell  [2]int // the cursor's cell, for input methods
	// selecting tells that the primary button selects; reporting that a
	// press went to the program, which gets release unless a link consumes it.
	selecting, reporting     bool
	linkPressed, linkDragged bool
	linkHover                linkHoverState
	selectionRectangle       bool
	pointer                  [2]float32
	scrolled                 float32 // scrolling not yet a whole row
	ticking                  bool    // the selection scroll timer is active
	selectionScrollDelta     int
	selectionScrollNext      time.Time
	selectionDragPoint       [2]float32
}

type pendingKey struct {
	identity            keyIdentity
	alternates          bool
	shifted, baseLayout rune
	key                 vt.Key
	unshifted           rune
	mods                ui.Modifiers
	repeat              bool
	keypad              bool
}

// build updates the view as a frame builds.
func (v *view) build(c *ui.Context, e ui.Element) {
	t := v.t
	dark := c.Theme().Dark
	focused := e.Focused() && !t.hidden.Load()
	x, y, over := e.PointerPosition()
	v.requestLinkHover(x, y, over && c.Modifiers()&ui.Cmd != 0 && focused)
	t.mu.Lock()
	theme := t.opts.Theme
	if dark && t.opts.DarkTheme != nil {
		theme = t.opts.DarkTheme
	} else if theme == nil {
		theme = defaultTheme(dark)
	}
	v.theme = theme
	if t.term == nil {
		v.release()
	} else {
		if v.applied != theme {
			theme.apply(t.term)
			v.applied = theme
		}
		t.curTheme = theme
		// Keep host state current even when reporting is disabled. The native
		// parser uses it if the program enables focus reports between frames.
		t.term.SetFocus(focused)
		// Input methods compose where the cursor is now, which the last
		// frame may not show yet.
		v.caretCell = [2]int{v.cursor.X, v.cursor.Y}
		if t.term.AtBottom() {
			x, y := t.term.Cursor()
			v.caretCell = [2]int{x, y}
		}
		v.refreshBidi()
		if x, y := v.caretCell[0], v.caretCell[1]; y >= 0 && y < len(v.bidi) {
			if m := v.bidi[y].Row.LogicalToVisual; x >= 0 && x < len(m) {
				v.caretCell[0] = m[x]
			}
		}
	}
	copied := t.clipOut
	t.clipOut = nil
	t.mu.Unlock()
	for _, s := range copied {
		c.WriteClipboard(s)
	}
	if focused != v.focused {
		if focused {
			v.wordClicks.seed = nil
		}
		v.focused = focused
		v.blinkStart = c.Now()
		if !focused {
			// Input methods drop their composition with the focus.
			v.cancelInput()
		}
	}
	v.resetHiddenScrollbar()
	// The cursor blinks, every 600 ms, while the terminal has the focus.
	if v.focused && v.cursor.Blinking && v.cursor.Visible {
		since := c.Now().Sub(v.blinkStart)
		c.After(blinkPeriod - since%blinkPeriod)
	}
	if v.ticking {
		c.After(max(time.Duration(0), v.selectionScrollNext.Sub(c.Now())))
	}
}

const blinkPeriod = 600 * time.Millisecond

// release frees what the view made of the library, once the terminal is
// closed.
func (v *view) release() {
	v.cancelSemanticClick()
	v.preeditShape = preeditLayout{}
	v.bidiView = bidiView{}
	for i := range v.lines {
		v.lines[i].bidi = bidilayout.Row{}
	}
	v.overlay.hide()
	if v.keys != nil {
		v.keys.Free()
		v.keys = nil
	}
	if v.mouse != nil {
		v.mouse.Free()
		v.mouse = nil
	}
	if v.gesture != nil {
		v.gesture.Free(nil)
		v.gesture = nil
	}
}

// caret returns where input methods compose: the cursor's cell.
func (v *view) caret() ui.Rect {
	if v.scale == 0 {
		return ui.Rect{}
	}
	cw, ch := float32(v.cellW)/v.scale, float32(v.cellH)/v.scale
	if y := v.caretCell[1]; y >= 0 && y < len(v.lines) {
		cw *= lineHorizontalScale(v.lines[y].mode)
	}
	x := float32(v.ox)/v.scale + float32(v.caretCell[0])*cw
	y := float32(v.oy)/v.scale + float32(v.caretCell[1])*ch
	return ui.Rect{X: x, Y: y, W: cw, H: ch}
}

// menu is the terminal's context menu.
func (v *view) menu(m *ui.Menu) {
	t := v.t
	cmd := ui.Cmd
	if runtime.GOOS != "darwin" {
		cmd = ui.Ctrl | ui.Shift
	}
	t.mu.Lock()
	text, selected := "", false
	if t.term != nil {
		text, selected = t.term.SelectionText()
	}
	t.mu.Unlock()
	copyItem := m.Item("Copy").Disabled(!selected || text == "")
	if !t.opts.ManagedClipboardBindings {
		copyItem.Shortcut(cmd, ui.KeyC)
	}
	if copyItem.Chosen() {
		v.services.WriteClipboard(v.t.cleanCopy(text))
	}
	pasteItem := m.Item("Paste")
	if !t.opts.ManagedClipboardBindings {
		pasteItem.Shortcut(cmd, ui.KeyV)
	}
	if pasteItem.Chosen() {
		v.paste()
	}
	if m.Item("Select All").Chosen() {
		v.selectAll()
	}
	m.Separator()
	if m.Item("Clear Scrollback").Chosen() {
		t.Feed([]byte("\x1b[3J"))
	}
}

// input handles the input of the view as it comes.
func (v *view) input(ev ui.InputEvent) bool {
	if ev.Kind == ui.InputKeyDown || ev.Kind == ui.InputKeyUp {
		ev = v.aliasKeyEvent(ev)
	}
	switch ev.Kind {
	case ui.InputCancel:
		v.cancelInput()
		return true
	case ui.InputKeyDown:
		return v.keyDown(ev)
	case ui.InputKeyUp:
		return v.keyUp(ev)
	case ui.InputText:
		if ev.Literal {
			v.pending = nil
			v.preedit = ""
			v.preeditShape = preeditLayout{}
			v.sendText(ev.Text)
			return true
		}
		v.typed(ev.Text)
		return true
	case ui.InputCompose:
		v.preedit, v.preeditCaret, v.pending = ev.Text, ev.Caret, nil
		if ev.Text == "" {
			v.preeditShape = preeditLayout{}
		}
		return true
	case ui.InputCommand:
		switch ev.Text {
		case "copy":
			v.copy()
		case "paste":
			v.paste()
		case "selectAll":
			v.selectAll()
		default:
			return false
		}
		return true
	case ui.InputPointerDown, ui.InputPointerUp, ui.InputPointerMove, ui.InputPointerCancel:
		return v.pointerEvent(ev)
	case ui.InputScroll:
		v.wordClicks = wordClickState{}
		return v.scroll(ev)
	}
	return false
}

func (v *view) cancelInput() {
	v.preedit, v.preeditCaret, v.pending = "", 0, nil
	v.preeditShape = preeditLayout{}
	v.pressedAliases = nil
	v.composedKeys = nil
	v.linkPressed, v.linkDragged = false, false
	v.linkHover.enabled, v.linkHover.dirty = false, true
}

func (v *view) keyDown(ev ui.InputEvent) bool {
	v.wordClicks.seed = nil
	t := v.t
	mac := runtime.GOOS == "darwin"
	mods, key := ev.Mods, ev.Key
	copyMods := ui.Ctrl | ui.Shift
	if mac {
		copyMods = ui.Super
	}
	switch {
	case !t.opts.ManagedClipboardBindings && mods == copyMods && key == ui.KeyC:
		v.copy()
		return true
	case !t.opts.ManagedClipboardBindings && mods == copyMods && key == ui.KeyV:
		v.paste()
		return true
	case !ev.Keypad && mods == 0 && (key == ui.KeyPageUp || key == ui.KeyPageDown) && v.scrollPage(key == ui.KeyPageUp):
		return true
	case mods&ui.Super != 0:
		return false // the app's shortcuts
	}
	k, unshifted := vtEventKey(ev)
	if k == vt.KeyUnidentified && ev.Unshifted == 0 {
		return false
	}
	delete(v.composedKeys, keyIdentity{ev.Key, ev.Keypad, ev.Unshifted})
	if ev.Unshifted != 0 {
		if ev.TextPending && (mods&(ui.Ctrl|ui.Alt) == 0 || runtime.GOOS == "windows" && mods&(ui.Ctrl|ui.Alt) == ui.Ctrl|ui.Alt) {
			v.pending = &pendingKey{identity: keyIdentity{ev.Key, ev.Keypad, ev.Unshifted}, key: k, unshifted: unshifted, mods: mods, repeat: ev.Repeat, alternates: true, shifted: ev.Shifted, baseLayout: ev.BaseLayout}
			return true
		}
		v.pending = nil
		action := vt.KeyPress
		if ev.Repeat {
			action = vt.KeyRepeat
		}
		v.sendKey(vt.KeyEvent{Action: action, Key: k, Mods: vtMods(mods), Text: ev.Text, Unshifted: unshifted, Alternates: true, Shifted: ev.Shifted, BaseLayout: ev.BaseLayout})
		return true
	}
	altMeta := mods&ui.Alt != 0 && (!mac || t.opts.OptionAsAlt)
	// AltGr types text on Windows, where it is Control and Alt.
	altGr := runtime.GOOS == "windows" && mods&(ui.Ctrl|ui.Alt) == ui.Ctrl|ui.Alt
	if typesText(key) && (ev.Keypad || mods&(ui.Ctrl|ui.Alt) == 0 || mods&ui.Alt != 0 && !altMeta && mods&ui.Ctrl == 0 || altGr) {
		// The text it types comes next.
		v.pending = &pendingKey{identity: keyIdentity{ev.Key, ev.Keypad, ev.Unshifted}, key: k, unshifted: unshifted, mods: mods, repeat: ev.Repeat, keypad: ev.Keypad}
		return true
	}
	v.pending = nil
	text := ""
	if altMeta && typesText(key) && mods&ui.Ctrl == 0 {
		// Alt as Meta: the character, which the encoder prefixes with Esc.
		text = string(unshifted)
		if mods&ui.Shift != 0 && !ev.Keypad {
			text = shiftedText(key, unshifted)
		}
	}
	action := vt.KeyPress
	if ev.Repeat {
		action = vt.KeyRepeat
	}
	v.sendKey(vt.KeyEvent{Action: action, Key: k, Mods: vtMods(mods), Text: text, Unshifted: unshifted})
	return true
}

// scrollPage follows OShell's ordinary page-key route. Even at a scrollback
// boundary it consumes the key locally; it must not leak bytes to the shell.
func (v *view) scrollPage(up bool) bool {
	t := v.t
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.term == nil || t.term.AltScreen() || t.term.Mode(vt.ModeCursorKeys) {
		return false
	}
	if flags, ok := t.term.KeyboardFlags(); !ok || flags != 0 {
		return false
	}
	_, rows := t.term.Size()
	page := max(rows, 1)
	if up {
		page = -page
	}
	before := t.term.Scrollbar().Offset
	t.term.ScrollBy(page)
	t.term.SetSelection(nil)
	v.wordClicks = wordClickState{}
	if t.term.Scrollbar().Offset != before {
		v.scrollingActivity()
	}
	return true
}

func (v *view) keyUp(ev ui.InputEvent) bool {
	k, unshifted := vtEventKey(ev)
	if v.composedKeys[keyIdentity{ev.Key, ev.Keypad, ev.Unshifted}] {
		delete(v.composedKeys, keyIdentity{ev.Key, ev.Keypad, ev.Unshifted})
		return true
	}
	if k == vt.KeyUnidentified && ev.Unshifted == 0 || ev.Mods&ui.Super != 0 {
		return false
	}
	if p := v.pending; p != nil && p.identity == (keyIdentity{ev.Key, ev.Keypad, ev.Unshifted}) {
		// No text came, as for Control, Alt and a letter on Windows,
		// where they may type one: the key alone.
		v.pending = nil
		action := vt.KeyPress
		if p.repeat {
			action = vt.KeyRepeat
		}
		text := ""
		if !p.alternates && (p.mods&ui.Ctrl == 0 || p.keypad) {
			text = string(p.unshifted) // what the key types on a US keyboard
			if p.mods&ui.Shift != 0 && !p.keypad {
				text = shiftedText(ev.Key, p.unshifted)
			}
		}
		v.sendKey(vt.KeyEvent{Action: action, Key: p.key, Mods: vtMods(p.mods), Text: text, Unshifted: p.unshifted, Alternates: p.alternates, Shifted: p.shifted, BaseLayout: p.baseLayout})
	}
	// The desktop view, unlike the low-level encoder, reports an ordinary
	// Unicode release only for report-all or a held Ctrl/Meta modifier.
	// Keep returning semantic success so synchronized targets decide using
	// their own current keyboard flags.
	if !ev.Keypad && ev.Key != ui.KeyAdd && ev.Key != ui.KeyMultiply && (typesText(ev.Key) || ev.Unshifted != 0) && ev.Mods&ui.Ctrl == 0 && (ev.Mods&ui.Alt == 0 || runtime.GOOS == "darwin" && !v.t.opts.OptionAsAlt) {
		v.t.mu.Lock()
		all := false
		if v.t.term != nil {
			flags, ok := v.t.term.KeyboardFlags()
			all = ok && flags&8 != 0
		}
		v.t.mu.Unlock()
		if !all {
			return true
		}
	}
	v.encodeKey(vt.KeyEvent{Action: vt.KeyRelease, Key: k, Mods: vtMods(ev.Mods), Unshifted: unshifted, Alternates: ev.Unshifted != 0, Shifted: ev.Shifted, BaseLayout: ev.BaseLayout}, false)
	// A recognized release is still semantic input when the local legacy
	// protocol emits no bytes. Synchronized targets can independently have
	// Kitty event reporting enabled and must receive the matching release.
	return true
}

// typed sends text typed or committed by an input method.
func (v *view) typed(text string) {
	v.preedit = ""
	v.preeditShape = preeditLayout{}
	p := v.pending
	v.pending = nil
	if text == "" {
		return
	}
	if runtime.GOOS == "windows" && p != nil && !p.keypad && p.mods&(ui.Ctrl|ui.Alt) == ui.Ctrl|ui.Alt && printableComposition(text) {
		// Windows reports AltGr as Ctrl+Alt. Only a translated printable
		// commit proves that these modifiers produced text. Preserve that
		// text even if Kitty report-all is enabled without associated text,
		// matching OShell's Option-composition exception. A combination
		// with no text still follows keyUp's ordinary shortcut fallback.
		if v.composedKeys == nil {
			v.composedKeys = make(map[keyIdentity]bool)
		}
		v.composedKeys[p.identity] = true
		v.sendText(text)
		return
	}
	if p == nil || !p.keypad && utf8.RuneCountInString(text) > 1 {
		// A composition commit has no hardware key identity. Let the encoder
		// honor report-all/associated-text instead of bypassing the protocol.
		v.sendKey(vt.KeyEvent{Action: vt.KeyPress, Key: vt.KeyUnidentified, Text: text})
		return
	}
	var consumed vt.Mods
	r, _ := utf8.DecodeRuneInString(text)
	if r != p.unshifted && !p.keypad {
		// Shift or Option made the character.
		consumed = vtMods(p.mods & (ui.Shift | ui.Alt))
	}
	action := vt.KeyPress
	if p.repeat {
		action = vt.KeyRepeat
	}
	v.sendKey(vt.KeyEvent{Action: action, Key: p.key, Mods: vtMods(p.mods), Consumed: consumed, Text: text, Unshifted: p.unshifted, Alternates: p.alternates, Shifted: p.shifted, BaseLayout: p.baseLayout})
}

func printableComposition(text string) bool {
	if text == "" || !utf8.ValidString(text) {
		return false
	}
	for _, r := range text {
		if r < 0x20 || r >= 0x7f && r <= 0x9f {
			return false
		}
	}
	return true
}

// sendKey sends a key, scrolling to the bottom and clearing the selection
// as Ghostty does when it sends something.
func (v *view) sendKey(k vt.KeyEvent) {
	if v.encodeKey(k, true) {
		v.blinkStart = time.Now()
	}
}

func (v *view) encodeKey(k vt.KeyEvent, press bool) bool {
	t := v.t
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.term == nil {
		return false
	}
	if v.keys == nil {
		var err error
		if v.keys, err = vt.NewKeyEncoder(); err != nil {
			return false
		}
	}
	v.keys.Sync(t.term, t.opts.OptionAsAlt)
	// Match OShell's desktop event route: physical keys keep their identity
	// through the encoder, including in RTL paragraphs and mode 1243.
	b := v.keys.Encode(k)
	if len(b) == 0 {
		return false
	}
	if accepted := t.sendInputLocked(b); accepted && press {
		t.noteIdleInput(b)
	}
	if press {
		v.wordClicks = wordClickState{}
		t.term.ScrollToBottom()
		t.term.SetSelection(nil)
	}
	return true
}

// sendText sends text that is no key, as of an input method.
func (v *view) sendText(text string) {
	v.wordClicks = wordClickState{}
	t := v.t
	t.mu.Lock()
	if t.term != nil {
		t.term.ScrollToBottom()
		t.term.SetSelection(nil)
	}
	data := []byte(text)
	if t.sendInputLocked(data) {
		t.noteIdleInput(data)
	}
	t.mu.Unlock()
	v.blinkStart = time.Now()
}

func (v *view) copy() {
	t := v.t
	t.mu.Lock()
	text, ok := "", false
	if t.term != nil {
		text, ok = t.term.SelectionText()
	}
	t.mu.Unlock()
	if ok && text != "" {
		v.services.WriteClipboard(v.t.cleanCopy(text))
	}
}

func (v *view) paste() {
	if hook := v.t.opts.BeforeClipboardPaste; hook != nil && hook() {
		return
	}
	if text := v.services.ReadClipboard(); text != "" {
		v.t.RequestPaste(text)
	}
}

func (v *view) selectAll() {
	t := v.t
	t.mu.Lock()
	if t.term != nil {
		if s, ok := t.term.SelectHistoryFrom(t.term.HistoryStart(t.historyLines)); ok {
			t.term.SetSelection(&s)
		}
	}
	t.mu.Unlock()
	if t.opts.CopyOnSelect {
		v.copy()
	}
	v.services.Invalidate()
}

// cellAt returns the cell under a point of the element, in DIPs, within
// the grid.
func (v *view) cellAt(x, y float32) (col, row int) {
	if v.cellW == 0 {
		return 0, 0
	}
	row = int(math.Floor(float64((y*v.scale - float32(v.oy)) / float32(v.cellH))))
	row = max(0, min(row, v.rows-1))
	col = int(math.Floor(float64((x*v.scale - float32(v.ox)) / float32(v.inputCellWidth(row)))))
	return v.logicalColumn(row, max(0, min(col, v.cols-1))), row
}

// pointerEvent reports the pointer to the program that takes the mouse, or
// selects.
func (v *view) pointerEvent(ev ui.InputEvent) bool {
	t := v.t
	v.pointer = [2]float32{ev.X, ev.Y}
	t.mu.Lock()
	defer t.mu.Unlock()
	if ev.Kind == ui.InputPointerUp && t.term == nil {
		ev.Kind = ui.InputPointerCancel
	}
	if ev.Kind == ui.InputPointerCancel {
		v.cancelSemanticClick()
		v.semanticPress = semanticPress{}
		// A valid grid is needed only to encode the remote release. The
		// normal queue still rejects it during protocol transfer/closure.
		if v.reporting && t.term != nil && v.cellW > 0 && v.cellH > 0 && v.scale > 0 {
			v.report(ui.InputEvent{Kind: ui.InputPointerUp, Button: ev.Button, Mods: ev.Mods, X: ev.X, Y: ev.Y})
		}
		v.selecting, v.reporting, v.overlay.dragging = false, false, false
		v.linkPressed, v.linkDragged = false, false
		v.wordClicks = wordClickState{}
		v.setSelectionAutoscroll(0, time.Time{})
		if v.gesture != nil && t.term != nil {
			v.gesture.Release(t.term, nil)
		}
		return true
	}
	if t.term == nil || v.cellW == 0 {
		return false
	}
	if ev.Kind == ui.InputPointerDown {
		v.beginSemanticPress(ev)
	}
	if ev.Kind == ui.InputPointerUp && ev.Button == 0 {
		defer func() { v.semanticPress.eligible = false }()
	}
	if v.scrollbarInput(ev) {
		v.wordClicks = wordClickState{}
		v.linkPressed, v.linkDragged = false, false
		return true
	}
	switch ev.Kind {
	case ui.InputPointerDown:
		if ev.Button == 0 {
			v.linkPressed, v.linkDragged = true, false
		}
	case ui.InputPointerMove:
		if v.linkPressed {
			v.linkDragged = true
		}
	case ui.InputPointerUp:
		if ev.Button == 0 {
			v.requestLinkRelease(ev.X, ev.Y, ev.Mods&ui.Cmd != 0)
			activate := v.linkPressed && !v.linkDragged && ev.Mods&ui.Cmd != 0
			v.linkPressed, v.linkDragged = false, false
			if activate {
				col, row := v.cellAt(ev.X, ev.Y)
				if match := v.linkMatchAt(col, row); match.text != "" || match.explicit {
					link := match.clickURL()
					// Swift resolves links at release, before mouse reporting.
					// A dragged gesture must never reach the external opener.
					v.selecting, v.ticking, v.reporting = false, false, false
					if v.gesture != nil {
						ref, ok := t.term.CellAt(col, row)
						if ok {
							v.gesture.Release(t.term, &ref)
						} else {
							v.gesture.Release(t.term, nil)
						}
					}
					if terminalLinkAllowed(link) {
						v.services.OpenURL(link)
					}
					return true
				}
			}
		}
	}
	tracking := t.term.MouseTracking() && ev.Mods&ui.Shift == 0
	if tracking && !v.selecting || v.reporting {
		v.wordClicks = wordClickState{}
		return v.report(ev)
	}
	if v.gesture == nil {
		g, err := vt.NewGesture(uint64(500*time.Millisecond), 5*float64(v.scale))
		if err != nil {
			return false
		}
		v.gesture = g
		v.gestureStart = time.Now()
	}
	col, row := v.cellAt(ev.X, ev.Y)
	px, py := float64(ev.X*v.scale), float64(ev.Y*v.scale)
	switch ev.Kind {
	case ui.InputPointerDown:
		if ev.Button != 0 {
			v.wordClicks = wordClickState{}
			return false // the context menu
		}
		ref, ok := t.term.CellAt(col, row)
		if !ok {
			return false
		}
		v.selecting = true
		v.semanticPress.eligible = true
		ns, changedClock := v.pointerTimestamp(ev)
		var selection vt.Selection
		var selected bool
		if t.opts.CustomWordSelection {
			selection, selected = v.wordPress(ref, ev, px, py, ns)
		} else if changedClock {
			selection, selected = v.gesture.PressCountWithShift(t.term, ref, px, py, ns, 1, ev.Mods&ui.Shift != 0)
		} else {
			selection, selected = v.gesture.PressWithShift(t.term, ref, px, py, ns, ev.Mods&ui.Shift != 0)
		}
		if selected {
			t.term.SetSelection(&selection)
		} else {
			t.term.SetSelection(nil)
		}
		return true
	case ui.InputPointerMove:
		if !v.selecting {
			return false
		}
		v.semanticPress.dragged = true
		v.selectionRectangle = ev.Mods&ui.Alt != 0
		ref, ok := t.term.CellAt(col, row)
		if !v.selectionRectangle {
			ref, row, ok = v.selectionCellAt(ev.X, ev.Y)
		}
		if !ok {
			return true
		}
		// SelectionService establishes its soft start on the first drag,
		// including a new drag after scrolling has invalidated the old range.
		// An active empty/out-of-storage selection must still keep its anchor.
		if !v.selectionRectangle && !t.term.SelectionActive() {
			v.gesture.PressCount(t.term, ref, px, py, uint64(time.Since(v.gestureStart)), 1)
		}
		if s, ok := v.gesture.Drag(t.term, ref, px, py, v.geometry(row), v.selectionRectangle); ok {
			t.term.SetSelection(&s)
		} else if v.gesture.Anchored(t.term) {
			t.term.SetSelection(nil)
		}
		v.wordClicks = wordClickState{}
		v.selectionDragPoint = [2]float32{ev.X, ev.Y}
		delta := 0
		if v.selectionRectangle {
			delta = v.gesture.Autoscroll(t.term)
		} else if t.term.SelectionActive() {
			delta = selectionScrollVelocity(row, v.rows)
		}
		v.setSelectionAutoscroll(delta, time.Now())
		return true
	case ui.InputPointerUp:
		if !v.selecting || ev.Button != 0 {
			return false
		}
		v.selecting = false
		v.setSelectionAutoscroll(0, time.Time{})
		ref, ok := t.term.CellAt(col, row)
		if ok {
			v.gesture.Release(t.term, &ref)
		} else {
			v.gesture.Release(t.term, nil)
		}
		v.deferSemanticClick(col, row, ev.Mods)
		return true
	}
	return false
}

// geometry is the grid in pixels, for selection gestures.
func (v *view) geometry(row int) vt.Geometry {
	return vt.Geometry{Columns: v.cols, CellWidth: v.inputCellWidth(row), PadLeft: v.ox, Height: v.oy + v.rows*v.cellH}
}

// autoscroll scrolls a selection dragged past the top or the bottom; t.mu
// is held.
func (v *view) autoscroll() {
	t := v.t
	if v.selectionScrollDelta == 0 || !v.selecting {
		v.setSelectionAutoscroll(0, time.Time{})
		return
	}
	t.term.ScrollBy(v.selectionScrollDelta)
	v.scrollingActivity()
	// Swift's timer keeps moving the viewport after selection cancellation;
	// only extending the range depends on an active selection.
	if !t.term.SelectionActive() || !v.gesture.Anchored(t.term) {
		return
	}
	x, y := v.selectionDragPoint[0], v.selectionDragPoint[1]
	col, row := v.cellAt(x, y)
	ref, ok := t.term.CellAt(col, row)
	if !v.selectionRectangle {
		ref, row, ok = v.selectionCellAt(x, y)
	}
	if !ok {
		return
	}
	// Scroll once, then map the pointer against the NEW viewport. Tick also
	// scrolls natively, which used to advance twice and apply a stale BiDi map.
	if s, ok := v.gesture.Drag(t.term, ref, float64(x*v.scale), float64(y*v.scale), v.geometry(row), v.selectionRectangle); ok {
		t.term.SetSelection(&s)
	} else if v.gesture.Anchored(t.term) {
		t.term.SetSelection(nil)
	}
}

// report reports the pointer to the program; t.mu is held.
func (v *view) report(ev ui.InputEvent) bool {
	t := v.t
	if v.mouse == nil {
		m, err := vt.NewMouseEncoder()
		if err != nil {
			return false
		}
		v.mouse = m
	}
	button := []int{vt.ButtonLeft, vt.ButtonRight, vt.ButtonMiddle}
	var action vt.MouseAction
	b := vt.ButtonNone
	switch ev.Kind {
	case ui.InputPointerDown:
		action = vt.MousePress
		v.reporting = true
	case ui.InputPointerUp:
		action = vt.MouseRelease
		v.reporting = false
	default:
		action = vt.MouseMotion
	}
	if ev.Button >= 0 && ev.Button < len(button) {
		b = button[ev.Button]
	}
	pressed := v.reporting
	_, row := v.cellAt(ev.X, ev.Y)
	v.mouse.Sync(t.term, v.ox*2+v.cols*v.cellW, v.oy*2+v.rows*v.cellH, v.inputCellWidth(row), v.cellH, v.ox, v.oy, pressed)
	px, py := v.mousePosition(ev.X, ev.Y)
	out := v.mouse.Encode(action, b, vtMods(ev.Mods), px, py)
	if len(out) > 0 {
		t.sendInputLocked(out)
	}
	return ev.Kind != ui.InputPointerMove || len(out) > 0
}

// scroll scrolls the scrollback, or reports the wheel to the program.
func (v *view) scroll(ev ui.InputEvent) bool {
	t := v.t
	if v.cellH == 0 {
		return false
	}
	ch := float32(v.cellH) / v.scale
	dy := ev.DY
	if !ev.Precise {
		dy = dy / 40 * 3 * ch // a notch scrolls three rows
	}
	v.scrolled += dy
	rows := int(v.scrolled / ch)
	v.scrolled -= float32(rows) * ch
	if rows == 0 {
		return true
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.term == nil {
		return false
	}
	switch {
	case t.term.MouseTracking() && ev.Mods&ui.Shift == 0:
		if v.mouse == nil {
			m, err := vt.NewMouseEncoder()
			if err != nil {
				return false
			}
			v.mouse = m
		}
		_, row := v.cellAt(ev.X, ev.Y)
		v.mouse.Sync(t.term, v.ox*2+v.cols*v.cellW, v.oy*2+v.rows*v.cellH, v.inputCellWidth(row), v.cellH, v.ox, v.oy, false)
		button := vt.WheelDown
		if rows < 0 {
			button = vt.WheelUp
		}
		px, py := v.mousePosition(ev.X, ev.Y)
		for range abs(rows) {
			if out := v.mouse.Encode(vt.MousePress, button, vtMods(ev.Mods), px, py); len(out) > 0 {
				t.sendInputLocked(out)
			}
		}
	case t.term.AltScreen() && t.term.Mode(vt.ModeAltScroll):
		// Full-screen programs scroll with the arrows (mode 1007).
		if v.keys == nil {
			var err error
			if v.keys, err = vt.NewKeyEncoder(); err != nil {
				return false
			}
		}
		v.keys.Sync(t.term, false)
		key := vt.KeyArrowDown
		if rows < 0 {
			key = vt.KeyArrowUp
		}
		for range abs(rows) {
			t.sendInputLocked(v.keys.Encode(vt.KeyEvent{Action: vt.KeyPress, Key: key}))
		}
	default:
		t.term.ScrollBy(rows)
		v.scrollingActivity()
	}
	return true
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// shiftedText returns what a key types with Shift on a US keyboard.
func shiftedText(k ui.Key, unshifted rune) string {
	for r, key := range shifted {
		if key == k {
			return string(r)
		}
	}
	if 'a' <= unshifted && unshifted <= 'z' {
		return string(unshifted - 'a' + 'A')
	}
	return string(unshifted)
}

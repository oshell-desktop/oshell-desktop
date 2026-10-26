package platform

import (
	"unicode/utf16"
)

// TextRange is a half-open range in UTF-16 code units of a text client's
// document. Native input protocols use these units, independently of UTF-8
// storage or the client's choice of buffer.
type TextRange struct{ Start, End int }

// TextSelection is the primary selection exposed to input methods. A client
// may own additional selections. Reversed puts the caret at Range.Start.
type TextSelection struct {
	Range    TextRange
	Reversed bool
}

func (s TextSelection) Caret() int {
	if s.Reversed {
		return s.Range.Start
	}
	return s.Range.End
}

// TextInputClient supplies a focused custom element's text to native input
// methods. All calls are synchronous on the main thread. Ranges are absolute
// UTF-16 document offsets, including marked text. Nil replacement ranges mean
// the marked range, if present, otherwise the selection.
type TextInputClient interface {
	TextForRange(TextRange) (string, TextRange)
	Selection() TextSelection
	MarkedRange() (TextRange, bool)
	ReplaceText(*TextRange, string)
	SetMarkedText(*TextRange, string, TextRange)
	UnmarkText()
	BoundsForRange(TextRange) (RectF, TextRange, bool)
	IndexForPoint(x, y float64) (int, bool)
}

// InputComposition preserves native end-preedit/commit ordering. A remembered
// range is usable only while its client and surrounding document stay unchanged.
type InputComposition struct {
	pending   *endedComposition
	epoch     uint64
	canceling bool
}

type endedComposition struct {
	client      TextInputClient
	commit      func(string) bool
	rangeWanted TextRange
	context     TextContext
	selection   TextSelection
	text        string
	actual      TextRange
}

func (c *InputComposition) Reset() { c.pending = nil; c.epoch++ }

// CancelNative isolates synchronous callbacks from an input method's reset.
// GTK XIM may emit commit during reset; it is not a user-confirmed result.
func (c *InputComposition) CancelNative(cancel func()) {
	c.Reset()
	if c.canceling {
		return
	}
	c.canceling = true
	defer func() {
		c.Reset()
		c.canceling = false
	}()
	cancel()
}

func (c *InputComposition) Canceling() bool { return c.canceling }

// Start separates a new native sequence from a cancelled empty preview.
// Empty updates inside one sequence must not create additional undo steps.
func (c *InputComposition) Start(client TextInputClient) {
	if c.canceling {
		return
	}
	c.Reset()
	if owner, ok := client.(interface{ BeginMarkedText() }); ok {
		owner.BeginMarkedText()
	}
}

// Preedit delivers native composition text. The caret is a Unicode character
// offset, while the client interface uses UTF-16 offsets.
func (c *InputComposition) Preedit(client TextInputClient, r *TextRange, text string, cursor int) {
	if c.canceling {
		return
	}
	c.Reset()
	if text == "" {
		if _, marked := client.MarkedRange(); !marked {
			// Cancellation must not replace an unrelated selection.
			return
		}
	}
	caret := 0
	for _, ch := range text {
		if cursor <= 0 {
			break
		}
		caret += utf16.RuneLen(ch)
		cursor--
	}
	client.SetMarkedText(r, text, TextRange{Start: caret, End: caret})
}

func (c *InputComposition) End(client TextInputClient) {
	if c.canceling {
		return
	}
	r, marked := client.MarkedRange()
	if !marked {
		// Some IMEs end twice before committing. Keep the first range only
		// for Replace to validate. Dropping it here would redirect a stale
		// commit to the new selection. No second unmark call is needed.
		return
	}
	c.Reset()
	epoch := c.epoch
	// Built-in editors can retain a private range and revision, including
	// password controls whose native text queries deliberately hide offsets.
	if owner, ok := client.(interface {
		EndMarkedText() (func(string) bool, bool)
	}); ok {
		if commit, handled := owner.EndMarkedText(); handled {
			if commit == nil {
				commit = func(string) bool { return false }
			}
			if c.epoch == epoch {
				c.pending = &endedComposition{client: client, commit: commit}
			}
			return
		}
	}
	client.UnmarkText()
	if c.epoch != epoch {
		return
	}
	if _, marked := client.MarkedRange(); marked {
		return
	}
	context := ClientTextContext(client)
	text, actual := client.TextForRange(r)
	selection := client.Selection()
	if c.epoch == epoch {
		c.pending = &endedComposition{client: client, rangeWanted: r, context: context, text: text, actual: actual, selection: selection}
	}
}

// Replace reports whether this result was delivered. A stale implicit result
// is dropped, not redirected to the client's new selection. Explicit ranges
// remain independent operations and discard any previous pending range.
func (c *InputComposition) Replace(client TextInputClient, r *TextRange, text string) bool {
	if c.canceling {
		return false
	}
	pending := c.pending
	c.Reset()
	if r == nil && pending != nil {
		if pending.commit != nil {
			return pending.client == client && pending.commit(text)
		}
		epoch := c.epoch
		if !pending.matches(client) || c.epoch != epoch {
			return false
		}
		r = &pending.rangeWanted
	}
	client.ReplaceText(r, text)
	return true
}

func (p *endedComposition) matches(client TextInputClient) bool {
	if p.client != client || p.selection != client.Selection() || p.context != ClientTextContext(client) {
		return false
	}
	// The pending preedit can be longer than the surrounding-text window.
	// Check the entire replacement range as well, without reading the whole
	// document or exposing it to the native input method.
	text, actual := client.TextForRange(p.rangeWanted)
	return p.text == text && p.actual == actual
}

// UTF16Len counts native text offsets without allocating an encoded string.
func UTF16Len(s string) (n int) {
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	return
}

// UTF16ByteOffset rounds an offset inside a surrogate pair down to its rune.
func UTF16ByteOffset(s string, offset int) int {
	for i, r := range s {
		if offset < utf16.RuneLen(r) {
			return i
		}
		offset -= utf16.RuneLen(r)
	}
	return len(s)
}

func NormalizeTextRange(r TextRange) TextRange {
	r.Start, r.End = max(0, min(r.Start, r.End)), max(0, max(r.Start, r.End))
	return r
}

// TextContext is a bounded window of a client document, with preedit removed
// as GTK and IMM32 surrounding-text protocols expect. Positions in Text are
// UTF-16 units; Range maps them back to the client's document.
type TextContext struct {
	Text       string
	Start, End int
	Caret      int
	Base       int
	mark       TextRange
	hasMark    bool
}

func ClientTextContext(client TextInputClient) TextContext {
	return clientTextContext(client, 1024, 1024)
}

func clientTextContext(client TextInputClient, before, after int) TextContext {
	sel := client.Selection()
	r := NormalizeTextRange(sel.Range)
	mark, marked := client.MarkedRange()
	if marked {
		mark = NormalizeTextRange(mark)
		r.Start, r.End = min(r.Start, mark.Start), max(r.End, mark.End)
	}
	// A selection may cover a huge document. Query a bounded window around
	// its active end; full text remains available through TextForRange.
	caret := max(0, sel.Caret())
	if marked {
		caret = mark.Start
	}
	wanted := TextRange{Start: max(0, caret-before), End: caret + after}
	if marked {
		wanted.Start, wanted.End = min(wanted.Start, mark.Start), max(wanted.End, mark.End)
	}
	text, actual := client.TextForRange(wanted)
	actual = NormalizeTextRange(actual)
	c := TextContext{Text: text, Base: actual.Start, mark: mark, hasMark: marked && mark.Start >= actual.Start && mark.End <= actual.End}
	if c.hasMark {
		a, z := UTF16ByteOffset(text, mark.Start-actual.Start), UTF16ByteOffset(text, mark.End-actual.Start)
		c.Text = text[:a] + text[z:]
	}
	local := func(i int) int {
		if c.hasMark {
			if i >= mark.End {
				i -= mark.End - mark.Start
			} else if i > mark.Start {
				i = mark.Start
			}
		}
		return max(0, min(i-c.Base, UTF16Len(c.Text)))
	}
	c.Start, c.End = local(r.Start), local(r.End)
	c.Caret = local(sel.Caret())
	if marked {
		c.Start, c.End = local(mark.Start), local(mark.Start)
		c.Caret = c.Start
	}
	return c
}

// Range maps a range of the surrounding text to the actual document. A
// collapsed position at preedit stays collapsed; an explicit range crossing
// it includes preedit, so replacement remains one client operation.
func (c TextContext) Range(r TextRange) TextRange {
	r = NormalizeTextRange(r)
	n := UTF16Len(c.Text)
	r.Start, r.End = min(r.Start, n)+c.Base, min(r.End, n)+c.Base
	if c.hasMark {
		if r.Start > c.mark.Start {
			r.Start += c.mark.End - c.mark.Start
		}
		if r.End > c.mark.Start {
			r.End += c.mark.End - c.mark.Start
		}
	}
	return r
}

// DeleteRange maps GTK character offsets around the selection's end, rather
// than confusing Unicode characters with UTF-16 or UTF-8 offsets.
func (c TextContext) DeleteRange(offset, count int) TextRange {
	caretByte := UTF16ByteOffset(c.Text, c.Caret)
	caretRune := len([]rune(c.Text[:caretByte]))
	runes := []rune(c.Text)
	a := max(0, min(caretRune+offset, len(runes)))
	z := max(a, min(a+max(count, 0), len(runes)))
	return c.Range(TextRange{Start: UTF16Len(string(runes[:a])), End: UTF16Len(string(runes[:z]))})
}

// ClientDeleteRange can query beyond the usual surrounding window when an
// input method asks to replace more text. GTK offsets count Unicode runes.
func ClientDeleteRange(client TextInputClient, offset, count int) TextRange {
	before, after := max(0, -offset)*2, max(0, offset+max(count, 0))*2
	return clientTextContext(client, before, after).DeleteRange(offset, count)
}

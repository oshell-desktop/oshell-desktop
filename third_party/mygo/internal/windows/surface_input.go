//go:build windows && (amd64 || arm64)

package windows

import (
	"slices"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"

	"github.com/egoist/mygo/internal/platform"
)

// Input methods ask for the text around the caret with WM_IME_REQUEST:
// IMR_DOCUMENTFEED to convert in context, and IMR_RECONVERTSTRING to
// convert again what was typed, whose range IMR_CONFIRMRECONVERTSTRING
// then settles. The composition that follows replaces that range.

const (
	wmImeRequest              = 0x0288
	imrReconvertString        = 0x0004
	imrConfirmReconvertString = 0x0005
	imrDocumentFeed           = 0x0007
)

// reconvertString is RECONVERTSTRING, which the string follows in memory.
// Lengths count UTF-16 units; offsets, bytes from the string's start.
type reconvertString struct {
	Size, Version, StrLen, StrOffset, CompStrLen, CompStrOffset, TargetStrLen, TargetStrOffset uint32
}

// imeRequest answers WM_IME_REQUEST.
func (s *surface) imeRequest(wp, lp uintptr) uintptr {
	if s.cancelingInput || !s.input.Active {
		return 0
	}
	context := s.inputTextContext()
	units := utf16Units(context.Text)
	start, end := context.Start, context.End
	switch wp {
	case imrDocumentFeed, imrReconvertString:
		head := uint32(unsafe.Sizeof(reconvertString{}))
		size := head + uint32(len(units))*2
		if lp == 0 {
			return uintptr(size)
		}
		rs := (*reconvertString)(native(lp))
		if rs.Size < size {
			return 0
		}
		*rs = reconvertString{Size: size, StrLen: uint32(len(units)), StrOffset: head,
			CompStrLen: uint32(end - start), CompStrOffset: uint32(start) * 2,
			TargetStrLen: uint32(end - start), TargetStrOffset: uint32(start) * 2}
		copy(unsafe.Slice((*uint16)(unsafe.Add(native(lp), head)), len(units)), units)
		if wp == imrReconvertString {
			// An IME may use the offered range without a confirmation, or
			// fall back to it when a proposed adjustment is rejected.
			s.rememberReconversion(context, units, start, end)
		}
		return uintptr(size)
	case imrConfirmReconvertString:
		if lp == 0 {
			return 0
		}
		rs := (*reconvertString)(native(lp))
		from, to, ok := reconversionRange(rs, units)
		if !ok {
			return 0
		}
		// Confirmation may adjust the range, not replace the surrounding
		// document. Reject a buffer obtained before that text changed.
		provided := unsafe.Slice((*uint16)(unsafe.Add(native(lp), rs.StrOffset)), len(units))
		if !slices.Equal(provided, units) {
			return 0
		}
		if s.reconvertContext != nil && *s.reconvertContext != context {
			return 0
		}
		s.rememberReconversion(context, units, from, to)
		return 1
	}
	return 0
}

func (s *surface) inputTextContext() platform.TextContext {
	if c := s.input.Client; c != nil {
		return platform.ClientTextContext(c)
	}
	start, end := unitOffset(s.input.Text, max(0, s.input.Start)), unitOffset(s.input.Text, max(0, s.input.End))
	return platform.TextContext{Text: s.input.Text, Start: min(start, end), End: max(start, end), Caret: end}
}

func (s *surface) rememberReconversion(context platform.TextContext, units []uint16, from, to int) {
	if s.input.Client != nil {
		r := context.Range(platform.TextRange{Start: from, End: to})
		s.reconvert = &[2]int{r.Start, r.End}
	} else {
		s.reconvert = &[2]int{runeOffset(units, from), runeOffset(units, to)}
	}
	s.reconvertContext = &context
}

// IMM lengths are UTF-16 unit counts; offsets are bytes. Validate in widened
// arithmetic before accessing the supplied text or translating to rune ranges.
func reconversionRange(rs *reconvertString, units []uint16) (int, int, bool) {
	head := uint32(unsafe.Sizeof(reconvertString{}))
	if rs.Size < head || rs.Version != 0 || rs.StrOffset < head || rs.StrOffset%2 != 0 ||
		uint64(rs.StrOffset)+2*uint64(rs.StrLen) > uint64(rs.Size) || uint64(rs.StrLen) != uint64(len(units)) {
		return 0, 0, false
	}
	boundary := func(i uint64) bool {
		if i > uint64(len(units)) {
			return false
		}
		return i == 0 || i == uint64(len(units)) || !(units[i-1] >= 0xD800 && units[i-1] <= 0xDBFF && units[i] >= 0xDC00 && units[i] <= 0xDFFF)
	}
	from, to := uint64(rs.CompStrOffset)/2, uint64(rs.CompStrOffset)/2+uint64(rs.CompStrLen)
	target, targetEnd := uint64(rs.TargetStrOffset)/2, uint64(rs.TargetStrOffset)/2+uint64(rs.TargetStrLen)
	if rs.CompStrOffset%2 != 0 || rs.TargetStrOffset%2 != 0 || !boundary(from) || !boundary(to) || !boundary(target) || !boundary(targetEnd) ||
		(rs.TargetStrLen != 0 && (target < from || targetEnd > to)) {
		return 0, 0, false
	}
	return int(from), int(to), true
}

// cancelStaleReconversion prevents a deferred native range from overwriting a
// document or selection that has changed since the range was offered.
func (s *surface) cancelStaleReconversion() bool {
	if s.reconvertContext != nil && *s.reconvertContext != s.inputTextContext() {
		// Native confirmation and delivery are separate calls. A changed
		// document/selection must not receive the old range or its text.
		// Cancel IMM with the existing reentrancy guard; its synchronous
		// callbacks are discarded, and inputEpoch stops a following preview
		// in the same WM_IME_COMPOSITION message.
		s.cancelInput(s.cancelIME)
		return true
	}
	return false
}

// composed sends what an input method composed or committed, in place of
// the text it reconverts, if any.
func (s *surface) composed(ev platform.SurfaceEvent) {
	if s.cancelingInput || s.cancelStaleReconversion() {
		return
	}
	if ev.Kind == platform.TextComposition {
		s.imePreview, s.imeCaret = ev.Text, max(0, min(ev.Caret, utf8.RuneCountInString(ev.Text)))
		if ev.Text == "" && s.input.Client != nil {
			if _, marked := s.input.Client.MarkedRange(); !marked {
				// An empty preview cancels marked text; it must not delete
				// the selection when reconversion has not marked anything yet.
				s.reconvert, s.reconvertContext = nil, nil
				s.clientComposition.Reset()
				return
			}
		}
	} else if ev.Kind == platform.TextInput {
		s.imePreview, s.imeCaret = "", 0
	}
	if r := s.reconvert; r != nil {
		ev.Replace, ev.From, ev.To = true, r[0], r[1]
		s.reconvert = nil
		s.reconvertContext = nil
	}
	if c := s.input.Client; c != nil {
		var r *platform.TextRange
		if ev.Replace {
			rangeWanted := platform.TextRange{Start: ev.From, End: ev.To}
			r = &rangeWanted
		}
		if ev.Kind == platform.TextInput {
			if !s.clientComposition.Replace(c, r, ev.Text) {
				s.cancelInput(s.cancelIME)
			}
		} else {
			s.clientComposition.Preedit(c, r, ev.Text, ev.Caret)
		}
		return
	}
	s.send(ev)
}

// A terminal has no TextInputClient: its functional target, not a nil client
// pointer, owns the composition. IMM cancellation can synchronously send text
// and end-composition messages after UI focus has already changed. Discard
// those messages instead of committing them to either the old or new target.
func (s *surface) cancelInput(cancel func()) {
	s.clearInputState()
	if s.cancelingInput {
		return
	}
	s.cancelingInput = true
	defer func() {
		s.clearInputState()
		s.cancelingInput = false
	}()
	cancel()
}

func (s *surface) clearInputState() {
	s.inputEpoch++
	s.layoutKeys, s.keypadKeys = nil, nil
	s.layoutComposing, s.layoutDead = false, false
	s.imePreview, s.imeCaret = "", 0
	s.high = 0
	s.reconvert = nil
	s.reconvertContext = nil
	s.clientComposition.Reset()
}

// IME data lengths are signed byte counts, not UTF-16 character counts. Keep
// failure distinct from a valid empty string, and decode only the bytes the
// second query actually returned (the composition can change between calls).
const maxIMEStringBytes = 16 << 20

func readIMEString(query func([]uint16) int32) (string, bool) {
	n := query(nil)
	if n < 0 || n > maxIMEStringBytes || n%2 != 0 {
		return "", false
	}
	if n == 0 {
		return "", true
	}
	buf := make([]uint16, int(n)/2)
	copied := query(buf)
	if copied < 0 || copied > n || copied%2 != 0 {
		return "", false
	}
	return string(utf16.Decode(buf[:int(copied)/2])), true
}

func imeString(himc, which uintptr) (string, bool) {
	return readIMEString(func(buf []uint16) int32 {
		if len(buf) == 0 {
			n, _, _ := procImmGetCompositionStrW.Call(himc, which, 0, 0)
			return int32(n)
		}
		n, _, _ := procImmGetCompositionStrW.Call(himc, which, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)*2))
		return int32(n)
	})
}

// CS_INSERTCHAR is provisional text, not a commit. Keep it in the same
// marked range as GCS_COMPSTR so a later result replaces it exactly once.
// Windows' Unicode window procedure supplies a UTF-16 unit in wParam.
func (s *surface) imeInsertCharacter(character, flags uintptr) {
	if s.cancelingInput {
		return
	}
	runes := []rune(s.imePreview)
	caret := max(0, min(s.imeCaret, len(runes)))
	text := string(runes[:caret]) + string(utf16.Decode([]uint16{uint16(character)})) + string(runes[caret:])
	if flags&csNoMoveCaret == 0 {
		caret++
	}
	s.layoutComposing = true
	s.composed(platform.SurfaceEvent{Kind: platform.TextComposition, Text: text, Caret: caret})
}

func (s *surface) imeComposition(flags uintptr, text func(uintptr) (string, bool), cursor func() int32) {
	if s.cancelingInput {
		return
	}
	if flags == 0 {
		// WM_IME_COMPOSITION without GCS data cancels the preview, even when
		// IMM has already released the input context. It does not commit text.
		if s.cancelStaleReconversion() {
			return
		}
		s.layoutComposing = false
		s.reconvert = nil
		s.reconvertContext = nil
		s.composed(platform.SurfaceEvent{Kind: platform.TextComposition})
		return
	}
	epoch := s.inputEpoch
	if flags&gcsResultStr != 0 {
		if result, ok := text(gcsResultStr); ok {
			s.layoutComposing = false
			s.composed(platform.SurfaceEvent{Kind: platform.TextInput, Text: result})
		}
	}
	// Delivering a result may switch focus or close its target. A preview
	// attached to that same native message still belongs to the old target.
	if epoch != s.inputEpoch {
		return
	}
	if flags&(gcsCompStr|gcsCursorPos) != 0 {
		if preview, ok := text(gcsCompStr); ok {
			pos := cursor()
			caret := utf8.RuneCountInString(preview)
			if pos >= 0 {
				end := platform.UTF16ByteOffset(preview, int(pos))
				caret = utf8.RuneCountInString(preview[:end])
			}
			s.layoutComposing = preview != ""
			s.composed(platform.SurfaceEvent{Kind: platform.TextComposition, Text: preview, Caret: caret})
		}
	}
}

func (s *surface) cancelIME() {
	if himc, _, _ := procImmGetContext.Call(s.hwnd); himc != 0 {
		procImmNotifyIME.Call(himc, niCompositionStr, cpsCancel, 0)
		procImmReleaseContext.Call(s.hwnd, himc)
	}
}

// unitOffset returns the UTF-16 offset of rune i of s.
func unitOffset(s string, i int) int {
	n := 0
	for _, r := range s {
		if i == 0 {
			break
		}
		n += utf16.RuneLen(r)
		i--
	}
	return n
}

// runeOffset returns the rune offset of UTF-16 unit i.
func runeOffset(units []uint16, i int) int {
	return len(utf16.Decode(units[:i]))
}

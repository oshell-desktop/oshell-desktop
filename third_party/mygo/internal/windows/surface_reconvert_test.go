//go:build windows && (amd64 || arm64)

package windows

import (
	"runtime"
	"strings"
	"testing"
	"unsafe"

	"github.com/egoist/mygo/internal/platform"
)

func reconvertBuffer(text string, start, end int) ([]uint32, *reconvertString) {
	units := utf16Units(text)
	head := uint32(unsafe.Sizeof(reconvertString{}))
	buf := make([]uint32, (int(head)+2*len(units)+3)/4)
	r := (*reconvertString)(unsafe.Pointer(&buf[0]))
	*r = reconvertString{Size: head + uint32(2*len(units)), StrOffset: head, StrLen: uint32(len(units)),
		CompStrOffset: uint32(start * 2), CompStrLen: uint32(end - start), TargetStrOffset: uint32(start * 2), TargetStrLen: uint32(end - start)}
	copy(unsafe.Slice((*uint16)(unsafe.Add(unsafe.Pointer(r), head)), len(units)), units)
	return buf, r
}

func TestIMEReconvertRejectsNullConfirmation(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("null confirmation panicked: %v", r)
		}
	}()
	s := &surface{input: platform.TextInputState{Active: true, Text: "text"}}
	if got := s.imeRequest(imrConfirmReconvertString, 0); got != 0 {
		t.Fatalf("accepted null: %d", got)
	}
}

func TestIMEReconvertValidatesBufferAndUnicodeBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*reconvertString)
	}{
		{"version", func(r *reconvertString) { r.Version = 1 }},
		{"short-header", func(r *reconvertString) { r.Size = 4 }},
		{"truncated-text", func(r *reconvertString) { r.Size-- }},
		{"text-over-header", func(r *reconvertString) { r.StrOffset = 0 }},
		{"odd-text-offset", func(r *reconvertString) { r.StrOffset++ }},
		{"text-overflow", func(r *reconvertString) { r.StrLen = ^uint32(0) }},
		{"text-offset-overflow", func(r *reconvertString) { r.StrOffset = ^uint32(0) - 1 }},
		{"odd-composition-offset", func(r *reconvertString) { r.CompStrOffset++ }},
		{"composition-start-in-pair", func(r *reconvertString) { r.CompStrOffset = 4; r.CompStrLen = 1 }},
		{"composition-end-in-pair", func(r *reconvertString) { r.CompStrLen = 1 }},
		{"odd-target-offset", func(r *reconvertString) { r.TargetStrOffset++ }},
		{"target-start-in-pair", func(r *reconvertString) { r.TargetStrOffset = 4; r.TargetStrLen = 1 }},
		{"target-end-in-pair", func(r *reconvertString) { r.TargetStrLen = 1 }},
		{"target-outside-composition", func(r *reconvertString) { r.TargetStrOffset = 0; r.TargetStrLen = 1 }},
		{"target-length-overflow", func(r *reconvertString) { r.TargetStrLen = ^uint32(0) }},
		{"composition-length-overflow", func(r *reconvertString) { r.CompStrLen = ^uint32(0) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			buf, r := reconvertBuffer("a😀bc", 1, 3)
			tc.change(r)
			s := &surface{input: platform.TextInputState{Active: true, Text: "a😀bc"}}
			if got := s.imeRequest(imrConfirmReconvertString, uintptr(unsafe.Pointer(r))); got != 0 || s.reconvert != nil {
				t.Fatalf("accepted invalid structure: %d %+v", got, r)
			}
			runtime.KeepAlive(buf)
		})
	}
}

func TestIMEReconvertAcceptsWholeEmojiAndRejectsChangedText(t *testing.T) {
	buf, r := reconvertBuffer("a😀bc", 1, 3)
	s := &surface{input: platform.TextInputState{Active: true, Text: "a😀bc"}}
	if got := s.imeRequest(imrConfirmReconvertString, uintptr(unsafe.Pointer(r))); got != 1 || s.reconvert == nil || *s.reconvert != [2]int{1, 2} {
		t.Fatalf("valid emoji range: %d %v", got, s.reconvert)
	}
	// Same length does not make a stale input-method buffer current.
	s.input.Text = "z😀bc"
	s.reconvert = nil
	if got := s.imeRequest(imrConfirmReconvertString, uintptr(unsafe.Pointer(r))); got != 0 || s.reconvert != nil {
		t.Fatalf("accepted stale context: %d %v", got, s.reconvert)
	}
	runtime.KeepAlive(buf)
}

func TestIMEDocumentFeedNormalizesLegacySelection(t *testing.T) {
	for _, tc := range []struct{ start, end, offset, length int }{{-2, 1, 0, 1}, {3, 1, 2, 3}, {0, 99, 0, 5}, {-1, -2, 0, 0}} {
		s := &surface{input: platform.TextInputState{Active: true, Text: "a😀bc", Start: tc.start, End: tc.end}}
		buf, r := reconvertBuffer("a😀bc", 0, 0)
		if got := s.imeRequest(imrDocumentFeed, uintptr(unsafe.Pointer(r))); got == 0 || r.CompStrOffset != uint32(tc.offset) || r.CompStrLen != uint32(tc.length) {
			t.Fatalf("selection %d..%d: offset=%d length=%d", tc.start, tc.end, r.CompStrOffset, r.CompStrLen)
		}
		runtime.KeepAlive(buf)
	}
}

type reconvertClient struct {
	compositionClient
	text        string
	sel         platform.TextRange
	replacement *platform.TextRange
}

func (c *reconvertClient) TextForRange(r platform.TextRange) (string, platform.TextRange) {
	a, z := platform.UTF16ByteOffset(c.text, r.Start), platform.UTF16ByteOffset(c.text, r.End)
	return c.text[a:z], platform.TextRange{Start: platform.UTF16Len(c.text[:a]), End: platform.UTF16Len(c.text[:z])}
}
func (c *reconvertClient) Selection() platform.TextSelection {
	return platform.TextSelection{Range: c.sel}
}
func (c *reconvertClient) MarkedRange() (platform.TextRange, bool) {
	return platform.TextRange{}, false
}
func (c *reconvertClient) SetMarkedText(r *platform.TextRange, text string, caret platform.TextRange) {
	if r != nil {
		copyRange := *r
		c.replacement = &copyRange
	} else {
		c.replacement = nil
	}
	c.compositionClient.SetMarkedText(r, text, caret)
}

func TestIMEReconvertClientRangeUsesDocumentUTF16Offsets(t *testing.T) {
	c := &reconvertClient{text: strings.Repeat("x", 1100) + "a😀bc", sel: platform.TextRange{Start: 1101, End: 1103}}
	s := &surface{input: platform.TextInputState{Active: true, Client: c}}
	for _, request := range []uintptr{imrDocumentFeed, imrReconvertString} {
		size := s.imeRequest(request, 0)
		buf := make([]uint32, (size+3)/4)
		r := (*reconvertString)(unsafe.Pointer(&buf[0]))
		r.Size = uint32(size)
		if got := s.imeRequest(request, uintptr(unsafe.Pointer(r))); got != size {
			t.Fatalf("fill size=%d want=%d", got, size)
		}
		context := platform.ClientTextContext(c)
		if context.Base == 0 {
			t.Fatal("fixture did not exercise a bounded document window")
		}
		if r.CompStrOffset != uint32((1101-context.Base)*2) || r.CompStrLen != 2 {
			t.Fatalf("exported wrong UTF-16 selection: %+v", r)
		}
		// The IME expands the composition by one ASCII unit while retaining
		// the Emoji target clause. The client needs absolute UTF-16 offsets.
		r.CompStrOffset -= 2
		r.CompStrLen++
		if got := s.imeRequest(imrConfirmReconvertString, uintptr(unsafe.Pointer(r))); got != 1 {
			t.Fatal("valid expanded range rejected")
		}
		s.composed(platform.SurfaceEvent{Kind: platform.TextComposition, Text: "新", Caret: 1})
		if c.replacement == nil || *c.replacement != (platform.TextRange{Start: 1100, End: 1103}) {
			t.Fatalf("client replacement: %v", c.replacement)
		}
		s.composed(platform.SurfaceEvent{Kind: platform.TextComposition, Text: "新版", Caret: 2})
		if c.replacement != nil {
			t.Fatal("original replacement range was reused for a later preview")
		}
		runtime.KeepAlive(buf)
	}
}

func TestIMEReconvertAllowsEmptyTargetAndKeepsRangeOnRejection(t *testing.T) {
	buf, r := reconvertBuffer("a😀bc", 1, 3)
	r.TargetStrOffset, r.TargetStrLen = 0, 0
	s := &surface{input: platform.TextInputState{Active: true, Text: "a😀bc"}}
	if got := s.imeRequest(imrConfirmReconvertString, uintptr(unsafe.Pointer(r))); got != 1 {
		t.Fatal("valid absent target rejected")
	}
	want := *s.reconvert
	r.CompStrOffset = 3
	if got := s.imeRequest(imrConfirmReconvertString, uintptr(unsafe.Pointer(r))); got != 0 || s.reconvert == nil || *s.reconvert != want {
		t.Fatalf("rejection changed accepted range: %d %v", got, s.reconvert)
	}
	runtime.KeepAlive(buf)
}

func prepareReconversion(t *testing.T, s *surface, confirm bool) {
	t.Helper()
	size := s.imeRequest(imrReconvertString, 0)
	buf := make([]uint32, (size+3)/4)
	r := (*reconvertString)(unsafe.Pointer(&buf[0]))
	r.Size = uint32(size)
	if s.imeRequest(imrReconvertString, uintptr(unsafe.Pointer(r))) != size {
		t.Fatal("reconversion offer failed")
	}
	if confirm && s.imeRequest(imrConfirmReconvertString, uintptr(unsafe.Pointer(r))) != 1 {
		t.Fatal("reconversion confirmation failed")
	}
	runtime.KeepAlive(buf)
}

func TestIMEReconvertRechecksContextBeforeReplacing(t *testing.T) {
	for _, confirm := range []bool{false, true} {
		for _, mode := range []string{"text", "selection", "repeated-context"} {
			for _, kind := range []platform.SurfaceEventKind{platform.TextComposition, platform.TextInput} {
				c := &reconvertClient{text: "abc😀def", sel: platform.TextRange{Start: 3, End: 5}}
				if mode == "repeated-context" {
					c.text = strings.Repeat("x", 5000)
					c.sel = platform.TextRange{Start: 2500, End: 2501}
				}
				s := &surface{input: platform.TextInputState{Active: true, Client: c}}
				prepareReconversion(t, s, confirm)
				if mode == "text" {
					c.text = "xyz😀def"
				} else {
					c.sel.Start++
					c.sel.End++
				}
				s.composed(platform.SurfaceEvent{Kind: kind, Text: "旧", Caret: 1})
				if len(c.changes) != 0 || s.reconvert != nil {
					t.Fatalf("confirm=%v mode=%s kind=%v stale replacement: %v range=%v", confirm, mode, kind, c.changes, s.reconvert)
				}
			}
		}
	}
}

func TestIMEReconvertStaleResultDoesNotDeliverFollowingPreview(t *testing.T) {
	c := &reconvertClient{text: "cafe", sel: platform.TextRange{Start: 3, End: 4}}
	s := &surface{input: platform.TextInputState{Active: true, Client: c}}
	prepareReconversion(t, s, true)
	c.text = "safe"
	reads := 0
	s.imeComposition(gcsResultStr|gcsCompStr, func(uintptr) (string, bool) { reads++; return "旧", true }, func() int32 { return 1 })
	if len(c.changes) != 0 || reads != 1 {
		t.Fatalf("stale result/preview: %v reads=%d", c.changes, reads)
	}
	// Cancelling stale native input must not disable the next direct commit.
	s.imeComposition(gcsResultStr, func(uintptr) (string, bool) { return "新", true }, nil)
	if len(c.changes) != 1 || c.changes[0] != "replace:新" {
		t.Fatalf("new input lost: %v", c.changes)
	}
}

func TestIMEReconvertEmptyPreviewNeverDeletesSelection(t *testing.T) {
	for _, flags := range []uintptr{0, gcsCompStr} {
		c := &reconvertClient{text: "abc😀def", sel: platform.TextRange{Start: 3, End: 5}}
		s := &surface{input: platform.TextInputState{Active: true, Client: c}}
		prepareReconversion(t, s, true)
		s.imeComposition(flags, func(uintptr) (string, bool) { return "", true }, func() int32 { return 0 })
		if len(c.changes) != 0 || s.reconvert != nil || s.reconvertContext != nil {
			t.Fatalf("flags=%x cancellation deleted selected text: %v", flags, c.changes)
		}
	}
}

func TestIMEReconvertGeometryChangeKeepsValidRange(t *testing.T) {
	c := &reconvertClient{text: "abc😀def", sel: platform.TextRange{Start: 3, End: 5}}
	s := &surface{input: platform.TextInputState{Active: true, Client: c}, ime: true}
	prepareReconversion(t, s, true)
	next := s.input
	next.Caret.X += 20
	s.setTextInput(next, func() { t.Fatal("geometry update cancelled reconversion") })
	s.composed(platform.SurfaceEvent{Kind: platform.TextComposition, Text: "新", Caret: 1})
	if c.replacement == nil || *c.replacement != (platform.TextRange{Start: 3, End: 5}) || s.reconvertContext != nil {
		t.Fatalf("valid range lost: %v context=%v", c.replacement, s.reconvertContext)
	}
}

func TestIMEReconvertContextReleasedAtInputBoundaries(t *testing.T) {
	for _, mode := range []string{"end", "cancel", "focus"} {
		c := &reconvertClient{text: "cafe", sel: platform.TextRange{Start: 3, End: 4}}
		s := &surface{input: platform.TextInputState{Active: true, Client: c}}
		prepareReconversion(t, s, true)
		switch mode {
		case "end":
			s.message(0, wmImeEndComp, 0, 0)
		case "cancel":
			s.message(0, wmImeComposition, 0, 0)
		case "focus":
			s.cancelInput(func() {})
		}
		if s.reconvert != nil || s.reconvertContext != nil {
			t.Fatalf("%s retained document snapshot", mode)
		}
	}
}

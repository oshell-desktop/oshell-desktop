package text

import (
	"strings"
	"testing"
	"unsafe"
)

func TestDWriteLegacyFamily(t *testing.T) {
	s := Shared()
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.engine().(*dwrite)
	if unsafe.Sizeof(dwLogFont{}) != 92 {
		t.Fatal("LOGFONTW ABI size")
	}
	for _, name := range []string{"OShell nonexistent family", strings.Repeat("Cascadia Code Light", 3), "Cascadia\x00Code"} {
		if got := e.legacyFamily(name, 400, false); got.family != "" {
			t.Fatalf("unknown/truncated font accepted: %+v", got)
		}
		if got := e.family(Style{Family: name + ", monospace"}); got.family != e.family(Style{Family: "monospace"}).family {
			t.Fatal("fallback list not honored")
		}
	}
	light := e.legacyFamily("Cascadia Code Light", 400, false)
	if light.family == "" {
		t.Skip("installed Cascadia Code Light required for Windows alias acceptance")
	}
	if light.family != "Cascadia Code" || light.weight != 300 || light.custom || light.italic || light.stretch != fontStretchNormal {
		t.Fatalf("wrong Light face: %+v", light)
	}
	for _, weight := range []int{400, 700} {
		for _, italic := range []bool{false, true} {
			style := Style{Family: "cascadia code light, monospace", Weight: weight, Italic: italic, Size: 18}
			match := e.family(style)
			if match.custom || match.family != light.family || match.italic != italic || weight == 700 && match.weight < 600 {
				t.Fatalf("wrong variant: %+v", match)
			}
			format := e.format(style)
			if format == 0 {
				t.Fatal("no format")
			}
			// IDWriteTextFormat getters: weight 22, style 23, stretch 24.
			if int(call(format, 22)) != match.weight || call(format, 23) != dwStyle(match.italic) || call(format, 24) != match.stretch {
				t.Fatal("format and metrics select different variants")
			}
			font := e.font(style)
			for _, spans := range []bool{false, true} {
				base := style
				var ranges []Span
				if spans {
					base = Style{Family: "monospace", Size: 18}
					ranges = []Span{{End: 3, Family: style.Family, Weight: weight, Italic: italic}}
				}
				lines := e.shape([]rune("MWi"), base, ranges, 0, false, false)
				if len(lines) != 1 || len(lines[0].runs) != 1 || len(lines[0].runs[0].glyphs) != 3 {
					t.Fatal("missing shaped text")
				}
				for _, g := range lines[0].runs[0].glyphs {
					if g.Font != font {
						t.Fatal("shaping and metrics select different font faces")
					}
				}
			}
		}
	}
}

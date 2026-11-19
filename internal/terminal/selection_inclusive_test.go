package terminal

import (
	"fmt"
	"strings"
	"testing"

	"oshell-desktop/internal/terminal/internal/vt"
)

func TestPointerDragIncludesTouchedCharacters(t *testing.T) {
	for _, tc := range []struct {
		name, text, want                         string
		start, end, highlightStart, highlightEnd int
	}{
		{"forward", "abcdef", "bcde", 1, 4, 1, 4},
		{"reverse", "abcdef", "bcde", 4, 1, 1, 4},
		{"wide-end", "A中文B", "A中文", 0, 3, 0, 4},
		{"wide-start-tail", "A中文B", "中文B", 2, 5, 1, 5},
		{"wide-reverse", "A中文B", "A中文", 4, 0, 0, 4},
		{"combined-end", "ab e\u0301x", "ab e\u0301", 0, 3, 0, 3},
		{"emoji-end", "ab😀x", "ab😀", 0, 2, 0, 3},
	} {
		for _, scale := range []float32{1, 1.25, 2} {
			t.Run(fmt.Sprintf("%s/scale=%g", tc.name, scale), func(t *testing.T) {
				term, tt := wordClickFixture(t, tc.text, 400)
				tt.SetScale(scale)
				tt.Frame()
				x, y := cellCenter(term, tc.start, 0)
				beginCharacterDrag(tt, x, y)
				x, y = cellCenter(term, tc.end, 0)
				tt.Move(x, y)
				if got := term.SearchSelection(); got != tc.want {
					t.Fatalf("drag selected %q, want %q", got, tc.want)
				}
				render, err := vt.NewRenderState()
				if err != nil {
					t.Fatal(err)
				}
				defer render.Free()
				term.mu.Lock()
				err = render.Update(term.term)
				term.mu.Unlock()
				if err != nil {
					t.Fatal(err)
				}
				render.Rows()
				if !render.NextRow() {
					t.Fatal("missing first row")
				}
				if start, end, ok := render.RowSelection(); !ok || start != tc.highlightStart || end != tc.highlightEnd {
					t.Fatalf("highlight = %d..%d (%v), want %d..%d", start, end, ok, tc.highlightStart, tc.highlightEnd)
				}
				tt.Release(x, y)
				if got := tt.Clipboard(); got != tc.want {
					t.Fatalf("copied %q, want %q", got, tc.want)
				}
			})
		}
	}
}

func TestPointerDragUsesReleasePosition(t *testing.T) {
	for _, move := range []bool{false, true} {
		t.Run(fmt.Sprint(move), func(t *testing.T) {
			term, tt := wordClickFixture(t, "abcdef", 400)
			x, y := cellCenter(term, 0, 0)
			tt.Press(x, y)
			if move {
				x, y = cellCenter(term, 3, 0)
				tt.Move(x, y)
			}
			x, y = cellCenter(term, 4, 0)
			tt.Release(x, y)
			if got := tt.Clipboard(); got != "abcde" {
				t.Fatalf("release copied %q, want abcde", got)
			}
		})
	}
}

func TestPointerDragIncludesLastColumnAndWrappedEnd(t *testing.T) {
	for _, wrap := range []bool{false, true} {
		term, tt := wordClickFixture(t, "", 320)
		text := strings.Repeat("a", term.v.cols-1) + "Z"
		end, row, want := term.v.cols-1, 0, text
		if wrap {
			text += "BC"
			end, row, want = 1, 1, text
		}
		term.Feed([]byte(text))
		tt.Frame()
		x, y := cellCenter(term, 0, 0)
		tt.Press(x, y)
		x, y = cellCenter(term, end, row)
		tt.Move(x, y)
		tt.Release(x, y)
		if got := tt.Clipboard(); got != want {
			t.Fatalf("wrap=%v copied %q, want %q", wrap, got, want)
		}
	}
}

func TestPointerClickDoesNotSelectOrCopy(t *testing.T) {
	term, tt := wordClickFixture(t, "abcdef", 400)
	term.v.services.WriteClipboard("keep clipboard")
	x, y := cellCenter(term, 2, 0)
	tt.Press(x, y)
	tt.Release(x, y)
	if term.SearchSelection() != "" || tt.Clipboard() != "keep clipboard" {
		t.Fatal("plain click selected or copied text")
	}
}

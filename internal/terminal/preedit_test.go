package terminal

import (
	"bytes"
	"fmt"
	"image/png"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestPreeditProvisionalCharacterCommitsOnlyFinalResult(t *testing.T) {
	for _, flags := range []int{0, 31} {
		t.Run(fmt.Sprint(flags), func(t *testing.T) {
			term, tt, conn := bidiViewFixture(t)
			term.Feed([]byte(fmt.Sprintf("\x1b[=%du", flags)))
			// Windows CS_INSERTCHAR produces these provisional composition
			// events. The final IMM result must be the only remote input.
			tt.Compose("가", 0)
			tt.Compose("각", 0)
			term.Send([]byte("CHECK"))
			if got := conn.take(5); got != "CHECK" {
				t.Fatalf("provisional text sent: %q", got)
			}
			tt.Type("각")
			tt.Compose("", 0)
			term.Send([]byte("END"))
			want := "각END"
			if flags != 0 {
				want = "\x1b[0;;44033uEND"
			}
			if got := conn.take(len(want)); got != want {
				t.Fatalf("result: %q, want %q", got, want)
			}
			if term.v.preedit != "" || term.v.preeditShape.shaped != nil {
				t.Fatal("result retained preview")
			}
		})
	}
}

func TestPreeditWrapsWithoutChangingTerminal(t *testing.T) {
	term, tt, conn := bidiViewFixture(t)
	term.Feed([]byte("prompt> \x1b[1;65H"))
	tt.Frame()
	before := append([]byte(nil), tt.Image().Pix...)
	tt.Compose(strings.Repeat("中文预编辑 ", 12), 2)
	im := tt.Image()
	if path := os.Getenv("OSHELL_PREEDIT_PREVIEW"); path != "" {
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, im)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	y0 := term.v.oy + term.v.cellH
	y1 := min(y0+term.v.cellH*3, im.Bounds().Dy())
	changed := false
	for y := y0; y < y1; y++ {
		a, b := y*im.Stride, (y+1)*im.Stride
		if !bytes.Equal(before[a:b], im.Pix[a:b]) {
			changed = true
			break
		}
	}
	if !changed {
		t.Fatal("long preedit was clipped at the first row instead of wrapping")
	}
	// The prompt area before the caret must remain untouched.
	for y := term.v.oy; y < term.v.oy+term.v.cellH; y++ {
		a := y*im.Stride + term.v.ox*4
		b := a + 8*term.v.cellW*4
		if !bytes.Equal(before[a:b], im.Pix[a:b]) {
			t.Fatal("preedit covered the prompt")
		}
	}
	term.Send([]byte("END"))
	if got := conn.take(3); got != "END" {
		t.Fatalf("preedit was sent remotely: %q", got)
	}
	tt.Compose("", 0)
	if !bytes.Equal(before, tt.Image().Pix) {
		t.Fatal("cancel did not restore original terminal rendering")
	}
}

func TestPreeditLayoutCacheAndResize(t *testing.T) {
	term, tt, _ := bidiViewFixture(t)
	text := strings.Repeat("中文 e\u0301 😀 paragraph ", 12)
	tt.Compose(text, 0)
	old := term.v.preeditShape.shaped
	if len(old) < 2 {
		t.Fatal("missing wrapped preedit")
	}
	tt.Frame()
	if &term.v.preeditShape.shaped[0] != &old[0] {
		t.Fatal("unchanged preedit reshaped")
	}
	tt.SetSize(320, 260)
	if len(term.v.preeditShape.shaped) <= len(old) {
		t.Fatal("resize retained stale wrapping")
	}
	tt.SetScale(1.5)
	if len(term.v.preeditShape.shaped) < 2 {
		t.Fatal("DPI change lost preedit")
	}
	term.SetTheme(&Theme{Background: ui.Hex("#ffffff"), Foreground: ui.Hex("#222222")}, nil)
	tt.Frame()
	if term.v.preedit != text {
		t.Fatal("theme change lost composition")
	}
	tt.Type("提交")
	if term.v.preeditShape.shaped != nil || term.v.preedit != "" {
		t.Fatal("commit retained composition layout")
	}
}

func TestPreeditDoubleWidthRowChangesAnchorNotFont(t *testing.T) {
	term, tt, _ := bidiViewFixture(t)
	term.Feed([]byte("\x1b[1;5H"))
	tt.Frame()
	tt.Compose("abc", 1)
	width := term.v.preeditShape.shaped[0].Width
	anchor := term.v.preeditShape.indent
	tt.Compose("", 0)
	term.Feed([]byte("\x1b#6\x1b[1;5H"))
	tt.Frame()
	tt.Compose("abc", 1)
	if math.Abs(float64(term.v.preeditShape.indent-2*anchor)) > 0.01 || math.Abs(float64(term.v.preeditShape.shaped[0].Width-width)) > 0.01 {
		t.Fatalf("double-width preedit: anchor=%v want=%v width=%v want=%v", term.v.preeditShape.indent, 2*anchor, term.v.preeditShape.shaped[0].Width, width)
	}
}

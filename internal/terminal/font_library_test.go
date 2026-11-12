package terminal

import (
	"bytes"
	"testing"

	"github.com/egoist/mygo/ui"
	"golang.org/x/image/font/gofont/goregular"
)

func TestPrivateFontReplacementRefreshesExistingTerminal(t *testing.T) {
	loadLib(t)
	defer func() { _ = ui.ReplaceUserFonts(nil) }()
	term, err := New(Options{Conn: newPipe(), NoBlink: true, Font: Font{Family: "Go, monospace", Size: 19}})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	tt := ui.NewTester(func(c *ui.Context) { View(c, term).Fill().AutoFocus() }, 1000, 170)
	term.Feed([]byte("MWil09 terminal content\r\n\x1b[1mbold\x1b[0m \x1b[3mitalic\x1b[0m 中文"))
	tt.Frame()
	beforeFont := term.v.font
	beforeWidth := term.v.cellW
	beforeHeight := term.v.cellH
	// Native font handles may be recreated when the private library is removed.
	renderSample := func() []byte {
		return ui.Render(func(c *ui.Context) {
			ui.Box(c).Fill().Draw(func(p *ui.Painter, r ui.Rect) {
				p.Fill(r, ui.Hex("#111111"), 0)
				p.Glyphs(ui.Shape("MWil09", term.v.fonts[0]), r.X, r.Y+24, ui.Hex("#ffffff"))
			})
		}, 160, 36, 1).Pix
	}
	before := renderSample()
	if err = ui.ReplaceUserFonts([][]byte{goregular.TTF}); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if term.v.font == beforeFont || bytes.Equal(before, renderSample()) {
		t.Fatal("existing terminal retained stale font")
	}
	if term.opts.Font.Size != 19 {
		t.Fatal("font reload reset local zoom")
	}
	v := term.v
	x := float32(v.ox) + 0.2*float32(v.cellW)
	y := float32(v.oy) + 0.5*float32(v.cellH)
	beginCharacterDrag(tt, x, y)
	tt.Move(x+6*float32(v.cellW), y)
	tt.Release(x+6*float32(v.cellW), y)
	tt.Command("copy")
	if got := tt.Clipboard(); got != "MWil09" {
		t.Fatalf("grid selection %q", got)
	}
	if err = ui.ReplaceUserFonts(nil); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if term.v.cellW != beforeWidth || term.v.cellH != beforeHeight {
		t.Fatal("fallback metrics not restored")
	}
	if !bytes.Equal(before, renderSample()) {
		t.Fatal("fallback glyphs not restored")
	}
}

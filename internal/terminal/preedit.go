package terminal

import "github.com/egoist/mygo/ui"

type preeditLayout struct {
	text                     string
	font                     ui.Font
	width, indent, cellWidth float32
	shaped                   []ui.GlyphLine
}

func (l *preeditLayout) lines(text string, font ui.Font, width, indent, cellWidth float32) []ui.GlyphLine {
	if text == l.text && font == l.font && width == l.width && indent == l.indent && cellWidth == l.cellWidth && l.shaped != nil {
		return l.shaped
	}
	natural := float32(0)
	for _, glyph := range ui.Shape("W", font) {
		natural += glyph.Advance
	}
	*l = preeditLayout{text: text, font: font, width: width, indent: indent, cellWidth: cellWidth,
		shaped: ui.ShapeLines(text, font, width, indent, max(0, cellWidth-natural))}
	return l.shaped
}

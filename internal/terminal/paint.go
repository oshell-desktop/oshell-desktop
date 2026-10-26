package terminal

import (
	"math"
	"oshell-desktop/internal/core"
	"oshell-desktop/internal/terminal/internal/bidilayout"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/terminal/internal/vt"
)

// Padding around the grid, in DIPs.
const padX, padY = 6, 4

// rowCache is what a row of the screen draws, made when it changes:
// backgrounds, runs of glyphs, lines and drawn characters, in cells.
type columnSpan struct{ start, end int }

type rowCache struct {
	bidi         bidilayout.Row
	selected     []columnSpan
	mode         vt.LineMode
	ok           bool
	bgs          []bgSpan
	runs         []glyphRun
	decos        []deco
	boxes        []boxCell
	placeholders []kittyPlaceholder
	selA         int
	selB         int
	sel          bool
	wide         []int16 // the columns of wide characters
}

type bgSpan struct {
	x0, x1 int
	c      ui.Color
}

// glyphRun is the glyphs of a run of cells of one style, placed in their
// cells, with X from the left of the grid in DIPs.
type glyphRun struct {
	glyphs []ui.Glyph
	cols   []int16 // the column of each glyph
	c      ui.Color
}

type deco struct {
	x0, x1 int
	kind   int // an underline style, decoStrike or decoOverline
	c      ui.Color
}

const (
	decoStrike   = 10
	decoOverline = 11
)

// boxCell is a character drawn rather than taken from the font: box
// drawing, blocks and Powerline's separators.
type boxCell struct {
	x int
	r rune
	c ui.Color
}

// layout sizes the grid for a box of the element at a scale.
func (v *view) layout(r ui.Rect, scale float32) {
	t := v.t
	t.mu.Lock()
	fk := t.opts.Font.key(scale)
	preferEmoji := t.opts.PreferColorEmoji
	t.mu.Unlock()
	if preferEmoji != v.preferColorEmoji {
		v.preferColorEmoji = preferEmoji
		v.shaped.clear()
		v.lines = v.lines[:0]
	}
	if fk != v.font {
		v.font, v.scale = fk, scale
		for i := range v.fonts {
			v.fonts[i] = fk.variant(i)
		}
		m := v.fonts[0].Metrics()
		adv := float32(0)
		for _, g := range ui.Shape(asciiPrintable, v.fonts[0]) {
			adv = max(adv, g.Advance)
		}
		v.cellW = max(int(math.Ceil(float64(adv*scale-0.05))), 1)
		v.cellH = max(int(math.Ceil(float64((m.Ascent+m.Descent+m.LineGap)*fk.lineHeight*scale-0.05))), 1)
		v.baseline = int(math.Round(float64((float32(v.cellH)-(m.Ascent+m.Descent)*scale)/2 + m.Ascent*scale)))
		v.shaped.clear()
		v.lines = v.lines[:0]
	}
	v.ox, v.oy = int(math.Round(padX*float64(scale))), int(math.Round(padY*float64(scale)))
	w, h := int(math.Round(float64(r.W*scale)))-2*v.ox, int(math.Round(float64(r.H*scale)))-2*v.oy
	v.cols, v.rows = max(w/v.cellW, 1), max(h/v.cellH, 1)
}

const asciiPrintable = " !\"#$%&'()*+,-./0123456789:;<=>?@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_`abcdefghijklmnopqrstuvwxyz{|}~"

// paint updates the rows that changed and draws the terminal.
func (v *view) paint(p *ui.Painter, r ui.Rect) {
	t := v.t
	v.layout(r, p.Scale())
	v.overlay.width, v.overlay.height = r.W, r.H
	bg := v.theme.Background
	t.mu.Lock()
	if t.term == nil {
		t.mu.Unlock()
		if !t.opts.Transparent {
			p.Fill(r, bg, 0)
		}
		return
	}
	t.resize(gridSize{v.cols, v.rows, v.cellW, v.cellH})
	if v.ticking && v.gesture != nil {
		v.autoscrollAt(p.Now())
	}
	t.term.ClampHistory(t.historyLines)
	t.rmu.Lock()
	defer t.rmu.Unlock()
	// A synchronized update shows when it ends, or after a second.
	if t.held && time.Since(t.heldSince) > time.Second {
		t.term.SetMode(vt.ModeSyncOutput, false)
		t.held = false
	}
	held := t.held
	if !held {
		t.rs.BeginUpdate(t.term)
		v.graphics = t.term.RenderGraphics(v.cellW, v.cellH)
		v.refreshBidi()
		v.bidiFrame = v.bidi
	}
	v.scrollbar = t.term.Scrollbar()
	linksChanged := v.refreshLinkHover(held)
	if t.term.AltScreen() || v.scrollbar.Total <= v.scrollbar.Len {
		v.overlay.hide()
		t.stopScrollbarFade()
	}
	rules, highlightVersion := t.highlights, t.highlightVersion
	t.mu.Unlock()
	rs := t.rs
	if !held {
		rs.EndUpdate()
	}
	colors := rs.Colors()
	full := rs.Dirty() == vt.Full || colors != v.colors || v.highlightVersion != highlightVersion || (!held && v.bidiDirty) || linksChanged
	v.rules = rules
	v.highlightVersion = highlightVersion
	v.colors, v.cursor = colors, rs.Cursor()
	cols, rows := rs.Size()
	if len(v.lines) != rows {
		v.lines = append(v.lines[:0], make([]rowCache, rows)...)
		full = true
	}
	rs.Rows()
	for y := 0; y < rows && rs.NextRow(); y++ {
		line := &v.lines[y]
		if full || !line.ok || rs.RowDirty() {
			var presentation vt.BidiPresentation
			if y < len(v.bidiFrame) {
				presentation = v.bidiFrame[y]
			}
			v.buildRow(line, rs, cols, presentation, v.hoveredLinkColumns(y))
		}
	}
	if !held {
		v.bidiDirty = false
		v.prepareGraphics()
	}
	rs.Clean()
	v.draw(p, r)
}

// resolved is a cell's style with its colors resolved.
type resolved struct {
	fg, bg, ul       ui.Color
	hasBg, hasUl     bool
	variant          int // 1 bold, 2 italic
	invisible        bool
	underline        int
	strike, overline bool
}

func (v *view) resolve(s vt.Style) resolved {
	pal := &v.colors.Palette
	col := func(c vt.Color, def vt.RGB) (ui.Color, bool) {
		switch c.Kind {
		case 1:
			return color(pal[c.Index]), true
		case 2:
			return color(c.RGB), true
		}
		return color(def), false
	}
	var r resolved
	r.fg, _ = col(s.FG, v.colors.Foreground)
	r.bg, r.hasBg = col(s.BG, v.colors.Background)
	r.ul, r.hasUl = col(s.UnderlineColor, v.colors.Foreground)
	if s.Inverse {
		r.fg, r.bg, r.hasBg = r.bg, r.fg, true
	}
	if s.Faint {
		r.fg = r.fg.Alpha(0.5)
	}
	if s.Bold {
		r.variant |= 1
	}
	if s.Italic {
		r.variant |= 2
	}
	r.invisible, r.underline, r.strike, r.overline = s.Invisible, s.Underline, s.Strikethrough, s.Overline
	return r
}

// buildRow makes the cache of the render state's current row.
func (v *view) buildRow(line *rowCache, rs *vt.RenderState, cols int, presentation vt.BidiPresentation, link columnSpan) {
	*line = rowCache{ok: true, mode: rs.RowLineMode(), bgs: line.bgs[:0], runs: line.runs[:0], decos: line.decos[:0], boxes: line.boxes[:0], placeholders: line.placeholders[:0], wide: line.wide[:0], selected: line.selected[:0], bidi: presentation.Row}
	cells := rs.RowCells()
	highlights := v.rowHighlights(rs, cells)
	line.selA, line.selB, line.sel = rs.RowSelection()
	def := resolved{fg: color(v.colors.Foreground), bg: color(v.colors.Background)}
	var styles []struct {
		id uint16
		r  resolved
	}
	styleOf := func(c vt.Cell, x int) resolved {
		if c.Style == 0 {
			return def
		}
		for _, s := range styles {
			if s.id == c.Style {
				return s.r
			}
		}
		r := v.resolve(rs.CellStyle(x))
		styles = append(styles, struct {
			id uint16
			r  resolved
		}{c.Style, r})
		return r
	}
	var run struct {
		text    []rune
		cols    []int16 // the column of each rune
		widths  []int8
		fg      ui.Color
		variant int
	}
	flush := func() {
		v.shapeRun(line, run.text, run.cols, run.widths, run.fg, run.variant)
		run.text, run.cols, run.widths = run.text[:0], run.cols[:0], run.widths[:0]
	}
	var buf []rune
	var previousPlaceholder *kittyPlaceholder
	for index, x := 0, 0; x < len(cells) && x < cols; index++ {
		logical, display := x, ""
		if len(line.bidi.Cells) > 0 {
			if index >= len(line.bidi.Cells) {
				break
			}
			logical, display = line.bidi.Cells[index].Column, line.bidi.Cells[index].Display
		}
		c := cells[logical]
		if len(line.bidi.Cells) == 0 && (c.Wide == vt.SpacerTail || c.Wide == vt.SpacerHead) {
			x++
			continue
		}
		w := 1
		if c.Wide == vt.WideChar {
			w = 2
			line.wide = append(line.wide, int16(x))
		}
		if line.sel && logical <= line.selB && logical+w > line.selA {
			n := len(line.selected)
			if n > 0 && line.selected[n-1].end == x {
				line.selected[n-1].end = x + w
			} else {
				line.selected = append(line.selected, columnSpan{x, x + w})
			}
		}
		st := styleOf(c, logical)
		if link.end > link.start && logical < link.end && logical+w > link.start {
			st.underline, st.ul, st.hasUl = vt.UnderlineDashed, st.fg, true
		}
		if c.HasBackground {
			bg := c.Background
			if c.Palette {
				bg = v.colors.Palette[c.Background.R]
			}
			st.bg, st.hasBg = color(bg), true
		}
		if h, ok := highlights[logical]; ok {
			if h.Foreground.A > 0 {
				st.fg = h.Foreground
			}
			if h.Background.A > 0 {
				st.bg = h.Background
				st.hasBg = true
			}
			if h.Bold {
				st.variant |= 1
			}
		}
		if st.hasBg && st.bg != def.bg {
			if n := len(line.bgs); n > 0 && line.bgs[n-1].x1 == x && line.bgs[n-1].c == st.bg {
				line.bgs[n-1].x1 = x + w
			} else {
				line.bgs = append(line.bgs, bgSpan{x, x + w, st.bg})
			}
		}
		if st.underline != 0 {
			v.addDeco(line, x, x+w, st.underline, st.ulColor())
		}
		if st.strike {
			v.addDeco(line, x, x+w, decoStrike, st.fg)
		}
		if st.overline {
			v.addDeco(line, x, x+w, decoOverline, st.fg)
		}
		// Placeholders remain images even when SGR hides or blinks text.
		// Decode before resolving colors; highlight/selection do not change IDs.
		if c.Codepoint == kittyPlaceholderScalar {
			chars := []rune{c.Codepoint}
			if c.Grapheme {
				chars = rs.CellGraphemes(logical, nil)
			}
			if placeholder, ok := decodeKittyPlaceholder(chars, rs.CellStyle(logical), x, previousPlaceholder); ok {
				line.placeholders = append(line.placeholders, placeholder)
				previousPlaceholder = &line.placeholders[len(line.placeholders)-1]
				c.Codepoint, c.Grapheme, display = ' ', false, ""
			}
		} else {
			previousPlaceholder = nil
		}
		cp := c.Codepoint
		if cp == 0 || st.invisible {
			cp = ' '
		}
		buf = buf[:0]
		if c.Grapheme && cp != ' ' {
			buf = rs.CellGraphemes(logical, buf)
		}
		if len(buf) == 0 {
			buf = append(buf, cp)
		}
		if display != "" && !st.invisible {
			buf = append(buf[:0], []rune(display)...)
		}
		if len(buf) == 1 && drawn(buf[0]) {
			line.boxes = append(line.boxes, boxCell{x, buf[0], st.fg})
			buf[0] = ' '
		}
		isolated := len(line.bidi.Cells) > 0 && bidilayout.Isolate(string(buf))
		if len(line.bidi.Cells) > 0 {
			st.variant |= 8
		}
		emoji := v.preferColorEmoji && core.ColorEmojiCluster(buf)
		if isolated || emoji {
			flush()
		}
		if emoji {
			st.variant |= 4
		}
		if len(run.text) > 0 && (st.variant != run.variant || st.fg != run.fg) {
			if cp == ' ' {
				// A space draws nothing: it can join any run.
				st.variant, st.fg = run.variant, run.fg
			} else {
				flush()
			}
		}
		if len(run.text) == 0 {
			run.fg, run.variant = st.fg, st.variant
		}
		for i, r := range buf {
			run.text = append(run.text, r)
			run.cols = append(run.cols, int16(x))
			ww := int8(0) // the runes after the first share its cell
			if i == 0 {
				ww = int8(w)
			}
			run.widths = append(run.widths, ww)
		}
		if isolated || emoji {
			flush()
		}
		x += w
	}
	flush()
}

func (r resolved) ulColor() ui.Color {
	if r.hasUl {
		return r.ul
	}
	return r.fg
}

func (v *view) addDeco(line *rowCache, x0, x1, kind int, c ui.Color) {
	if n := len(line.decos); n > 0 {
		d := &line.decos[n-1]
		if d.x1 == x0 && d.kind == kind && d.c == c {
			d.x1 = x1
			return
		}
	}
	line.decos = append(line.decos, deco{x0, x1, kind, c})
}

// shapeRun shapes a run of cells and places its glyphs in their cells.
func (v *view) shapeRun(line *rowCache, text []rune, cols []int16, widths []int8, fg ui.Color, variant int) {
	// Trailing spaces draw nothing.
	for len(text) > 0 && text[len(text)-1] == ' ' {
		text, cols, widths = text[:len(text)-1], cols[:len(cols)-1], widths[:len(widths)-1]
	}
	if len(text) == 0 {
		return
	}
	shaped := v.shape(string(text), variant, 1)
	cw := float32(v.cellW) / v.scale
	out := glyphRun{glyphs: make([]ui.Glyph, 0, len(shaped)), cols: make([]int16, 0, len(shaped)), c: fg}
	// The glyphs of a cell, or of the cells a ligature spans, keep their
	// places relative to each other and are centered in the cells, as the
	// glyphs of fallback fonts, wider or narrower than a cell, are. Those
	// too wide, as emoji are, shrink to fit, as in Ghostty.
	for i := 0; i < len(shaped); {
		first := max(0, min(shaped[i].Cluster, len(cols)-1))
		col := cols[first]
		last := first // the last rune of the group
		x0, x1 := shaped[i].X, shaped[i].X+shaped[i].Advance
		j := i
		for {
			g := shaped[j]
			last = max(last, min(g.Cluster+max(g.Runes, 1), len(cols))-1)
			x0, x1 = min(x0, g.X), max(x1, g.X+g.Advance)
			if j+1 >= len(shaped) || cols[max(0, min(shaped[j+1].Cluster, len(cols)-1))] != col {
				break
			}
			j++
		}
		span := 0
		for k := first; k <= last; k++ {
			span += int(widths[k])
		}
		room := float32(max(span, 1)) * cw
		group := shaped[i : j+1]
		if f := room / (x1 - x0); f < 0.95 && x1 > x0 {
			// Too wide: the same text, smaller.
			small := v.shape(string(text[first:last+1]), variant, max(f, 0.5))
			if len(small) > 0 {
				group = small
				x0, x1 = small[0].X, small[0].X+small[0].Advance
				for _, g := range small {
					x0, x1 = min(x0, g.X), max(x1, g.X+g.Advance)
				}
			}
		}
		off := (room - (x1 - x0)) / 2
		if math.Abs(float64(off)) < 0.5/float64(v.scale) {
			off = 0
		}
		cellX := float32(col) * cw
		for _, gl := range group {
			gl.X = cellX + off + (gl.X - x0)
			out.glyphs = append(out.glyphs, gl)
			out.cols = append(out.cols, col)
		}
		i = j + 1
	}
	line.runs = append(line.runs, out)
}

// shape shapes text in a variant of the font, at a fraction of its size,
// from the cache.
func (v *view) shape(text string, variant int, scale float32) []ui.Glyph {
	k := shapeKey{text, variant, scale}
	if g, ok := v.shaped.get(k); ok {
		return g
	}
	f := v.fonts[variant&3]
	if variant&4 != 0 {
		f.Family = core.ColorEmojiFamily()
	}
	f.Size *= scale
	var g []ui.Glyph
	if variant&8 == 0 {
		g = ui.Shape(text, f)
	} else {
		// The terminal has already resolved and mirrored this visual run.
		// Override host BiDi so it cannot mirror punctuation a second time.
		n := utf8.RuneCountInString(text)
		for _, glyph := range ui.Shape("\u202d"+text+"\u202c", f) {
			if glyph.Cluster < 1 || glyph.Cluster > n {
				continue
			}
			glyph.Cluster--
			glyph.Runes = min(glyph.Runes, n-glyph.Cluster)
			g = append(g, glyph)
		}
	}
	v.shaped.put(k, g)
	return g
}

// draw draws the rows and the cursor.
func (v *view) draw(p *ui.Painter, r ui.Rect) {
	s := v.scale
	cw, ch := float32(v.cellW)/s, float32(v.cellH)/s
	// The grid starts at a whole pixel of the display.
	ox := (float32(math.Round(float64(r.X*s))) + float32(v.ox)) / s
	oy := (float32(math.Round(float64(r.Y*s))) + float32(v.oy)) / s
	base := float32(v.baseline) / s
	if !v.t.opts.Transparent {
		p.Fill(r, color(v.colors.Background), 0)
	}
	v.drawGraphics(p, ox, oy, 1)
	selection := v.theme.Selection
	if selection.A == 0 {
		selection = color(v.colors.Foreground).Alpha(0.3)
	}
	for y := range v.lines {
		line := &v.lines[y]
		ry := oy + float32(y)*ch
		lcw := cw * lineHorizontalScale(line.mode)
		background := func() {
			for _, b := range line.bgs {
				p.Fill(ui.Rect{X: ox + float32(b.x0)*lcw, Y: ry, W: float32(b.x1-b.x0) * lcw, H: ch}, b.c, 0)
			}
			for _, span := range line.selected {
				p.Fill(ui.Rect{X: ox + float32(span.start)*lcw, Y: ry, W: float32(span.end-span.start) * lcw, H: ch}, selection, 0)
			}
		}
		if line.mode != vt.LineSingle {
			p.Clip(ui.Rect{X: ox, Y: ry, W: float32(v.cols) * cw, H: ch}, 0, background)
		} else {
			background()
		}
	}
	v.drawGraphics(p, ox, oy, 2)
	cur, curColor, curText, block := v.cursorRect(ox, oy, cw, ch)
	if block {
		p.Fill(cur, curColor, 0)
	}
	for y := range v.lines {
		line := &v.lines[y]
		ry := oy + float32(y)*ch
		text := func(over ui.Color) {
			if line.mode == vt.LineSingle {
				v.drawText(p, line, ox, ry, cw, ch, base, over)
				return
			}
			clip := ui.Rect{X: ox, Y: oy, W: float32(v.cols) * cw, H: float32(v.rows) * ch}
			if line.mode != vt.LineDoubleWidth {
				clip.Y, clip.H = ry, ch
			}
			p.Clip(clip, 0, func() { v.drawText(p, line, ox, ry, cw, ch, base, over) })
		}
		if !line.sel || v.theme.SelectionText.A == 0 {
			text(ui.Color{})
			continue
		}
		// The selected text takes the selection's color in its cells,
		// as far as its glyphs reach.
		lcw := cw * lineHorizontalScale(line.mode)
		at := r.X
		for _, span := range line.selected {
			a, b := ox+float32(span.start)*lcw, ox+float32(span.end)*lcw
			if a > at {
				p.Clip(ui.Rect{X: at, Y: r.Y, W: a - at, H: r.H}, 0, func() { text(ui.Color{}) })
			}
			p.Clip(ui.Rect{X: a, Y: r.Y, W: b - a, H: r.H}, 0, func() { text(v.theme.SelectionText) })
			at = b
		}
		if at < r.X+r.W {
			p.Clip(ui.Rect{X: at, Y: r.Y, W: r.X + r.W - at, H: r.H}, 0, func() { text(ui.Color{}) })
		}
	}
	v.drawKittyPlaceholders(p, ox, oy)
	v.drawGraphics(p, ox, oy, 3)
	if block && len(v.graphics) != 0 {
		p.Fill(cur, curColor, 0) // The caret stays visible above terminal images.
	}
	if cur.W > 0 && !block {
		if v.focused || v.cursor.Style != vt.CursorBlock {
			p.Fill(cur, curColor, 0)
		} else {
			p.Stroke(cur, curColor, 0, 1/s)
		}
	}
	if block && v.cursor.Y < len(v.lines) {
		// The text under a block cursor, in the cursor's text color.
		line := &v.lines[v.cursor.Y]
		ry := oy + float32(v.cursor.Y)*ch
		p.Clip(cur, 0, func() {
			v.drawText(p, line, ox, ry, cw, ch, base, curText)
		})
	}
	if v.preedit != "" && v.focused {
		v.drawPreedit(p, ox, oy, cw, ch, base, max(cw, r.W-2*padX), r.Y+r.H)
	}
	v.drawScrollbar(p, r)
	v.drawLinkPreview(p, r)
}

// drawText draws the glyphs, drawn characters and lines of a row, in their
// colors or, when over is not zero, in over.
func (v *view) drawText(p *ui.Painter, line *rowCache, ox, ry, cw, ch, base float32, over ui.Color) {
	sx, sy, offset := lineTransform(line.mode, ch)
	ry += offset
	pick := func(c ui.Color) ui.Color {
		if over.A > 0 {
			return over
		}
		return c
	}
	for _, run := range line.runs {
		p.GlyphsScaled(run.glyphs, ox, ry+base*sy, sx, sy, pick(run.c))
	}
	cw, ch, base = cw*sx, ch*sy, base*sy
	for _, b := range line.boxes {
		drawBox(p, b.r, ui.Rect{X: ox + float32(b.x)*cw, Y: ry, W: cw, H: ch}, pick(b.c))
	}
	for _, d := range line.decos {
		d.c = pick(d.c)
		v.drawDeco(p, d, ox, ry, cw, ch, base, sx, sy)
	}
}

// cursorRect returns the cursor's rectangle and colors, and whether it is
// a filled block, whose text shows in the cursor's text color. A cursor
// that does not show has an empty rectangle.
func (v *view) cursorRect(ox, oy, cw, ch float32) (r ui.Rect, c, text ui.Color, block bool) {
	cu := v.cursor
	if !cu.InView || !cu.Visible || v.preedit != "" {
		return
	}
	if v.focused && cu.Blinking && time.Since(v.blinkStart)%(2*blinkPeriod) >= blinkPeriod {
		return
	}
	c = color(v.colors.Foreground)
	if v.colors.HasCursor {
		c = color(v.colors.Cursor)
	} else if v.theme.Cursor.A > 0 {
		c = v.theme.Cursor
	}
	text = color(v.colors.Background)
	if v.theme.CursorText.A > 0 {
		text = v.theme.CursorText
	}
	x := cu.X
	if cu.WideTail && x > 0 {
		x--
	}
	x = v.visualColumn(cu.Y, x)
	gridWidth := float32(v.cols) * cw
	if cu.Y >= 0 && cu.Y < len(v.lines) {
		cw *= lineHorizontalScale(v.lines[cu.Y].mode)
	}
	if v.theme.AutoCursor {
		background := color(v.colors.Background)
		if cu.Y >= 0 && cu.Y < len(v.lines) {
			for _, span := range v.lines[cu.Y].bgs {
				if x >= span.x0 && x < span.x1 {
					background = span.c
					break
				}
			}
		}
		c = CursorContrast(background)
		text = CursorContrast(c)
	}
	w := cw
	if v.wideAt(cu.Y, x) {
		w *= 2
	}
	r = ui.Rect{X: ox + float32(x)*cw, Y: oy + float32(cu.Y)*ch, W: w, H: ch}
	if float32(x)*cw >= gridWidth {
		return ui.Rect{}, c, text, false
	}
	r.W = min(r.W, gridWidth-float32(x)*cw)
	px := 1 / v.scale
	style := cu.Style
	if !v.focused && style == vt.CursorBlock {
		style = vt.CursorBlockHollow
	}
	switch style {
	case vt.CursorBar:
		r.W = max(float32(math.Round(float64(1.5*v.scale))), 1) * px
	case vt.CursorUnderline:
		h := max(float32(math.Round(float64(1.5*v.scale))), 1) * px
		r.Y, r.H = r.Y+r.H-h, h
	case vt.CursorBlockHollow:
		return r, c, text, false // stroked
	default:
		block = true
	}
	return r, c, text, block
}

// wideAt reports whether the cell at column x of row y holds a wide
// character.
func (v *view) wideAt(y, x int) bool {
	return y < len(v.lines) && slices.Contains(v.lines[y].wide, int16(x))
}

// drawDeco draws an underline, a strikethrough or an overline.
func (v *view) drawDeco(p *ui.Painter, d deco, ox, ry, cw, ch, base, sx, sy float32) {
	px := 1 / v.scale
	stroke := max(float32(math.Round(float64(v.font.size*v.scale/14))), 1) * px
	thick := stroke * sy
	x0, x1 := ox+float32(d.x0)*cw, ox+float32(d.x1)*cw
	ul := ry + base + max(float32(math.Round(float64(v.font.size*v.scale/10))), 1)*px*sy
	switch d.kind {
	case decoStrike:
		p.Fill(ui.Rect{X: x0, Y: ry + base - v.font.size*0.3*sy, W: x1 - x0, H: thick}, d.c, 0)
	case decoOverline:
		p.Fill(ui.Rect{X: x0, Y: ry, W: x1 - x0, H: thick}, d.c, 0)
	case vt.UnderlineDouble:
		p.Fill(ui.Rect{X: x0, Y: ul - thick, W: x1 - x0, H: thick}, d.c, 0)
		p.Fill(ui.Rect{X: x0, Y: ul + thick, W: x1 - x0, H: thick}, d.c, 0)
	case vt.UnderlineCurly:
		var path ui.Path
		amp := 1.5 * thick
		period := cw
		n := int((x1 - x0) / period * 8)
		path.MoveTo(x0, ul)
		for i := 1; i <= n; i++ {
			x := x0 + (x1-x0)*float32(i)/float32(n)
			path.LineTo(x, ul+amp*float32(math.Sin(float64((x-x0)/period*2*math.Pi))))
		}
		p.StrokePath(&path, thick, d.c)
	case vt.UnderlineDotted, vt.UnderlineDashed:
		step, dash := 2*stroke*sx, stroke*sx
		if d.kind == vt.UnderlineDashed {
			step, dash = cw/2, cw/4
		}
		for x := x0; x < x1; x += step {
			p.Fill(ui.Rect{X: x, Y: ul, W: min(dash, x1-x), H: thick}, d.c, 0)
		}
	default:
		p.Fill(ui.Rect{X: x0, Y: ul, W: x1 - x0, H: thick}, d.c, 0)
	}
}

// drawPreedit draws an input method's composition over the cursor.
func (v *view) drawPreedit(p *ui.Painter, ox, oy, cw, ch, base, width, bottom float32) {
	sx := float32(1)
	if y := v.cursor.Y; y >= 0 && y < len(v.lines) {
		sx = lineHorizontalScale(v.lines[y].mode)
	}
	indent := float32(v.visualColumn(v.cursor.Y, v.cursor.X)) * cw * sx
	lines := v.preeditShape.lines(v.preedit, v.fonts[0], width, indent, cw)
	y := oy + float32(v.cursor.Y)*ch
	fg := color(v.colors.Foreground)
	for i, line := range lines {
		ly := y + float32(i)*ch
		if ly >= bottom {
			break
		}
		// Only used fragments cover the terminal: the prompt and the rest
		// of each row remain visible. The overlay never writes to the grid.
		p.Fill(ui.Rect{X: ox + line.X, Y: ly, W: line.Width, H: ch}, color(v.colors.Background), 0)
		p.Glyphs(line.Glyphs, ox, ly+base, fg)
	}
}

// The transparent overlay never reserves space or changes terminal geometry.
func (v *view) drawScrollbar(p *ui.Painter, r ui.Rect) {
	v.resetHiddenScrollbar()
	now := time.Now()
	alpha := v.overlay.alpha(now)
	if alpha == 0 {
		return
	}
	y, h := scrollbarKnob(v.scrollbar, r.H)
	if h == 0 {
		return
	}
	p.Fill(ui.Rect{X: r.X + r.W - 6, Y: r.Y + y + 2, W: 4, H: max(float32(0), h-4)}, color(v.colors.Foreground).Alpha(alpha), 2)
}

// lru caches shaped text.
type lru struct {
	cap  int
	m    map[shapeKey]*lruEntry
	head lruEntry // a ring of the entries, most recent first
}

type shapeKey struct {
	text    string
	variant int
	scale   float32
}

type lruEntry struct {
	k          shapeKey
	g          []ui.Glyph
	prev, next *lruEntry
}

func (c *lru) init(n int) {
	c.cap = n
	c.clear()
}

func (c *lru) clear() {
	c.m = map[shapeKey]*lruEntry{}
	c.head.prev, c.head.next = &c.head, &c.head
}

func (c *lru) get(k shapeKey) ([]ui.Glyph, bool) {
	e := c.m[k]
	if e == nil {
		return nil, false
	}
	c.unlink(e)
	c.front(e)
	return e.g, true
}

func (c *lru) put(k shapeKey, g []ui.Glyph) {
	if len(c.m) >= c.cap {
		last := c.head.prev
		c.unlink(last)
		delete(c.m, last.k)
	}
	e := &lruEntry{k: k, g: g}
	c.m[k] = e
	c.front(e)
}

func (c *lru) unlink(e *lruEntry) { e.prev.next, e.next.prev = e.next, e.prev }

func (c *lru) front(e *lruEntry) {
	e.prev, e.next = &c.head, c.head.next
	c.head.next.prev = e
	c.head.next = e
}

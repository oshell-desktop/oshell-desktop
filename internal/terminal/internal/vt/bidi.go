package vt

import (
	"slices"
	"unsafe"

	"oshell-desktop/internal/terminal/internal/bidilayout"
)

// SetBidiEnabled controls the paragraph presentation bridge. NewTerminal
// enables it for runtimes with the complete rendering/input bridge (ABI 32+).
// Disabling remains available for legacy-host tests. VT snapshots still export
// only the active screen and do not preserve per-paragraph BiDi attributes.
func (t *Terminal) SetBidiEnabled(enabled bool) bool {
	if oshellABI < 30 || oshellABI > 62 || t.set(1005, unsafe.Pointer(&enabled)) != nil {
		return false
	}
	t.bidiEnabled = enabled
	if !enabled {
		t.bidiCache = nil
		t.bidiCacheColumns = 0
	}
	return true
}

func (t *Terminal) BidiEnabled() bool { return t.bidiEnabled }

type rawRowCells struct {
	y, length uint32
	row       uint64
	ptr       uintptr
}

func (t *Terminal) bidiRawRow(y int) (uint64, []uint64, bool) {
	if oshellABI < 31 || oshellABI > 62 || y < 0 {
		return 0, nil, false
	}
	data := rawRowCells{y: uint32(y)}
	if t.get(1002, unsafe.Pointer(&data)) != nil || data.ptr == 0 || data.length > 65535 {
		return 0, nil, false
	}
	return data.row, append([]uint64(nil), unsafe.Slice((*uint64)(mem(data.ptr)), data.length)...), true
}

// BidiPresentation contains owned coordinates for one physical row. An empty
// Row means identity; RTL still records the paragraph's resolved direction.
type BidiPresentation struct {
	Row bidilayout.Row
	RTL bool
}

// BidiRows reads absolute screen rows without changing the viewport. Paragraph
// context includes history and offscreen continuations, up to Swift's 500-row
// limit. No GridRef or borrowed native memory escapes this call.
func (t *Terminal) BidiRows(first, count int) []BidiPresentation {
	if !t.bidiEnabled || count <= 0 || first < 0 {
		return nil
	}
	cols, _ := t.Size()
	total := int(t.Scrollbar().Total)
	count = min(count, max(0, total-first))
	out := make([]BidiPresentation, count)
	type metadata struct {
		state        bidilayout.State
		continuation bool
		raw          []uint64
	}
	meta := make(map[int]metadata)
	read := func(y int) metadata {
		if m, ok := meta[y]; ok {
			return m
		}
		m := metadata{state: bidilayout.Default}
		if raw, cells, found := t.bidiRawRow(y); found {
			m.raw = cells
			m.state = rawBidiState(raw)
			call(fnRawRowGet, uintptr(raw), 2, uintptr(unsafe.Pointer(&m.continuation)))
		} else if ref, found := t.screenCell(0, y); found {
			var raw uint64
			if ok(call(fnGridRefRow, uintptr(unsafe.Pointer(&ref)), uintptr(unsafe.Pointer(&raw)))) {
				m.state = rawBidiState(raw)
				call(fnRawRowGet, uintptr(raw), 2, uintptr(unsafe.Pointer(&m.continuation)))
			}
		}
		meta[y] = m
		return m
	}
	for y := first; y < first+count; {
		m := read(y)
		start, end := y, y
		if !m.state.Explicit {
			for start > 0 && end-start < 500 && read(start).continuation {
				start--
			}
			for end+1 < total && end-start < 500 && read(end+1).continuation {
				end++
			}
		}
		if end-start >= 500 {
			// Match the reference's logical fallback for oversized paragraphs.
			out[y-first].RTL = m.state.RTL
			y++
			continue
		}
		state := read(start).state
		identity := state.Explicit && !state.RTL
		if !state.Explicit && !state.RTL && oshellABI >= 31 {
			identity = true
			for row := start; row <= end && identity; row++ {
				raw := read(row).raw
				if len(raw) != cols {
					identity = false
					break
				}
				for _, c := range raw {
					if cell.tag.of(c) == 1 || cell.tag.of(c) < 2 && bidilayout.PotentialRTL(rune(cell.codepoint.of(c))) {
						identity = false
						break
					}
				}
			}
		}
		if identity {
			y = min(end+1, first+count)
			continue
		}
		cells := make([][]bidilayout.Cell, end-start+1)
		for row := start; row <= end; row++ {
			cells[row-start] = t.bidiCellsFromRaw(row, cols, read(row).raw)
		}
		layout, valid := t.bidiParagraph(cells, cols, read(start).state)
		for ; y <= end && y < first+count; y++ {
			p := &out[y-first]
			p.RTL = layout.RTL
			if valid && len(layout.Rows) != 0 {
				p.Row = layout.Rows[y-start]
			}
		}
	}
	return out
}

func (t *Terminal) bidiCells(y, cols int) []bidilayout.Cell {
	_, raw, _ := t.bidiRawRow(y)
	return t.bidiCellsFromRaw(y, cols, raw)
}

func (t *Terminal) bidiCellsFromRaw(y, cols int, rawCells []uint64) []bidilayout.Cell {
	ref, found := t.screenCell(0, y)
	if !found {
		return nil
	}
	limit := cols
	if len(rawCells) == cols {
		for limit > 0 {
			c := rawCells[limit-1]
			if cell.tag.of(c) < 2 && cell.codepoint.of(c) != 0 {
				break
			}
			limit--
		}
	}
	cells := make([]bidilayout.Cell, 0, limit)
	last := 0
	var buf [16]uint32
	for x := 0; x < limit; {
		ref.x = uint16(x)
		var raw uint64
		if x < len(rawCells) {
			raw = rawCells[x]
		} else if !ok(call(fnGridRefCell, uintptr(unsafe.Pointer(&ref)), uintptr(unsafe.Pointer(&raw)))) {
			break
		}
		width := 1
		if Wide(cell.wide.of(raw)) == WideChar && x+1 < cols {
			width = 2
		}
		cp := rune(0)
		if cell.tag.of(raw) < 2 {
			cp = rune(cell.codepoint.of(raw))
		}
		text := " "
		if cp != 0 {
			text = string(cp)
			last = len(cells) + 1
		}
		if cell.tag.of(raw) == 1 {
			var n uintptr
			chars := buf[:]
			r := call(fnGridRefGraphemes, uintptr(unsafe.Pointer(&ref)), uintptr(unsafe.Pointer(&chars[0])), uintptr(len(chars)), uintptr(unsafe.Pointer(&n)))
			if Result(int32(r)) == OutOfSpace && n > uintptr(len(chars)) {
				chars = make([]uint32, n)
				r = call(fnGridRefGraphemes, uintptr(unsafe.Pointer(&ref)), uintptr(unsafe.Pointer(&chars[0])), uintptr(len(chars)), uintptr(unsafe.Pointer(&n)))
			}
			if ok(r) && n > 0 && n <= uintptr(len(chars)) {
				runes := make([]rune, n)
				for i, c := range chars[:n] {
					runes[i] = rune(c)
				}
				text = string(runes)
				last = len(cells) + 1
			}
		}
		cells = append(cells, bidilayout.Cell{Column: x, Width: width, Text: text})
		x += width
	}
	return cells[:last]
}

type bidiCacheKey struct {
	hash  uint64
	cols  int
	state bidilayout.State
}
type bidiCachedLayout struct {
	rows   [][]bidilayout.Cell
	layout bidilayout.Layout
}

// Content keys survive scrolling, but every hit also compares exact cells.
// Returned maps are immutable to callers. The cache belongs to this terminal
// and is discarded on disable/free. As well as Swift's 256-entry limit, a
// column budget bounds the memory occupied by maps of unusually long lines.
func (t *Terminal) bidiParagraph(rows [][]bidilayout.Cell, cols int, state bidilayout.State) (bidilayout.Layout, bool) {
	h := uint64(14695981039346656037)
	add := func(n int) {
		for range 8 {
			h ^= uint64(byte(n))
			h *= 1099511628211
			n >>= 8
		}
	}
	for _, row := range rows {
		add(len(row))
		for _, c := range row {
			add(c.Column)
			add(c.Width)
			add(len(c.Text))
			for i := range len(c.Text) {
				h ^= uint64(c.Text[i])
				h *= 1099511628211
			}
		}
	}
	key := bidiCacheKey{h, cols, state}
	if entry, ok := t.bidiCache[key]; ok && slices.EqualFunc(entry.rows, rows, func(a, b []bidilayout.Cell) bool { return slices.Equal(a, b) }) {
		return entry.layout, true
	}
	layout, ok := bidilayout.Paragraph(rows, cols, state)
	if !ok {
		return layout, false
	}
	const maxCachedColumns = 262144
	cost := len(rows) * cols
	if cost > maxCachedColumns {
		return layout, true
	}
	if t.bidiCache == nil || len(t.bidiCache) >= 256 || t.bidiCacheColumns+cost > maxCachedColumns {
		t.bidiCache = make(map[bidiCacheKey]bidiCachedLayout)
		t.bidiCacheColumns = 0
	}
	if prior, ok := t.bidiCache[key]; ok {
		t.bidiCacheColumns -= len(prior.rows) * cols
	}
	owned := make([][]bidilayout.Cell, len(rows))
	for i, row := range rows {
		owned[i] = slices.Clone(row)
	}
	t.bidiCache[key] = bidiCachedLayout{owned, layout}
	t.bidiCacheColumns += cost
	return layout, true
}

func decodeBidiState(bits uint8) bidilayout.State {
	return bidilayout.State{Explicit: bits&1 != 0, Auto: bits&2 == 0, RTL: bits&4 != 0, MirrorBoxes: bits&8 != 0}
}

// BidiState returns future paragraphs' state and the global arrow-swap mode.
func (t *Terminal) BidiState() (bidilayout.State, bool) {
	var raw uint8
	if oshellABI < 30 || oshellABI > 62 || t.get(1001, unsafe.Pointer(&raw)) != nil {
		return bidilayout.Default, false
	}
	return decodeBidiState(raw), raw&16 != 0
}

func rawBidiState(raw uint64) bidilayout.State {
	var bits uint8
	if oshellABI >= 30 && oshellABI <= 62 {
		call(fnRawRowGet, uintptr(raw), 1001, uintptr(unsafe.Pointer(&bits)))
	}
	return decodeBidiState(bits)
}

func (r *RenderState) RowBidiState() bidilayout.State {
	var raw uint64
	r.rowGet(2, unsafe.Pointer(&raw))
	return rawBidiState(raw)
}

func (t *Terminal) BidiRowAt(y int) bidilayout.State {
	ref, found := t.CellAt(0, y)
	var raw uint64
	if !found || !ok(call(fnGridRefRow, uintptr(unsafe.Pointer(&ref)), uintptr(unsafe.Pointer(&raw)))) {
		return bidilayout.Default
	}
	return rawBidiState(raw)
}

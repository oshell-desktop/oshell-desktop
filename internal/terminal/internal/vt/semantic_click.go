package vt

import (
	"strconv"
	"unsafe"
)

type semanticClickPlan struct {
	x, y, kind, reportY uint32
	left                uint64
	vertical            int64
	right               uint64
}

// SemanticClickData translates an absolute logical cell into one owned packet.
// The native query rechecks the active buffer, input lifetime and prompt group.
// Callers must separately arbitrate gestures/modifiers and delayed click
// identity, and hold the terminal owner lock through querying and sending.
func (t *Terminal) SemanticClickData(x, y int) []byte {
	if oshellABI < 60 || oshellABI > 62 || x < 0 || y < 0 || uint64(x) > uint64(^uint32(0)) || uint64(y) > uint64(^uint32(0)) {
		return nil
	}
	plan := semanticClickPlan{x: uint32(x), y: uint32(y)}
	if t.get(1009, unsafe.Pointer(&plan)) != nil {
		return nil
	}
	if plan.kind == 4 {
		data := []byte("\x1b[<0;")
		data = strconv.AppendUint(data, uint64(plan.x)+1, 10)
		data = append(data, ';')
		data = strconv.AppendUint(data, uint64(plan.reportY), 10)
		return append(data, 'M')
	}
	left, right, up, down := "\x1b[D", "\x1b[C", "\x1b[A", "\x1b[B"
	switch plan.kind {
	case 1:
	case 2:
		left, right, up, down = "\x1bOD", "\x1bOC", "\x1bOA", "\x1bOB"
	case 3:
		left, right = "\x1b[1;1u", "\x1b[1u"
	default:
		return nil
	}
	vertical, count := down, plan.vertical
	if count < 0 {
		vertical, count = up, -count
	}
	// Native counts are bounded by uint32 rows * uint16 columns. Supported
	// hosts are 64 bit, so the complete packet length fits their int range.
	size := plan.left*uint64(len(left)) + uint64(count)*uint64(len(vertical)) + plan.right*uint64(len(right))
	data := make([]byte, 0, int(size))
	appendSequence := func(sequence string, count uint64) {
		for range count {
			data = append(data, sequence...)
		}
	}
	appendSequence(left, plan.left)
	appendSequence(vertical, uint64(count))
	appendSequence(right, plan.right)
	return data
}

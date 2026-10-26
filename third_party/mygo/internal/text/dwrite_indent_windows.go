//go:build windows && (amd64 || arm64)

package text

import (
	"sync"
	"syscall"
)

var (
	dwIndentOnce           sync.Once
	dwIndentVtbl           [7]uintptr
	iidIDWriteInlineObject = guid{0x8339fde3, 0x106f, 0x47ab, [8]byte{0x83, 0x73, 0x1c, 0x62, 0x95, 0xeb, 0x10, 0xb3}}
)

type dwIndentObject struct {
	vtbl  *[7]uintptr
	width float32
}

func newDWIndentObject(width float32) *dwIndentObject {
	dwIndentOnce.Do(func() {
		dwIndentVtbl = [7]uintptr{
			syscall.NewCallback(dwIndentQuery),
			syscall.NewCallback(rendererAddRef),
			syscall.NewCallback(rendererAddRef),
			syscall.NewCallback(rendererDrawInlineObject),
			syscall.NewCallback(dwIndentMetrics),
			syscall.NewCallback(dwIndentOverhang),
			syscall.NewCallback(dwIndentBreaks),
		}
	})
	return &dwIndentObject{vtbl: &dwIndentVtbl, width: width}
}

func dwIndentQuery(this, iid, out uintptr) uintptr {
	switch *(*guid)(ptr(iid)) {
	case iidIUnknown, iidIDWriteInlineObject:
		*(*uintptr)(ptr(out)) = this
		return 0
	}
	*(*uintptr)(ptr(out)) = 0
	return errNoInterface
}

func dwIndentMetrics(this, out uintptr) uintptr {
	m := (*struct {
		width, height, baseline float32
		sideways                int32
	})(ptr(out))
	m.width, m.height, m.baseline, m.sideways = (*dwIndentObject)(ptr(this)).width, 1, 1, 0
	return 0
}

func dwIndentOverhang(this, out uintptr) uintptr {
	*(*[4]float32)(ptr(out)) = [4]float32{}
	return 0
}

func dwIndentBreaks(this, before, after uintptr) uintptr {
	*(*uint32)(ptr(before)) = 2 // DWRITE_BREAK_CONDITION_MAY_NOT_BREAK
	*(*uint32)(ptr(after)) = 1  // DWRITE_BREAK_CONDITION_CAN_BREAK
	return 0
}

// Remove only the synthetic object and paragraph-direction mark. Every
// exposed range/cluster stays relative to the caller's original text.
func stripIndentPrefix(lines []shapedLine) {
	for i := range lines {
		line := &lines[i]
		line.start, line.end = max(0, line.start-2), max(0, line.end-2)
		runs := line.runs[:0]
		for _, run := range line.runs {
			if run.end <= 2 {
				continue
			}
			run.start, run.end = max(0, run.start-2), run.end-2
			glyphs := run.glyphs[:0]
			for _, glyph := range run.glyphs {
				if glyph.Cluster < 2 {
					continue
				}
				glyph.Cluster -= 2
				glyphs = append(glyphs, glyph)
			}
			run.glyphs = glyphs
			runs = append(runs, run)
		}
		line.runs = runs
	}
}

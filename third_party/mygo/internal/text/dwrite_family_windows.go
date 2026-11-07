package text

import (
	"strings"
	"syscall"
	"unsafe"
)

// GDI's family list can name a weight as its own family (e.g. Cascadia Code
// Light). DirectWrite groups that face under Cascadia Code instead. Resolve
// the installed face through GDI interop, retaining its actual weight and
// stretch, rather than falling through to an unrelated proportional font.
func (e *dwrite) legacyFamily(name string, weight int, italic bool) faceKey {
	key := faceKey{family: strings.ToLower(name), weight: weight, italic: italic}
	if match, ok := e.legacy[key]; ok {
		return match
	}
	match := e.findLegacyFamily(name, weight, italic)
	e.legacy[key] = match
	return match
}

type dwLogFont struct {
	height, width, escapement, orientation, weight       int32
	italic, underline, strikeout, charset                byte
	outPrecision, clipPrecision, quality, pitchAndFamily byte
	faceName                                             [32]uint16
}

func (e *dwrite) findLegacyFamily(name string, weight int, italic bool) faceKey {
	encoded, err := syscall.UTF16FromString(name)
	if err != nil || len(encoded) > 32 {
		return faceKey{}
	}
	lf := dwLogFont{weight: int32(weight), charset: 1 /* DEFAULT_CHARSET */}
	copy(lf.faceName[:], encoded)
	if italic {
		lf.italic = 1
	}
	var interop, font uintptr
	if failed(call(e.factory, 17 /* GetGdiInterop */, uintptr(unsafe.Pointer(&interop)))) {
		return faceKey{}
	}
	defer release(interop)
	if failed(call(interop, 3 /* CreateFontFromLOGFONT */, uintptr(unsafe.Pointer(&lf)), uintptr(unsafe.Pointer(&font)))) {
		return faceKey{}
	}
	defer release(font)
	// Do not accept a substituted family for an unknown name.
	var names uintptr
	var exists int32
	if failed(call(font, 9 /* GetInformationalStrings */, 11 /* WIN32_FAMILY_NAMES */, uintptr(unsafe.Pointer(&names)), uintptr(unsafe.Pointer(&exists)))) || exists == 0 {
		return faceKey{}
	}
	matched := false
	for i, n := uint32(0), uint32(call(names, 3 /* GetCount */)); i < n; i++ {
		var length uint32
		if failed(call(names, stringsGetStringLength, uintptr(i), uintptr(unsafe.Pointer(&length)))) {
			continue
		}
		buf := make([]uint16, length+1)
		if !failed(call(names, stringsGetString, uintptr(i), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))) && strings.EqualFold(syscall.UTF16ToString(buf), name) {
			matched = true
			break
		}
	}
	release(names)
	if !matched {
		return faceKey{}
	}
	var family, familyNames uintptr
	if failed(call(font, fontGetFontFamily, uintptr(unsafe.Pointer(&family)))) {
		return faceKey{}
	}
	defer release(family)
	if failed(call(family, familyGetFamilyNames, uintptr(unsafe.Pointer(&familyNames)))) {
		return faceKey{}
	}
	defer release(familyNames)
	return faceKey{family: englishName(familyNames), weight: int(call(font, 4 /* GetWeight */)), stretch: call(font, 5 /* GetStretch */), italic: call(font, 6 /* GetStyle */) != 0}
}

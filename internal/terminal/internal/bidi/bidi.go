// Derived from go-text/typesetting v0.3.5; see LICENSE.
// bidi implements the Unicode Bidi algorithm.
//
// The implementation is inspired by x/text/unicode/bidi, providing
// a similar API.
package bidi

import (
	ucd "oshell-desktop/internal/terminal/internal/bidi/ucd"
)

// Paragraph is the main entry point of the package.
//
// It stores internal data required to segment a string,
// and should be reused to reduce allocations.
type Paragraph struct {
	text []rune // input values

	initialTypes []ucd.BidiClass
	pairTypes    []bracketType
	pairValues   []rune

	embeddingLevel Level // default: = implicitLevel;

	// at the paragraph levels
	resultTypes  []ucd.BidiClass
	resultLevels []Level

	// Index of matching PDI for isolate initiator characters. For other
	// characters, the value of matchingPDI will be set to -1. For isolate
	// initiators with no matching PDI, matchingPDI will be set to the length of
	// the input string.
	matchingPDI []int

	// Index of matching isolate initiator for PDI characters. For other
	// characters, and for PDIs with no matching isolate initiator, the value of
	// matchingIsolateInitiator will be set to -1.
	matchingIsolateInitiator []int

	runForCharacter []int
}

// A Direction indicates the overall flow of text.
type Direction uint8

const (
	Neutral Direction = iota
	LeftToRight
	RightToLeft
)

// Initialize the p.pairTypes, p.pairValues and p.types from the input previously
// set by p.SetBytes() or p.SetString(). Also limit the input up to (and including) a paragraph
// separator (bidi class B).
func (p *Paragraph) prepareInput() (classesUnion ucd.BidiClass) {
	// reset storage

	if L := len(p.text); cap(p.pairTypes) >= L {
		p.pairTypes = p.pairTypes[:L]
		p.pairValues = p.pairValues[:L]
		p.initialTypes = p.initialTypes[:L]
		p.resultTypes = p.resultTypes[:L]
		p.resultLevels = p.resultLevels[:L]
		p.matchingPDI = p.matchingPDI[:L]
		p.matchingIsolateInitiator = p.matchingIsolateInitiator[:L]
		p.runForCharacter = p.runForCharacter[:L]
	} else {
		p.pairTypes = make([]bracketType, L)
		p.pairValues = make([]rune, L)
		p.initialTypes = make([]ucd.BidiClass, L)
		p.resultTypes = make([]ucd.BidiClass, L)
		p.resultLevels = make([]Level, L)
		p.matchingPDI = make([]int, L)
		p.matchingIsolateInitiator = make([]int, L)
		p.runForCharacter = make([]int, L)
	}

	// Try to short-circuit the full algorithm for single direction text
	classesUnion = ucd.BidiClass(0)

	for i, r := range p.text {
		class, bracket := ucd.LookupBidiClass(r)

		classesUnion |= class

		if class == ucd.BD_B {
			// Unlikely, but trim the arrays and exit
			p.text = p.text[:i]
			p.pairTypes = p.pairTypes[:i]
			p.pairValues = p.pairValues[:i]
			p.initialTypes = p.initialTypes[:i]
			p.resultTypes = p.resultTypes[:i]
			p.resultLevels = p.resultLevels[:i]
			p.matchingPDI = p.matchingPDI[:i]
			p.matchingIsolateInitiator = p.matchingIsolateInitiator[:i]
			p.runForCharacter = p.runForCharacter[:i]
			return
		}
		p.initialTypes[i] = class
		p.resultTypes[i] = class
		if bracket.IsOpening() {
			p.pairTypes[i] = bpOpen
			p.pairValues[i] = r
		} else if bracket.IsBracket() {
			// this must be a closing bracket,
			// since IsOpeningBracket is not true
			p.pairTypes[i] = bpClose
			p.pairValues[i] = bracket.Reverse(r)
		} else {
			p.pairTypes[i] = bpNone
			p.pairValues[i] = 0
		}
	}

	return classesUnion
}

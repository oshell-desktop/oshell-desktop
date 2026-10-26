// SPDX-License-Identifier: Unlicense OR BSD-3-Clause

package unicodedata

// BidiClass is a flag storing the Bidi_Class property used for UAX9.
type BidiClass uint32

// BidiBracket stores the matching bracket
type BidiBracket uint8

func (bb BidiBracket) IsBracket() bool { return bb&bidiBracketMask != 0 }

func (bb BidiBracket) IsOpening() bool { return bb&bidiBracketOpenMask != 0 }

func (bb BidiBracket) Reverse(r rune) rune {
	return bracketsXORMasks[bb&bidiBracketReverseMask] ^ r
}

// LookupBidiClass return the value of the Bidi_Class,
// or zero.
func LookupBidiClass(r rune) (BidiClass, BidiBracket) {
	i := bidiLookup(r)
	if i == 0 {
		return 0, 0
	}
	class := i & 0xFF
	return 1 << (class - 1), BidiBracket(i >> 8)
}

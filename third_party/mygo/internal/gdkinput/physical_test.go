package gdkinput

import (
	"github.com/egoist/mygo/internal/platform"
	"testing"
	"unsafe"
)

func TestPhysicalWritingPositions(t *testing.T) {
	for _, tc := range []struct {
		name string
		code uint16
		want rune
	}{
		{"AD01", 24, 'q'}, {"AC01", 38, 'a'}, {"AB01", 52, 'z'},
		{"AE01", 10, '1'}, {"AE12", 21, '='}, {"AD12", 35, ']'},
		{"AC11", 48, '\''}, {"AB10", 61, '/'}, {"TLDE", 49, '`'},
		{"BKSL", 51, '\\'}, {"SPCE", 65, ' '},
		{"LSGT", 94, 0}, {"KPEN", 104, 0}, {"RTRN", 36, 0}, {"FK01", 67, 0},
		{"AB00", 0, 0}, {"AD99", 65535, 0},
	} {
		if got := PhysicalXKBName([4]byte([]byte(tc.name))); got != tc.want {
			t.Errorf("XKB %s: %q, want %q", tc.name, got, tc.want)
		}
		if got := PhysicalWaylandKey(tc.code); got != tc.want {
			t.Errorf("Wayland %d: %q, want %q", tc.code, got, tc.want)
		}
	}
}

func TestXKBPhysicalDescriptionOwnsNames(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("LP64 native ABI")
	}
	var desc XKBDescription
	var names XKBNames
	if unsafe.Sizeof(desc) != 72 || unsafe.Offsetof(desc.Names) != 48 || unsafe.Offsetof(names.Keys) != 456 || unsafe.Sizeof(names) != 496 {
		t.Fatal("XKB LP64 ABI mismatch")
	}
	var keys [256][4]byte
	// Arbitrary server codes prove this does not assume an evdev assignment.
	keys[177] = [4]byte{'A', 'D', '0', '1'}
	keys[178] = [4]byte{'S', 'P', 'C', 'E'}
	keys[7] = [4]byte{'A', 'C', '0', '1'}
	desc.Min, desc.Max, desc.Names = 8, 255, &names
	names.Keys = &keys[0]
	got := CopyXKBPhysicalKeys(&desc)
	keys[177] = [4]byte{}
	if got[177] != 'q' || got[178] != ' ' || got[7] != 0 {
		t.Fatal("wrong or borrowed key names", got)
	}
	desc.Min, desc.Max = 9, 8
	for _, d := range []*XKBDescription{nil, {}, &desc} {
		if result := CopyXKBPhysicalKeys(d); result != [256]rune{} {
			t.Fatal("invalid description accepted")
		}
	}
}

func TestPrintablePhysicalIdentitySurvivesLayoutChange(t *testing.T) {
	var h functionalHarness
	for i, kind := range []platform.SurfaceEventKind{platform.KeyPressed, platform.KeyPressed, platform.KeyReleased} {
		base, physical := 'я', 'q'
		if i > 0 {
			base, physical = 'z', 'a'
		}
		if !h.keys.HandlePrintableLayout(kind, uint32(base), 177, platform.ModCtrl, base, base, physical, false, h.send, func() bool { return false }) {
			t.Fatal("unhandled")
		}
	}
	if len(h.events) != 3 {
		t.Fatal(h.events)
	}
	for i, e := range h.events {
		if e.Unshifted != 'я' || e.BaseLayout != 'q' || e.Repeat != (i == 1) {
			t.Fatal("held physical identity changed", e)
		}
	}
	h.events = nil
	h.keys.HandlePrintableLayout(platform.KeyPressed, 'a', 177, 0, 'a', 'a', 'a', false, h.send, func() bool { return false })
	if len(h.events) != 1 || h.events[0].BaseLayout != 0 {
		t.Fatal("redundant US identity", h.events)
	}
}

func TestPrintablePhysicalIdentityDoesNotLeakIntoComposition(t *testing.T) {
	for _, literal := range []bool{false, true} {
		var h functionalHarness
		text := "组合"
		if literal {
			text = "€"
		}
		h.keys.HandlePrintableLayout(platform.KeyPressed, '€', 26, platform.ModAlt, 'e', '€', 'e', literal, h.send, func() bool {
			h.send(platform.SurfaceEvent{Kind: platform.TextInput, Text: text})
			return true
		})
		h.keys.HandlePrintableLayout(platform.KeyReleased, '€', 26, 0, 0, 0, 0, false, h.send, func() bool { return false })
		if len(h.events) != 1 || h.events[0].Kind != platform.TextInput || h.events[0].Text != text || h.events[0].BaseLayout != 0 || h.events[0].Literal != literal {
			t.Fatal("composition acquired physical key", h.events)
		}
	}
}

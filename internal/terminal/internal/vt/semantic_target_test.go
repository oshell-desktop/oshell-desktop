package vt

import (
	"strings"
	"testing"
)

func TestOShellSemanticTargetLifetime(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	if _, ok := term.CaptureSemanticClickTarget(0); ok {
		t.Fatal("unarmed terminal captured a target")
	}
	term.Write([]byte("\x1b]133;A;cl=m\a\x1b]133;B\aabc\r\ndef"))
	target, ok := term.CaptureSemanticClickTarget(1)
	if !ok {
		t.Fatal("missing armed target")
	}
	term.Write([]byte("\x1b[H\x1b[L"))
	if y, ok := term.ResolveSemanticClickTarget(target); !ok || y != 2 {
		t.Fatal("whole row insertion did not preserve target", y, ok)
	}
	term.Write([]byte("\x1b[3;1H\x1b[2K"))
	if y, ok := term.ResolveSemanticClickTarget(target); !ok || y != 2 {
		t.Fatal("ordinary erasure invalidated target", y, ok)
	}
	term.Write([]byte("\x1b[M"))
	if _, ok := term.ResolveSemanticClickTarget(target); ok {
		t.Fatal("deleted target survived")
	}
	term.Reset()
	term.Write([]byte("\x1b]133;A;cl=m\a\x1b]133;B\ax"))
	target, ok = term.CaptureSemanticClickTarget(0)
	if !ok {
		t.Fatal("missing new target")
	}
	term.Write([]byte("\x1b[2J"))
	if y, ok := term.ResolveSemanticClickTarget(target); !ok || y != 0 {
		t.Fatal("ED erased the line incarnation", y, ok)
	}
	term.Write([]byte("\x1b[?47h"))
	if _, ok := term.ResolveSemanticClickTarget(target); ok {
		t.Fatal("primary target resolved in alternate")
	}
	term.Write([]byte("\x1b[?47l"))
	if y, ok := term.ResolveSemanticClickTarget(target); !ok || y != 0 {
		t.Fatal("primary identity disappeared after switch", y, ok)
	}
	other, _ := resetTerminal(t, Effects{})
	if _, ok := other.ResolveSemanticClickTarget(target); ok {
		t.Fatal("another terminal accepted target")
	}
	term.Reset()
	if _, ok := term.ResolveSemanticClickTarget(target); ok {
		t.Fatal("reset resurrected target")
	}
}

func TestOShellSemanticTargetCaptureDoesNotChangeReflow(t *testing.T) {
	for _, width := range []int{6, 18} {
		control, _ := resetTerminal(t, Effects{})
		captured, _ := resetTerminal(t, Effects{})
		for _, term := range []*Terminal{control, captured} {
			term.Write([]byte("\x1b]133;A;cl=m\a\x1b]133;B\aabc"))
		}
		target, ok := captured.CaptureSemanticClickTarget(4)
		if !ok {
			t.Fatal("cheap gate must not build geometry or reject outside-group rows")
		}
		for _, term := range []*Terminal{control, captured} {
			if err := term.Resize(width, 5, 8, 16); err != nil {
				t.Fatal(err)
			}
		}
		if control.Text() != captured.Text() || control.Scrollbar() != captured.Scrollbar() {
			t.Fatal("capturing an empty row changed reflow")
		}
		if y, ok := captured.ResolveSemanticClickTarget(target); !ok || y != 4 {
			t.Fatal("unchanged empty line lost identity", width, y, ok)
		}
	}
}

func TestOShellSemanticTargetRecycling(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	if err := term.SetHistoryLines(2); err != nil {
		t.Fatal(err)
	}
	term.Write([]byte("\x1b]133;A;cl=m\a\x1b]133;B\ax"))
	target, ok := term.CaptureSemanticClickTarget(0)
	if !ok {
		t.Fatal("missing target")
	}
	term.Write([]byte(strings.Repeat("\r\nnew", 40)))
	if term.Scrollbar().Total > 7 {
		t.Fatal("fixture did not trim history", term.Scrollbar())
	}
	if _, ok := term.ResolveSemanticClickTarget(target); ok {
		t.Fatal("recycled storage retained old incarnation")
	}
	term.Write([]byte("\r\n\x1b]133;N;cl=m\a\x1b]133;B\ax"))
	if _, ok := term.ResolveSemanticClickTarget(target); ok {
		t.Fatal("new prompt resurrected recycled target")
	}
}

func TestOShellSemanticTargetRemovedByReflow(t *testing.T) {
	term, _ := resetTerminal(t, Effects{})
	term.Write([]byte("\x1b]133;A;cl=m\a\x1b]133;B\aabcdefghijklmnop\r\nz"))
	first, ok := term.CaptureSemanticClickTarget(0)
	if !ok {
		t.Fatal("missing first line")
	}
	second, ok := term.CaptureSemanticClickTarget(1)
	if !ok {
		t.Fatal("missing second line")
	}
	if err := term.Resize(24, 5, 8, 16); err != nil {
		t.Fatal(err)
	}
	if y, ok := term.ResolveSemanticClickTarget(first); !ok || y != 0 {
		t.Fatal("surviving reflow line lost identity", y, ok)
	}
	if _, ok := term.ResolveSemanticClickTarget(second); ok {
		t.Fatal("removed reflow line accepted old target")
	}
}

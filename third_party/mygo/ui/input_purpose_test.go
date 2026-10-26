package ui

import "testing"

func TestPasswordInputPurposePreservesSelectionAndComposition(t *testing.T) {
	value := "a😀\"-'"
	hidden, readonly := true, false
	tt := coreNewTester(func(c *context) {
		e := coreTextInput(c, &value).Fill().AutoFocus().NoSpellcheck().ReadOnly(readonly)
		if hidden {
			e.Password()
		}
	}, 320, 120)
	defer tt.rt.close()
	if !tt.h.ime.Password || !tt.h.ime.NoSpellcheck {
		t.Fatal("password purpose not sent to host")
	}
	client := tt.h.ime.Client
	tt.Key(Cmd, KeyA)
	ed := tt.rt.states[tt.rt.focused].editor
	hidden = false
	tt.Frame()
	if tt.h.ime.Password || !tt.h.ime.NoSpellcheck || tt.h.ime.Client != client || ed.selectedText() != value {
		t.Fatal("reveal changed selection/client or lost no-spellcheck")
	}
	client.SetMarkedText(nil, "候補😀", TextInputRange{Start: 4, End: 4})
	hidden = true
	tt.Frame()
	if !tt.h.ime.Password || !tt.h.ime.NoSpellcheck || ed.compose != "候補😀" || tt.h.ime.Client != client {
		t.Fatal("hiding changed active composition or client")
	}
	client.ReplaceText(nil, "最终")
	tt.Frame()
	tt.Key(Cmd, KeyZ)
	if value != "a😀\"-'" {
		t.Fatal("purpose update broke atomic undo")
	}
	readonly = true
	tt.Frame()
	if tt.h.ime.Active || tt.h.ime.Password || tt.h.ime.NoSpellcheck {
		t.Fatal("readonly retained active input hints")
	}
}

func TestNoSpellcheckDoesNotLeakAcrossFrames(t *testing.T) {
	for _, indexed := range []bool{false, true} {
		value := "plain"
		doc := NewTextBuffer(value)
		noSpell := true
		tt := coreNewTester(func(c *context) {
			var e *node
			if indexed {
				e = coreTextInputBuffer(c, doc)
			} else {
				e = coreTextInput(c, &value)
			}
			e.Fill().AutoFocus()
			if noSpell {
				e.NoSpellcheck()
			}
		}, 320, 120)
		if !tt.h.ime.NoSpellcheck || tt.h.ime.Password {
			t.Fatal("explicit hint missing or masked input")
		}
		noSpell = false
		tt.Frame()
		if tt.h.ime.NoSpellcheck || !tt.h.ime.Active {
			t.Fatal("normal editor retained no-spellcheck")
		}
		tt.rt.close()
	}
}

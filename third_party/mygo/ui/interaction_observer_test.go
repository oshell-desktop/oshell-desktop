package ui

import "testing"

func TestWindowInteractionObserverDoesNotConsumeInput(t *testing.T) {
	enabled := true
	var seen []WindowInteraction
	draft := ""
	view := NewTester(func(c *Context) {
		if enabled {
			c.ObserveInteractions(func(ev WindowInteraction) { seen = append(seen, ev) })
		}
		TextInput(c, &draft).Label("editor").AutoFocus()
	}, 300, 100)
	view.Type("中文")
	if draft != "中文" || len(seen) != 1 || !seen[0].Keyboard {
		t.Fatal("input intercepted", draft, seen)
	}
	view.RightClickAt(240, 80)
	if event := seen[len(seen)-1]; !event.Pointer || event.X != 240 || event.Y != 80 {
		t.Fatal("pointer coordinates", event)
	}
	view.Scroll(230, 70, 0, 20)
	if event := seen[len(seen)-1]; !event.Pointer || event.X != 230 || event.Y != 70 {
		t.Fatal("scroll coordinates", event)
	}
	view.SetFocused(false)
	if !seen[len(seen)-1].Blur {
		t.Fatal("missing focus-loss event")
	}
	enabled = false
	view.Frame()
	count := len(seen)
	view.SetFocused(true)
	view.Type("x")
	if len(seen) != count || draft != "中文x" {
		t.Fatal("omitted observer retained, or input consumed", seen, draft)
	}
}

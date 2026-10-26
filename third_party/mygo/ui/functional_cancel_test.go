package ui

import (
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

func TestFunctionalCancelBeforeNextFrame(t *testing.T) {
	for _, mode := range []string{"blur-return", "switch-return", "disable", "ordinary"} {
		t.Run(mode, func(t *testing.T) {
			pending := false
			cancels, commits := [2]int{}, [2]int{}
			tt := NewTester(func(c *Context) {
				for i := range 2 {
					e := Box(c.Key(i)).Width(100).Height(50).Focusable().TextCaret(Rect{W: 1, H: 20}).FunctionalInput(mode != "ordinary").HandleInput(func(ev InputEvent) bool {
						switch ev.Kind {
						case InputKeyDown:
							pending = true
						case InputCancel:
							pending = false
							cancels[i]++
						case InputText:
							commits[i]++
						}
						return true
					})
					if i == 0 {
						e.AutoFocus()
					}
				}
			}, 200, 100)
			defer tt.rt.close()
			first := tt.rt.focused
			tt.rt.event(platform.SurfaceEvent{Kind: platform.KeyPressed, Key: platform.KeyA})
			if !pending {
				t.Fatal("missing pending key")
			}
			switch mode {
			case "blur-return", "ordinary":
				tt.rt.event(platform.SurfaceEvent{Kind: platform.SurfaceBlur})
				tt.rt.event(platform.SurfaceEvent{Kind: platform.SurfaceFocus})
			case "switch-return":
				for id, s := range tt.rt.states {
					if id != first && s.functionalInput {
						tt.rt.focused = id
						break
					}
				}
				tt.rt.updateTextInput()
				tt.rt.focused = first
				tt.rt.updateTextInput()
			case "disable":
				tt.rt.states[first].functionalInput = false
				tt.rt.updateTextInput()
			}
			// No settle/frame between loss and regain: a rendered Focused() value
			// alone cannot observe this lifecycle boundary.
			if mode == "ordinary" {
				if cancels != [2]int{} {
					t.Fatalf("ordinary handler received cancellation: %v", cancels)
				}
				return
			}
			if pending || cancels[0] != 1 {
				t.Fatalf("pending=%v cancels=%v", pending, cancels)
			}
			tt.rt.updateTextInput()
			if cancels[0] != 1 {
				t.Fatal("repeated caret update cancelled again")
			}
			tt.rt.event(platform.SurfaceEvent{Kind: platform.TextInput, Text: "中文"})
			if commits != [2]int{1, 0} {
				t.Fatalf("new commit routed incorrectly: %v", commits)
			}
		})
	}
}

func TestFunctionalCancelBeforeViewIsRemoved(t *testing.T) {
	visible, pending := true, false
	cancels := 0
	tt := NewTester(func(c *Context) {
		if visible {
			Box(c).Fill().Focusable().AutoFocus().TextCaret(Rect{W: 1, H: 20}).FunctionalInput(true).HandleInput(func(ev InputEvent) bool {
				if ev.Kind == InputKeyDown {
					pending = true
				}
				if ev.Kind == InputCancel {
					pending = false
					cancels++
				}
				return true
			})
		} else {
			Text(c, "another tab")
		}
	}, 200, 100)
	defer tt.rt.close()
	tt.rt.event(platform.SurfaceEvent{Kind: platform.KeyPressed, Key: platform.KeyA})
	visible = false
	tt.Frame()
	if pending || cancels != 1 {
		t.Fatalf("removed view retained pending input: pending=%v cancels=%d", pending, cancels)
	}
	visible = true
	tt.Frame()
	if pending || cancels != 1 {
		t.Fatal("recreated view revived or cancelled input again")
	}
}

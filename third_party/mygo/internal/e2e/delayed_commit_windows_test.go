//go:build windows && (amd64 || arm64)

package e2e

import (
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo"
	win "github.com/egoist/mygo/internal/windows"
	"github.com/egoist/mygo/ui"
)

func TestContentWindowDelayedCompositionCommit(t *testing.T) {
	for _, mode := range []string{"unchanged", "text", "selection", "new-preedit", "duplicate-end", "stale-double-end"} {
		t.Run(mode, func(t *testing.T) {
			var frames atomic.Int32
			client := &nativeInputClient{text: "cafe", selection: ui.TextInputSelection{Range: ui.TextInputRange{Start: 4, End: 4}}}
			w := newWindow(t, mygo.WindowOptions{Title: "Delayed composition", Width: 400, Height: 160, Content: ui.View(func(c *ui.Context) {
				ui.Box(c).Size(350, 70).HandleTextInput(client).AutoFocus().Label("Delayed composition test").Role(ui.RoleTextField)
				frames.Add(1)
			})})
			eventually(t, "input frame", func() bool { return frames.Load() > 0 })
			if !click(w, 120, 20) {
				t.Fatal("native input activation unavailable")
			}
			eventually(t, "input context", func() bool { _, text, ok := inputClient(w); return ok && text == "cafe" })
			if !composeOver(w, "候補", 2, false, 3, 1) {
				t.Fatal("native preedit failed")
			}
			var ended bool
			var beforeText string
			var beforeSelection ui.TextInputSelection
			var beforeMark *ui.TextInputRange
			mygo.RunOnMain(func() {
				ended = win.TestEndComposition(w.NativeHandle())
				switch mode {
				case "text", "stale-double-end":
					client.text = "safe!!"
				case "selection":
					client.selection.Range = ui.TextInputRange{Start: 0, End: 0}
				case "new-preedit":
					client.SetMarkedText(nil, "新", ui.TextInputRange{Start: 1, End: 1})
				}
				if mode == "duplicate-end" || mode == "stale-double-end" {
					ended = win.TestEndComposition(w.NativeHandle()) && ended
				}
				beforeText, beforeSelection, beforeMark = client.text, client.selection, client.mark
			})
			if !ended {
				t.Fatal("native composition end failed")
			}
			composeOver(w, "é", 1, true, -1, 0)
			mygo.RunOnMain(func() {
				if mode == "unchanged" || mode == "duplicate-end" {
					if client.text != "café" || client.mark != nil {
						t.Errorf("normal delayed commit: %q mark=%v", client.text, client.mark)
					}
				} else if client.text != beforeText || client.selection != beforeSelection || client.mark != beforeMark {
					t.Errorf("stale commit changed new state: %q selection=%+v mark=%v", client.text, client.selection, client.mark)
				}
			})
		})
	}
}

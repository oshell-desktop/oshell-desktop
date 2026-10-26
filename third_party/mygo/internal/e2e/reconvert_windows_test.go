//go:build windows && (amd64 || arm64)

package e2e

import (
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo"
	win "github.com/egoist/mygo/internal/windows"
	"github.com/egoist/mygo/ui"
)

func TestContentWindowStaleReconversion(t *testing.T) {
	for _, mode := range []string{"text", "selection", "empty-preview"} {
		t.Run(mode, func(t *testing.T) {
			var frames atomic.Int32
			client := &nativeInputClient{text: "cafe", selection: ui.TextInputSelection{Range: ui.TextInputRange{Start: 4, End: 4}}}
			w := newWindow(t, mygo.WindowOptions{Title: "Reconversion lifecycle", Width: 400, Height: 160, Content: ui.View(func(c *ui.Context) {
				ui.Box(c).Size(350, 70).HandleTextInput(client).AutoFocus().Label("Reconversion test").Role(ui.RoleTextField)
				frames.Add(1)
			})})
			eventually(t, "client frame", func() bool { return frames.Load() > 0 })
			if !click(w, 120, 20) {
				t.Fatal("Windows input activation unavailable")
			}
			eventually(t, "native context", func() bool { _, text, ok := inputClient(w); return ok && text == "cafe" })
			confirmed := false
			mygo.RunOnMain(func() {
				if mode == "empty-preview" {
					client.selection.Range = ui.TextInputRange{Start: 3, End: 4}
				}
				confirmed = win.TestConfirmReconversion(w.NativeHandle(), 3, 1)
				if mode == "text" {
					client.text = "safe"
				} else if mode == "selection" {
					client.selection.Range = ui.TextInputRange{Start: 1, End: 1}
				}
			})
			if !confirmed {
				t.Fatal("native confirmation failed")
			}
			if mode == "empty-preview" {
				composeOver(w, "", 0, false, -1, 0)
			} else {
				composeOver(w, "é", 1, true, -1, 0)
			}
			want := "safe"
			if mode != "text" {
				want = "cafe"
			}
			mygo.RunOnMain(func() {
				if client.text != want || client.mark != nil {
					t.Errorf("stale input mutated new document: %q mark=%v", client.text, client.mark)
				}
			})
			composeOver(w, "!", 1, true, -1, 0)
			want = "safe!"
			if mode == "selection" {
				want = "c!afe"
			} else if mode == "empty-preview" {
				want = "caf!"
			}
			mygo.RunOnMain(func() {
				if client.text != want {
					t.Errorf("fresh direct commit: %q want %q", client.text, want)
				}
			})
		})
	}
}

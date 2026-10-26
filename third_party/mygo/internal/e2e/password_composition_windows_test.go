//go:build windows && (amd64 || arm64)

package e2e

import (
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo"
	win "github.com/egoist/mygo/internal/windows"
	"github.com/egoist/mygo/ui"
)

func TestContentWindowPasswordDelayedCommit(t *testing.T) {
	for _, mode := range []string{"hidden", "reveal", "hide", "external"} {
		t.Run(mode, func(t *testing.T) {
			var frames atomic.Int32
			value := "native-base-"
			hidden := mode != "hide"
			w := newWindow(t, mygo.WindowOptions{Title: "Password composition", Width: 400, Height: 160, Content: ui.View(func(c *ui.Context) {
				ui.Column(c).Fill().Padding(20).Children(func() {
					e := ui.TextInput(c, &value).AutoFocus()
					if hidden {
						e.Password()
					}
				})
				frames.Add(1)
			})})
			eventually(t, "password frame", func() bool { return frames.Load() > 0 })
			if !click(w, 300, 36) {
				t.Fatal("native password activation unavailable")
			}
			if !composeOver(w, "候補😀", 3, false, -1, 0) {
				t.Fatal("native password preview failed")
			}
			ended := false
			mygo.RunOnMain(func() { ended = win.TestEndComposition(w.NativeHandle()) })
			if !ended {
				t.Fatal("native password composition end failed")
			}
			eventually(t, "unmarked candidate", func() bool { var ok bool; mygo.RunOnMain(func() { ok = value == "native-base-候補😀" }); return ok })
			if hidden {
				if _, doc, _ := inputClient(w); doc != "" {
					t.Fatal("committed password exposed to IMM")
				}
			}
			mygo.RunOnMain(func() {
				switch mode {
				case "reveal":
					hidden = false
				case "hide":
					hidden = true
				case "external":
					value = "changed"
				}
			})
			before := frames.Load()
			w.Invalidate()
			eventually(t, "visibility/value frame", func() bool { return frames.Load() > before })
			composeOver(w, "新", 1, true, -1, 0)
			want := "native-base-新"
			if mode == "external" {
				want = "changed"
			}
			eventually(t, "final password value", func() bool { var ok bool; mygo.RunOnMain(func() { ok = value == want }); return ok })
			if hidden {
				if _, doc, _ := inputClient(w); doc != "" {
					t.Fatal("password result exposed to IMM")
				}
			}
		})
	}
}

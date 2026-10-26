package ui

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

func TestPasswordDeferredCommitRejectsStalePrivateState(t *testing.T) {
	for _, mode := range []string{"text", "before-frame", "selection", "queued-selection", "new-preedit", "readonly", "binding", "focus"} {
		t.Run(mode, func(t *testing.T) {
			value, other := "base-", "base-候補"
			bound := &value
			readonly := false
			tt := coreNewTester(func(c *context) { coreTextInput(c, bound).Fill().AutoFocus().Password().ReadOnly(readonly) }, 300, 120)
			defer tt.rt.close()
			client := tt.h.ime.Client
			client.SetMarkedText(nil, "候補", TextInputRange{Start: 2, End: 2})
			var order platform.InputComposition
			order.End(client)
			tt.Frame()
			switch mode {
			case "text":
				value = "changed"
				tt.Frame()
			case "before-frame":
				value = "changed"
			case "selection":
				tt.Key(0, KeyLeft)
			case "queued-selection":
				tt.rt.event(platform.SurfaceEvent{Kind: platform.KeyPressed, Key: platform.KeyLeft})
			case "new-preedit":
				client.SetMarkedText(nil, "新", TextInputRange{Start: 1, End: 1})
			case "readonly":
				readonly = true
				tt.Frame()
			case "binding":
				bound = &other
				tt.Frame()
			case "focus":
				tt.rt.event(platform.SurfaceEvent{Kind: platform.SurfaceBlur})
			}
			before := *bound
			if order.Replace(client, nil, "旧") {
				t.Fatal("stale private commit accepted")
			}
			tt.Frame()
			if *bound != before {
				t.Fatal("stale commit changed synthetic password")
			}
			if text, _ := client.TextForRange(TextInputRange{End: 1000}); strings.Contains(text, "base") {
				t.Fatal("committed password exposed to native input")
			}
		})
	}
}

func TestPasswordDeferredCommitMapsSelectionAndMiddleInsertion(t *testing.T) {
	for _, all := range []bool{false, true} {
		value := "b😀ase-"
		tt := coreNewTester(func(c *context) { coreTextInput(c, &value).Fill().AutoFocus().Password() }, 300, 120)
		if all {
			tt.Key(Cmd, KeyA)
		} else {
			tt.Key(0, KeyHome)
			tt.Key(0, KeyRight)
			tt.Key(0, KeyRight)
		}
		client := tt.h.ime.Client
		client.SetMarkedText(nil, "候補😀", TextInputRange{Start: 4, End: 4})
		var order platform.InputComposition
		order.End(client)
		if text, _ := client.TextForRange(TextInputRange{End: 1000}); text != "" {
			t.Fatal("unmarked password exposed")
		}
		if !order.Replace(client, nil, "新") {
			t.Fatal("valid private commit rejected")
		}
		tt.Frame()
		want := "b😀新ase-"
		if all {
			want = "新"
		}
		if value != want {
			t.Fatalf("all=%v mapped result=%q want=%q", all, value, want)
		}
		tt.Key(Cmd, KeyZ)
		if value != "b😀ase-" {
			t.Fatal("selection composition did not undo atomically")
		}
		tt.rt.close()
	}
}

func TestPasswordBufferDeferredCommitRejectsExternalUpdate(t *testing.T) {
	doc := NewTextBuffer("base-")
	tt := coreNewTester(func(c *context) { coreTextInputBuffer(c, doc).Fill().AutoFocus().Password() }, 300, 120)
	defer tt.rt.close()
	client := tt.h.ime.Client
	client.SetMarkedText(nil, "候補", TextInputRange{Start: 2, End: 2})
	var order platform.InputComposition
	order.End(client)
	doc.Set("changed")
	if order.Replace(client, nil, "旧") {
		t.Fatal("indexed external edit accepted stale composition")
	}
	if doc.String() != "changed" {
		t.Fatal("stale commit overwrote indexed document")
	}
}

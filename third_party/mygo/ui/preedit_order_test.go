package ui

import (
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

// GtkIMContextSimple and GtkIMContextXIM clear preedit before emitting end
// and commit. The entire replacement must remain one undo transaction.
func TestPreeditEmptyBeforeCommitKeepsSingleUndo(t *testing.T) {
	for _, mode := range []string{"input", "password", "area"} {
		for _, refill := range []bool{false, true} {
			t.Run(mode+map[bool]string{false: "/clear", true: "/refill"}[refill], func(t *testing.T) {
				value := "before-😀-after"
				tt := coreNewTester(func(c *context) {
					if mode == "area" {
						coreTextArea(c, &value).Fill().AutoFocus()
					} else {
						e := coreTextInput(c, &value).Fill().AutoFocus()
						if mode == "password" {
							e.Password()
						}
					}
				}, 320, 120)
				defer tt.rt.close()
				tt.Key(Cmd, KeyA)
				client := tt.h.ime.Client
				var order platform.InputComposition
				order.Start(client)
				order.Preedit(client, nil, "候補😀", 3)
				order.Preedit(client, nil, "", 0)
				if refill {
					order.Preedit(client, nil, "新的候補", 4)
					order.Preedit(client, nil, "", 0)
				}
				order.End(client)
				if !order.Replace(client, nil, "最终😀") {
					t.Fatal("valid result rejected")
				}
				tt.Frame()
				if value != "最终😀" {
					t.Fatalf("wrong final text: %q", value)
				}
				tt.Key(Cmd, KeyZ)
				if value != "before-😀-after" {
					t.Fatalf("single undo lost original selection: %q", value)
				}
				tt.Key(Cmd|Shift, KeyZ)
				if value != "最终😀" {
					t.Fatalf("redo lost final commit: %q", value)
				}
			})
		}
	}
}

func TestPreeditNewSequenceDoesNotJoinCancelledUndo(t *testing.T) {
	value := "abc"
	tt := inputTester(&value, false)
	defer tt.rt.close()
	tt.Key(Cmd, KeyA)
	client := tt.h.ime.Client
	var order platform.InputComposition
	order.Start(client)
	order.Preedit(client, nil, "候補", 2)
	order.Preedit(client, nil, "", 0)
	order.End(client)
	order.Start(client)
	order.Preedit(client, nil, "新", 1)
	order.Preedit(client, nil, "", 0)
	order.End(client)
	order.Replace(client, nil, "new")
	tt.Frame()
	tt.Key(Cmd, KeyZ)
	if value != "" {
		t.Fatalf("new composition joined cancelled sequence: %q", value)
	}
	tt.Key(Cmd, KeyZ)
	if value != "abc" {
		t.Fatalf("original selection was lost: %q", value)
	}
}

func TestPreeditEmptyBufferCommitKeepsSingleUndo(t *testing.T) {
	for _, multiline := range []bool{false, true} {
		doc := NewTextBuffer("原始😀内容")
		tt := bufferTester(doc, multiline)
		client := tt.h.ime.Client
		tt.Key(Cmd, KeyA)
		var order platform.InputComposition
		order.Start(client)
		order.Preedit(client, nil, "候補", 2)
		order.Preedit(client, nil, "", 0)
		order.End(client)
		order.Replace(client, nil, "最终")
		tt.Frame()
		if doc.String() != "最终" {
			t.Fatalf("indexed commit: %q", doc.String())
		}
		tt.Key(Cmd, KeyZ)
		if doc.String() != "原始😀内容" {
			t.Fatalf("indexed single undo: %q", doc.String())
		}
		tt.rt.close()
	}
}

func TestPreeditNewStartDiscardsPreviousDeferredRange(t *testing.T) {
	c := &primitiveClient{text: "abc", selection: TextInputSelection{Range: TextInputRange{Start: 3, End: 3}}}
	tt := primitiveTester(c)
	defer tt.rt.close()
	client := tt.h.ime.Client
	var order platform.InputComposition
	order.Start(client)
	order.Preedit(client, nil, "😀", 1)
	order.End(client)
	order.Start(client)
	if !order.Replace(client, nil, "é") || c.text != "abc😀é" {
		t.Fatalf("new sequence replaced previous range: %q", c.text)
	}
}

func TestPreeditEmptyThenNavigationEndsUndoTransaction(t *testing.T) {
	value := "abc"
	tt := inputTester(&value, false)
	defer tt.rt.close()
	tt.Key(Cmd, KeyA)
	client := tt.h.ime.Client
	var order platform.InputComposition
	order.Preedit(client, nil, "候補", 2)
	order.Preedit(client, nil, "", 0)
	order.End(client)
	// A real key starts a separate edit after cancelled composition.
	tt.Key(0, KeyLeft)
	order.Replace(client, nil, "new")
	tt.Frame()
	tt.Key(Cmd, KeyZ)
	if value != "" {
		t.Fatalf("new input joined cancelled composition: %q", value)
	}
	tt.Key(Cmd, KeyZ)
	if value != "abc" {
		t.Fatalf("original selection was lost: %q", value)
	}
}

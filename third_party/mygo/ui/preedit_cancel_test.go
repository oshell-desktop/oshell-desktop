package ui

import (
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

func TestPreeditNativeResetDoesNotDeliverReentrantText(t *testing.T) {
	c := &primitiveClient{text: "abc", selection: TextInputSelection{Range: TextInputRange{Start: 3, End: 3}}}
	tt := primitiveTester(c)
	defer tt.rt.close()
	client := tt.h.ime.Client
	var order platform.InputComposition
	order.Preedit(client, nil, "候補", 2)
	order.End(client)
	before, changes, unmarks := c.text, c.changes, c.unmarks
	order.CancelNative(func() {
		// XIM reset can synchronously emit commit and changed. Start/end
		// and nested reset must also be harmless during native cancellation.
		if order.Replace(client, nil, "旧") {
			t.Error("reset delivered a commit")
		}
		order.Start(client)
		order.Preedit(client, nil, "新预览", 3)
		order.End(client)
		order.Reset()
		if !order.Canceling() {
			t.Error("ordinary reset cleared native cancellation guard")
		}
		order.CancelNative(func() { t.Error("recursive native reset") })
	})
	if c.text != before || c.changes != changes || c.unmarks != unmarks || c.mark != nil {
		t.Fatalf("reset mutated client: text=%q changes=%d unmarks=%d", c.text, c.changes, c.unmarks)
	}
	if order.Canceling() || !order.Replace(client, nil, "!") || c.text != before+"!" {
		t.Fatalf("fresh commit blocked or reused stale range: %q", c.text)
	}
}

func TestPreeditNativeResetRestoresGuardAfterPanic(t *testing.T) {
	var order platform.InputComposition
	func() {
		defer func() {
			if recover() == nil {
				t.Error("test panic missing")
			}
		}()
		order.CancelNative(func() { panic("synthetic reset failure") })
	}()
	if order.Canceling() {
		t.Fatal("reset failure permanently disabled composition")
	}
}

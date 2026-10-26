package e2e

import (
	"github.com/egoist/mygo"
	"testing"
)

func TestClipboardGenerationRejectsRestoredText(t *testing.T) {
	previous, err := mygo.Clipboard.Read()
	if err != nil {
		t.Skip("cannot preserve clipboard representations")
	}
	t.Cleanup(func() {
		if err := mygo.Clipboard.Write(previous); err != nil {
			t.Error("clipboard restoration failed")
		}
		_ = mygo.Clipboard.Flush()
	})
	mygo.Clipboard.WriteText("oshell-synthetic-token")
	if mygo.Clipboard.ReadText() != "oshell-synthetic-token" {
		t.Fatal("native clipboard write failed")
	}
	before, ok := mygo.Clipboard.Generation()
	if !ok {
		t.Fatal("native generation unavailable")
	}
	mygo.Clipboard.WriteText("oshell-synthetic-replacement")
	mygo.Clipboard.WriteText("oshell-synthetic-token")
	if mygo.Clipboard.ReadText() != "oshell-synthetic-token" {
		t.Fatal("native clipboard restore fixture failed")
	}
	after, ok := mygo.Clipboard.Generation()
	if !ok || before == after {
		t.Fatal("same text hid native clipboard changes")
	}
}

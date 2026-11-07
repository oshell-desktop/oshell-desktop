package desktop

import (
	"oshell-desktop/internal/core"
	"strings"
	"testing"
)

func TestPastePreviewFreezesTargetsAndAllowsEditing(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	left, lc := operatorPane(t, a, "left")
	right, rc := operatorPane(t, a, "right")
	third, tc := operatorPane(t, a, "excluded")
	a.config.Settings.Input.ConfirmMultiline = true
	a.features.syncEnabled = true
	a.features.syncTargets = map[string]bool{left.id: true, right.id: true}
	a.requestPaste(left, "before\npreview")
	if !a.features.pasteOpen || lc.text() != "" || rc.text() != "" {
		t.Fatal("sent before confirmation")
	}
	a.features.syncTargets = map[string]bool{third.id: true}
	a.features.pasteText = "edited\n中文"
	a.confirmPaste()
	waitOperator(t, a, func() bool { return lc.text() == "edited\r中文" && rc.text() == "edited\r中文" })
	if tc.text() != "" || a.features.pasteOpen || len(a.features.pasteTargets) != 0 {
		t.Fatal("wrong targets or stale preview")
	}
}

func TestPasteClosedTargetRejectsWholeDispatchAndEmptyCancelSendNothing(t *testing.T) {
	for _, action := range []string{"closed", "cancel", "empty", "invalid", "oversized", "missing"} {
		t.Run(action, func(t *testing.T) {
			a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
			left, lc := operatorPane(t, a, "left")
			right, rc := operatorPane(t, a, "right")
			a.config.Settings.Input.ConfirmMultiline = true
			a.features.syncEnabled = true
			a.features.syncTargets = map[string]bool{left.id: true, right.id: true}
			if action == "missing" {
				a.features.syncTargets["missing"] = true
			}
			a.requestPaste(left, "first\nsecond")
			switch action {
			case "closed":
				right.closed = true
			case "empty":
				a.features.pasteText = ""
			case "invalid":
				a.features.pasteText = "bad\x00input"
			case "oversized":
				a.features.pasteText = strings.Repeat("x", 1<<20+1)
			case "cancel":
				a.cancelPaste()
			}
			if action != "cancel" {
				a.confirmPaste()
			}
			// A later packet is a queue barrier, so the absence check cannot race the writers.
			left.term.Send([]byte("barrier"))
			right.term.Send([]byte("barrier"))
			waitOperator(t, a, func() bool { return len(lc.text()) >= 7 && len(rc.text()) >= 7 })
			if lc.text() != "barrier" || rc.text() != "barrier" {
				t.Fatal("rejected paste sent data", lc.text(), rc.text())
			}
		})
	}
}

func TestPasteCRLFPreviewAndPerTargetDispatch(t *testing.T) {
	for _, preview := range []bool{false, true} {
		name := "direct"
		if preview {
			name = "preview-edited"
		}
		t.Run(name, func(t *testing.T) {
			a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
			left, lc := operatorPane(t, a, "plain")
			right, rc := operatorPane(t, a, "bracketed")
			right.term.Feed([]byte("\x1b[?2004h"))
			a.config.Settings.Input.ConfirmMultiline = preview
			a.features.syncEnabled = true
			a.features.syncTargets = map[string]bool{left.id: true, right.id: true}
			a.requestPaste(left, "first\r\n\r\n  中文\r\n")
			want := "first\n\n  中文\n"
			if preview {
				if !a.features.pasteOpen || a.features.pasteText != want || lc.text() != "" || rc.text() != "" {
					t.Fatal("preview did not normalize CRLF or sent before confirmation")
				}
				// Editing can introduce Windows line endings again; dispatch must
				// normalize them too without removing intentional blank lines.
				a.features.pasteText = "edited\r\n\r\n\t中文\rfinal"
				want = "edited\n\n\t中文\nfinal"
				a.confirmPaste()
			}
			left.term.Send([]byte("<barrier>"))
			right.term.Send([]byte("<barrier>"))
			waitOperator(t, a, func() bool {
				return strings.HasSuffix(lc.text(), "<barrier>") && strings.HasSuffix(rc.text(), "<barrier>")
			})
			if lc.text() != strings.ReplaceAll(want, "\n", "\r")+"<barrier>" || rc.text() != "\x1b[200~"+want+"\x1b[201~<barrier>" {
				t.Fatalf("line endings/framing changed: plain %q, bracketed %q", lc.text(), rc.text())
			}
		})
	}
}

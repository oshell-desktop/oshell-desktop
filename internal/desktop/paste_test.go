package desktop

import (
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"oshell-desktop/internal/terminal"
	"strings"
	"testing"
)

func TestRightClickPasteFocusesTargetPane(t *testing.T) {
	for _, path := range []string{"direct", "menu", "preview", "from-command-bar"} {
		t.Run(path, func(t *testing.T) {
			a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
			left, leftConn := operatorPane(t, a, "left")
			right := &pane{id: core.NewID(), profile: core.Profile{Name: "right", Kind: "ssh"}}
			right.owner.Store(a)
			rightConn := &operatorConn{testConn: testConn{done: make(chan struct{})}}
			var frame *ui.Context
			var err error
			right.term, err = terminal.New(terminal.Options{
				Conn: rightConn, NoBlink: true, RightClickPaste: path != "menu",
				BeforePaste: func(text string) bool {
					if !right.focus.Focused(frame) {
						t.Error("paste began before the target received keyboard focus")
					}
					a.requestPaste(right, text)
					return false
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { right.term.Close() })
			a.config.Settings.Input.ConfirmMultiline = true
			a.config.Settings.QuickSendVisible = true
			tab := a.tabs[0]
			tab.root = &node{id: core.NewID(), first: tab.root, second: &node{id: core.NewID(), pane: right}, splitRatio: .5}
			a.selectTab(tab)
			tt := ui.NewTester(func(c *ui.Context) {
				frame = c
				a.view(c)
			}, 1150, 780)
			if !left.focus.Focused(frame) {
				t.Fatal("expected initial keyboard focus in left pane")
			}
			if path == "from-command-bar" {
				if err := tt.Click("快捷命令"); err != nil {
					t.Fatal(err)
				}
				if !a.features.operators.quickFocus.Focused(frame) {
					t.Fatal("command bar did not take keyboard focus")
				}
			}
			command := "echo target"
			if path == "preview" {
				command = "echo one\necho two"
			}
			tt.SetClipboard(command)
			bounds := right.focus.Bounds(frame)
			tt.RightClickAt(bounds.X+bounds.W/2, bounds.Y+bounds.H/2)
			if path == "menu" {
				if !left.focus.Focused(frame) {
					t.Fatal("opening the context menu changed keyboard focus")
				}
				if err := tt.ChooseMenuItem("Paste"); err != nil {
					t.Fatal(err)
				}
			}
			if path == "preview" {
				if !a.features.pasteOpen || right.focus.Focused(frame) || rightConn.text() != "" {
					t.Fatal("preview must take focus before any bytes are sent")
				}
				if err := tt.Click("粘贴"); err != nil {
					t.Fatal(err)
				}
			}
			tt.Type("!")
			tt.Key(0, ui.KeyEnter)
			left.term.Send([]byte("|"))
			right.term.Send([]byte("|"))
			waitOperator(t, a, func() bool {
				return strings.HasSuffix(leftConn.text(), "|") && strings.HasSuffix(rightConn.text(), "|")
			})
			want := strings.ReplaceAll(command, "\n", "\r") + "!\r|"
			if !right.focus.Focused(frame) || tab.active != right || leftConn.text() != "|" || rightConn.text() != want {
				t.Fatalf("target focused=%t active=%t, left=%q right=%q", right.focus.Focused(frame), tab.active == right, leftConn.text(), rightConn.text())
			}
		})
	}
}

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

package desktop

import (
	"os"
	"oshell-desktop/internal/core"
	"path/filepath"
	"testing"
)

func TestImagePasteRejectsLostContextAndLock(t *testing.T) {
	for _, change := range []string{"closed", "moved", "host", "terminal", "lock", "shutdown"} {
		t.Run(change, func(t *testing.T) {
			a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
			p, conn := operatorPane(t, a, "image")
			s := &imagePasteState{pane: p, term: p.term, data: []byte("not-written"), contextToken: p.term.PasteContextToken()}
			if !a.imagePasteValid(s) {
				t.Fatal("fixture not ready")
			}
			switch change {
			case "closed":
				p.closed = true
			case "moved":
				p.owner.Store(nil)
			case "host":
				p.hostProbe.epoch++
			case "terminal":
				p.term = nil
			case "lock":
				if err := a.vault.SetMaster("synthetic-image-master"); err != nil {
					t.Fatal(err)
				}
				a.vault.Lock()
 a.workspaceAuthorized = false
			case "shutdown":
				a.stopped.Store(true)
			}
			a.startImagePaste(s)
			if s.error == "" || s.busy || conn.text() != "" {
				t.Fatal("pasted into invalid target")
			}
			if _, err := os.Stat(filepath.Join(a.store.Dir, "clipboard-images")); !os.IsNotExist(err) {
				t.Fatal("wrote file before validating target", err)
			}
		})
	}
}

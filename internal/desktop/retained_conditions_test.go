package desktop

import (
	"github.com/jlaffaye/ftp"
	"os"
	"oshell-desktop/internal/core"
	"testing"
)

type retainedListingFailure struct {
	core.RemoteFileSystem
	err error
}

func (f *retainedListingFailure) RealPath(string) (string, error)       { return "/requested", nil }
func (f *retainedListingFailure) ReadDir(string) ([]os.FileInfo, error) { return nil, f.err }
func (f *retainedListingFailure) Close() error                          { return nil }

func TestRetainedF06ListingFailureKeepsDisplayedDirectory(t *testing.T) {
	for _, failure := range []error{ftp.ErrListingTooLarge, ftp.ErrListingEncoding} {
		a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
		info, err := os.Stat(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		client := &retainedListingFailure{err: failure}
		p := &pane{id: core.NewID(), ftp: &core.FTPSession{}, files: &fileState{client: client, lease: func() {}, dir: "/original", entries: []os.FileInfo{info}}}
		p.owner.Store(a)
		defer p.ftp.Close()
		a.loadFiles(p, "/requested")
		waitOperator(t, a, func() bool { return !p.files.busy })
		if p.files.dir != "/original" || len(p.files.entries) != 1 || p.files.entries[0] != info || !p.files.statusError {
			t.Fatal("failed listing replaced browser state")
		}
	}
}

func TestRetainedI04O06PerTargetPasteMode(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	left, lc := operatorPane(t, a, "left")
	right, rc := operatorPane(t, a, "right")
	third, tc := operatorPane(t, a, "excluded")
	left.term.Feed([]byte("\x1b[?2004h"))
	a.config.Settings.Input.ConfirmMultiline = true
	a.features.syncEnabled = true
	a.features.syncTargets = map[string]bool{left.id: true, right.id: true}
	a.requestPaste(left, "before\npreview")
	a.features.pasteText = "edited\ntext"
	a.features.syncTargets = map[string]bool{third.id: true}
	a.confirmPaste()
	wantLeft := "\x1b[200~edited\ntext\x1b[201~"
	wantRight := "edited\rtext"
	waitOperator(t, a, func() bool { return lc.text() == wantLeft && rc.text() == wantRight })
	if tc.text() != "" {
		t.Fatal("paste retargeted")
	}
}

func TestRetainedA10ClosingOneWindowReleasesTerminal(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	b := newApp(a.store, core.DefaultConfig())
	h := &applicationHub{windows: []*App{a, b}}
	a.hub = h
	b.hub = h
	t.Cleanup(a.idleMemory.cancel)
	t.Cleanup(b.idleMemory.cancel)
	p, pc := operatorPane(t, a, "close")
	q, qc := operatorPane(t, b, "keep")
	q.term.Feed([]byte("keep-history"))
	term := p.term
	a.closeTab(a.tabs[0])
	p.disposal.Wait()
	select {
	case <-pc.done:
	default:
		t.Fatal("closed terminal connection retained")
	}
	if term.Available() || a.idleMemory.timer != nil {
		t.Fatal("close waited for global reclaim")
	}
	q.term.Send([]byte("still-active"))
	waitOperator(t, b, func() bool { return qc.text() == "still-active" })
	if q.term.Text() != "keep-history" {
		t.Fatal("other window lost history")
	}
	b.closeTab(b.tabs[0])
	q.disposal.Wait()
	waitOperator(t, b, func() bool { return b.idleMemory.completed == 1 })
	operatorPane(t, a, "reopened")
	if !a.tabs[0].active.term.Available() {
		t.Fatal("reopen failed")
	}
}

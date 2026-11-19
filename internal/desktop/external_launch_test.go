package desktop

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"oshell-desktop/internal/core"
)

// Use the native event loop: Focus alone does not make a hidden Windows
// workspace visible, even though the IPC server continues to accept requests.
func runExternalLaunchVisibilityFixture() int {
	var failure error
	done := make(chan struct{})
	mygo.App.WhenReady(func() {
		store := &core.Store{Dir: os.Getenv("OSHELL_DATA_DIR")}
		a := newApp(store, core.DefaultConfig())
		if err := a.vault.SetMaster("fixture-master"); err != nil {
			failure = err
			close(done)
			mygo.App.Quit()
			return
		}
		a.vault.Lock()
		a.win = mygo.NewWindow(mygo.WindowOptions{Title: "OShell external launch fixture", Hidden: true})
		h := &applicationHub{store: store, vault: a.vault, config: a.config, windows: []*App{a}}
		a.hub = h
		h.startExternalLaunch()
		go func() {
			defer mygo.RunOnMain(func() {
				h.quitting = true
				if h.externalServer != nil {
					h.externalServer.Close()
				}
				for _, tab := range a.tabs {
					a.walk(tab.root, a.dispose)
				}
				close(done)
				a.win.Close()
			})
			request, err := core.ParseExternalLauncher("OShell-Bastion", []string{"-ssh", "-l", "fixture", "-pw", "fixture-ticket", "-P", "1", "127.0.0.1"})
			if err != nil {
				failure = err
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			receipt := make(chan error, 1)
			go func() { receipt <- core.LaunchApplicationAndSend(ctx, store.Dir, "must-not-start.exe", request) }()
			pending := false
			deadline := time.Now().Add(3 * time.Second)
			for !pending && time.Now().Before(deadline) {
				mygo.RunOnMain(func() { pending = len(h.externalPending) == 1 })
				if !pending {
					time.Sleep(10 * time.Millisecond)
				}
			}
			mygo.RunOnMain(func() {
				if !pending || !a.win.IsVisible() || !a.features.vaultOpen || a.features.vaultAction != "unlock" || len(a.tabs) != 0 {
					failure = fmt.Errorf("hidden locked workspace was not presented for unlock: pending=%v visible=%v unlock=%v tabs=%d", pending, a.win.IsVisible(), a.features.vaultOpen, len(a.tabs))
					return
				}
				// Unlock can complete while the window is hidden again. Accepting
				// the queued session must also restore the terminal window.
				a.win.Hide()
				if failure = a.vault.Unlock("fixture-master"); failure != nil {
					return
				}
				a.afterVaultUnlock()
				if !a.win.IsVisible() || len(h.externalPending) != 0 || len(a.tabs) != 1 {
					failure = fmt.Errorf("unlocked request did not open exactly one visible terminal")
				}
			})
			if failure != nil {
				return
			}
			select {
			case failure = <-receipt:
			case <-ctx.Done():
				failure = ctx.Err()
			}
		}()
	})
	if err := mygo.App.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	<-done
	if failure != nil {
		fmt.Fprintln(os.Stderr, failure)
		return 1
	}
	fmt.Println("hidden workspace shown for unlock; queued PuTTY session accepted once and terminal shown")
	return 0
}

func TestExternalLaunchRestoresHiddenWorkspace(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows native hidden-window activation")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "OSHELL_NATIVE_EXTERNAL_LAUNCH=1", "OSHELL_DATA_DIR="+t.TempDir())
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("external launch visibility: %v\n%s", err, out)
	}
	t.Log(string(out))
}

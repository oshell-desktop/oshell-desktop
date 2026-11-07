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

// No UI frames drain App.events. This models a minimized/occluded workspace
// while its auxiliary manager waits for the credential transaction to finish.
func runManagerCompletionFixture() int {
	var failure error
	done := make(chan struct{})
	mygo.App.WhenReady(func() {
		store := &core.Store{Dir: os.Getenv("OSHELL_DATA_DIR")}
		cfg := core.DefaultConfig()
		p := core.NewProfile()
		p.Name, p.Host = "completion-source", "fixture.invalid"
		cfg.Profiles = []core.Profile{p}
		if err := store.Save(cfg); err != nil {
			failure = err
			close(done)
			mygo.App.Quit()
			return
		}
		a := newApp(store, cfg)
		w := mygo.NewWindow(mygo.WindowOptions{Title: "OShell hidden completion fixture", Hidden: true})
		a.win = w
		completed := make(chan struct{})
		a.copyManagerProfiles([]core.Profile{p}, nil, func() {
			if a.configurationBusy() || len(a.config.Profiles) != 2 {
				failure = fmt.Errorf("completion did not release transaction and apply copy")
			}
			a.win = nil // Check editor state without showing/focusing the hidden fixture.
			a.editManagerProfile(a.config.Profiles[1])
			a.win = w
			if !a.editing {
				failure = fmt.Errorf("completed copy cannot be edited immediately")
			}
			close(completed)
		})
		go func() {
			select {
			case <-completed:
			case <-time.After(5 * time.Second):
				// Main-thread observation keeps the assertion independent of worker timing.
				mygo.RunOnMain(func() { failure = fmt.Errorf("copy completion waits for a paint: busy=%v", a.configurationBusy()) })
			}
			mygo.RunOnMain(func() { close(done); w.Close() })
		}()
	})
	if err := mygo.App.Run(); err != nil {
		return 1
	}
	<-done
	if failure != nil {
		fmt.Fprintln(os.Stderr, failure)
		return 1
	}
	fmt.Println("hidden workspace: copy committed, busy released, editor opened without frame drain")
	return 0
}

func TestManagerCompletionWithoutWorkspacePaint(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows event-loop fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "OSHELL_NATIVE_MANAGER_COMPLETION=1", "OSHELL_DATA_DIR="+t.TempDir())
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("hidden completion: %v\n%s", err, out)
	}
	t.Log(string(out))
}

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
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

// Exercise actual Windows destruction inside drain, just like the posted
// confirmation callback. A model-only stopped flag cannot invalidate Context.
func runNativeDrainClose() int {
	var failure error
	done := make(chan struct{})
	mygo.App.WhenReady(func() {
		a := newApp(&core.Store{Dir: os.Getenv("OSHELL_DATA_DIR")}, core.DefaultConfig())
		keeper := mygo.NewWindow(mygo.WindowOptions{Title: "drain-close keeper", Hidden: true})
		views := map[string]func(*ui.Context){
			"file": a.fileWindowView, "workspace": a.view,
			"settings": a.settingsView, "manager": a.managerView, "sync": a.syncStatusView,
		}
		view := views[os.Getenv("OSHELL_NATIVE_DRAIN_CLOSE")]
		var frame *ui.Context
		w := mygo.NewWindow(mygo.WindowOptions{Title: "drain-close fixture", Width: 900, Height: 650,
			Content: ui.View(func(c *ui.Context) { frame = c; view(c) })})
		a.win = w
		closed := make(chan struct{})
		// Deliberately enqueue for the next view's drain. App.post normally
		// dispatches outside paint now; this still covers a close during paint.
		a.events = append(a.events, func() {
			if frame == nil || !frame.Valid() {
				failure = fmt.Errorf("close did not run inside a live frame")
			}
			w.Close()
			if frame.Valid() {
				failure = fmt.Errorf("native close did not invalidate the frame")
			}
			close(closed)
		})
		w.Invalidate()
		go func() {
			<-closed
			mygo.RunOnMain(func() {
				if len(mygo.Windows()) != 1 || mygo.Windows()[0] != keeper {
					failure = fmt.Errorf("closing a window affected its sibling")
				}
				close(done)
				keeper.Close()
			})
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
	fmt.Println("native close invalidated frame; view returned; sibling survived; clean exit")
	return 0
}

func TestNativeCloseDuringFrameDrain(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows synchronous native window destruction")
	}
	for _, view := range []string{"file", "workspace", "settings", "manager", "sync"} {
		t.Run(view, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^$")
			cmd.Env = append(os.Environ(), "OSHELL_NATIVE_DRAIN_CLOSE="+view, "OSHELL_DATA_DIR="+t.TempDir())
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("native %s close: %v\n%s", view, err, out)
			}
			t.Log(string(out))
		})
	}
}

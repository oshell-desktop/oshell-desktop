package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/egoist/mygo"
	"os"
	"os/exec"
	"oshell-desktop/internal/core"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func runP2NativeLifecycle() int {
	results := map[string]bool{}
	var failure error
	main := func(fn func()) { done := make(chan struct{}); mygo.RunOnMain(func() { fn(); close(done) }); <-done }
	count := func() int { n := 0; main(func() { n = len(mygo.Windows()) }); return n }
	wait := func(want int) error {
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if count() == want {
				return nil
			}
			time.Sleep(20 * time.Millisecond)
		}
		return fmt.Errorf("window count != %d", want)
	}
	mygo.App.WhenReady(func() {
		go func() {
			defer func() { mygo.App.Quit() }()
			if failure = wait(1); failure != nil {
				return
			}
			main(func() { item := mygo.App.Dock.Menu().Items()[0]; item.Click(item, mygo.Windows()[0]) })
			if failure = wait(2); failure != nil {
				return
			}
			results["TwoWindows"] = true
			main(func() {
				for _, w := range mygo.Windows() {
					w.Close()
				}
			})
			if failure = wait(0); failure != nil {
				return
			}
			time.Sleep(200 * time.Millisecond)
			main(func() { results["ZeroWindowAlive"] = true })
			request, err := core.ParseExternalLauncher("OShell-ZOC", []string{"/SSH:fixture@127.0.0.1:1"})
			if err != nil {
				failure = err
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if failure = core.SendExternalLaunch(ctx, os.Getenv("OSHELL_DATA_DIR"), request); failure != nil {
				return
			}
			if failure = wait(1); failure != nil {
				return
			}
			results["ExternalRestoreOneWindow"] = true
			time.Sleep(500 * time.Millisecond)
			main(func() {
				mygo.App.Quit()
				results["ExplicitQuitRequested"] = true
			})
		}()
	})
	if err := runDesktop(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if failure != nil {
		fmt.Fprintln(os.Stderr, failure)
		return 1
	}
	results["ExplicitQuitCompleted"] = true
	data, _ := json.Marshal(results)
	fmt.Println(string(data))
	if err := os.WriteFile(os.Getenv("OSHELL_P2_NATIVE_RESULT"), data, 0600); err != nil {
		return 1
	}
	return 0
}
func TestP2W01NativeProductionLifecycle(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows native lifecycle fixture")
	}
	dir := t.TempDir()
	result := filepath.Join(dir, "lifecycle.json")
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "OSHELL_P2_NATIVE_LIFECYCLE=1", "OSHELL_DATA_DIR="+dir, "OSHELL_P2_NATIVE_RESULT="+result)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("native lifecycle: %v\n%s", err, out)
	}
	data, err := os.ReadFile(result)
	if err != nil {
		t.Fatal(err)
	}
	var facts map[string]bool
	if err = json.Unmarshal(data, &facts); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"TwoWindows", "ZeroWindowAlive", "ExternalRestoreOneWindow", "ExplicitQuitRequested", "ExplicitQuitCompleted"} {
		if !facts[key] {
			t.Fatal("missing native fact", key)
		}
	}
	t.Log(string(data))
}

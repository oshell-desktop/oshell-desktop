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
	done := make(chan struct{})
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
			defer func() {
				// Success must exit solely from closing the last window.
				if failure != nil {
					mygo.App.Quit()
				}
				close(done)
			}()
			if failure = wait(1); failure != nil {
				return
			}
			main(func() { item := mygo.App.Dock.Menu().Items()[0]; item.Click(item, mygo.Windows()[0]) })
			if failure = wait(2); failure != nil {
				return
			}
			results["TwoWindows"] = true
			main(func() { mygo.Windows()[0].Close() })
			if failure = wait(1); failure != nil {
				return
			}
			time.Sleep(200 * time.Millisecond)
			main(func() { results["OtherWindowKeptAlive"] = len(mygo.Windows()) == 1 })
			request, err := core.ParseExternalLauncher("OShell-ZOC", []string{"/SSH:fixture@127.0.0.1:1"})
			files := os.Getenv("OSHELL_P2_NATIVE_LAST_FILE") == "1"
			if files {
				request, err = core.ParseExternalLauncher("FileZilla", []string{"sftp://fixture@127.0.0.1:1/"})
			}
			if err != nil {
				failure = err
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if failure = core.SendExternalLaunch(ctx, os.Getenv("OSHELL_DATA_DIR"), request); failure != nil {
				return
			}
			wanted := 1
			if files {
				wanted = 2
			}
			if failure = wait(wanted); failure != nil {
				return
			}
			results["ExternalLaunchAccepted"] = true
			if files {
				main(func() {
					for _, w := range mygo.Windows() {
						if w.Title() != "文件管理" {
							w.Close()
						}
					}
				})
				if failure = wait(1); failure != nil {
					return
				}
				main(func() { results["FileWindowKeptAlive"] = mygo.Windows()[0].Title() == "文件管理" })
			}
			time.Sleep(500 * time.Millisecond)
			main(func() {
				results["LastWindowCloseRequested"] = true
				mygo.Windows()[0].Close()
			})
		}()
	})
	if err := runDesktop(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	<-done
	if failure != nil {
		fmt.Fprintln(os.Stderr, failure)
		return 1
	}
	results["LastWindowQuitCompleted"] = true
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
	for _, last := range []string{"terminal", "file"} {
		t.Run(last, func(t *testing.T) { testNativeLastWindowExit(t, last == "file") })
	}
}

func testNativeLastWindowExit(t *testing.T, files bool) {
	dir := t.TempDir()
	result := filepath.Join(dir, "lifecycle.json")
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "OSHELL_P2_NATIVE_LIFECYCLE=1", "OSHELL_DATA_DIR="+dir, "OSHELL_P2_NATIVE_RESULT="+result)
	if files {
		cmd.Env = append(cmd.Env, "OSHELL_P2_NATIVE_LAST_FILE=1")
	}
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
	keys := []string{"TwoWindows", "OtherWindowKeptAlive", "ExternalLaunchAccepted", "LastWindowCloseRequested", "LastWindowQuitCompleted"}
	if files {
		keys = append(keys, "FileWindowKeptAlive")
	}
	for _, key := range keys {
		if !facts[key] {
			t.Fatal("missing native fact", key)
		}
	}
	t.Log(string(data))
}

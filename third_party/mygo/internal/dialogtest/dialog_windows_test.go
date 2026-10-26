//go:build windows

// Package dialogtest exercises only fresh native pickers, without WebView,
// clipboard access, saved settings or connections. Opt in with MYGO_E2E=1.
package dialogtest

import (
	"context"
	"errors"
	"fmt"
	"github.com/egoist/mygo"
	"os"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func TestMain(m *testing.M) {
	if os.Getenv("MYGO_E2E") != "1" {
		fmt.Println("native picker test not run; requires MYGO_E2E=1 and Windows desktop")
		os.Exit(0)
	}
	code := 1
	mygo.App.WhenReady(func() { go func() { code = m.Run(); mygo.App.Quit() }() })
	if err := mygo.App.Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	os.Exit(code)
}

var user32 = syscall.NewLazyDLL("user32.dll")

func visiblePickers() int {
	count := 0
	callback := syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		var pid uint32
		user32.NewProc("GetWindowThreadProcessId").Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		if pid != uint32(os.Getpid()) {
			return 1
		}
		var class [128]uint16
		user32.NewProc("GetClassNameW").Call(hwnd, uintptr(unsafe.Pointer(&class[0])), uintptr(len(class)))
		visible, _, _ := user32.NewProc("IsWindowVisible").Call(hwnd)
		if visible != 0 && syscall.UTF16ToString(class[:]) == "#32770" {
			count++
		}
		return 1
	})
	user32.NewProc("EnumWindows").Call(callback, 0)
	return count
}

func TestNativeOpenContextClosesWindow(t *testing.T) {
	for i, directory := range []bool{false, true, false} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			observed := make(chan bool, 1)
			go func() {
				for ctx.Err() == nil {
					count := 0
					mygo.RunOnMain(func() { count = visiblePickers() })
					if count == 1 {
						observed <- true
						cancel()
						return
					}
					time.Sleep(20 * time.Millisecond)
				}
				observed <- false
			}()
			paths, err := mygo.Dialog.OpenContext(ctx, mygo.OpenDialogOptions{Title: "OShell synthetic picker test", DefaultPath: t.TempDir(), Directory: directory, Multiple: !directory})
			if !<-observed {
				t.Fatal("no native picker observed")
			}
			if !errors.Is(err, context.Canceled) || len(paths) != 0 {
				t.Fatalf("%v %v", paths, err)
			}
			deadline := time.Now().Add(3 * time.Second)
			for {
				count := 0
				mygo.RunOnMain(func() { count = visiblePickers() })
				if count == 0 {
					return
				}
				if time.Now().After(deadline) {
					t.Fatal("cancelled picker remains visible")
				}
				time.Sleep(20 * time.Millisecond)
			}
		})
	}
}

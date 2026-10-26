//go:build windows && (amd64 || arm64)

package windows

import (
	"runtime"
	"strings"
	"syscall"
	"testing"
	"unsafe"
)

func TestFileDialogShowErrors(t *testing.T) {
	if err := fileDialogShowError(hrDialogCancelled); err != nil {
		t.Fatal(err)
	}
	if err := fileDialogShowError(0x80070005); err == nil || !strings.Contains(err.Error(), "80070005") {
		t.Fatal("lost HRESULT", err)
	}
}

func TestFileDialogResultsAreAllOrError(t *testing.T) {
	for _, failure := range []string{"none", "count", "second-item", "path", "empty-path", "nil-item"} {
		t.Run(failure, func(t *testing.T) {
			var itemVT [6]uintptr
			var arrayVT [9]uintptr
			item := struct{ vt *[6]uintptr }{&itemVT}
			array := struct{ vt *[9]uintptr }{&arrayVT}
			releases := 0
			itemVT[2] = syscall.NewCallback(func(uintptr) uintptr { releases++; return 1 })
			itemVT[5] = syscall.NewCallback(func(_ uintptr, _ uintptr, out uintptr) uintptr {
				if failure == "path" {
					return 0x80070005
				}
				path := `C:\fixture\selected.bin`
				if failure == "empty-path" {
					path = ""
				}
				chars := syscall.StringToUTF16(path)
				p, _, _ := ole32.NewProc("CoTaskMemAlloc").Call(uintptr(len(chars) * 2))
				copy(unsafe.Slice((*uint16)(native(p)), len(chars)), chars)
				*(*uintptr)(native(out)) = p
				return 0
			})
			arrayVT[7] = syscall.NewCallback(func(_ uintptr, out uintptr) uintptr {
				if failure == "count" {
					return 0x80070005
				}
				*(*uint32)(native(out)) = 2
				return 0
			})
			arrayVT[8] = syscall.NewCallback(func(_ uintptr, index uintptr, out uintptr) uintptr {
				if failure == "second-item" && index == 1 {
					return 0x80070005
				}
				if failure != "nil-item" {
					*(*uintptr)(native(out)) = uintptr(unsafe.Pointer(&item))
				}
				return 0
			})
			paths, err := fileDialogPaths(uintptr(unsafe.Pointer(&array)))
			runtime.KeepAlive(&array)
			runtime.KeepAlive(&item)
			if failure == "none" {
				if err != nil || len(paths) != 2 || releases != 2 {
					t.Fatalf("%v %v releases=%d", paths, err, releases)
				}
			} else if err == nil || paths != nil {
				t.Fatalf("partial selection accepted: %v %v", paths, err)
			}
		})
	}
}

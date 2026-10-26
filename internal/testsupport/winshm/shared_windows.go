// Package winshm provides synthetic Windows mappings for image protocol tests.
// Production packages do not import it.
package winshm

import (
	"crypto/rand"
	"encoding/hex"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

type Mapping struct {
	Name   string
	Handle windows.Handle
	view   uintptr
}

func New(t testing.TB, data []byte, size uint32, protection uint32) *Mapping {
	t.Helper()
	return NewFile(t, windows.InvalidHandle, data, size, protection)
}

func NewFile(t testing.TB, file windows.Handle, data []byte, size uint32, protection uint32) *Mapping {
	t.Helper()
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		t.Fatal(err)
	}
	m := &Mapping{Name: "oshell-kitty-" + hex.EncodeToString(id[:])}
	wide, err := windows.UTF16PtrFromString("Local\\" + m.Name)
	if err != nil {
		t.Fatal(err)
	}
	if size == 0 {
		size = uint32(len(data))
	}
	if protection == 0 {
		protection = windows.PAGE_READWRITE
	}
	m.Handle, err = windows.CreateFileMapping(file, nil, protection, 0, size, wide)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	if len(data) > 0 {
		m.view, err = windows.MapViewOfFile(m.Handle, windows.FILE_MAP_WRITE, 0, 0, uintptr(len(data)))
		if err != nil {
			t.Fatal(err)
		}
		copy(unsafe.Slice((*byte)(unsafe.Pointer(m.view)), len(data)), data)
	}
	return m
}

func (m *Mapping) Close() {
	if m.view != 0 {
		windows.UnmapViewOfFile(m.view)
		m.view = 0
	}
	if m.Handle != 0 {
		windows.CloseHandle(m.Handle)
		m.Handle = 0
	}
}

func Exists(name string) bool {
	wide, err := windows.UTF16PtrFromString("Local\\" + name)
	if err != nil {
		return false
	}
	h, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("OpenFileMappingW").Call(windows.FILE_MAP_READ, 0, uintptr(unsafe.Pointer(wide)))
	if h == 0 {
		return false
	}
	windows.CloseHandle(windows.Handle(h))
	return true
}

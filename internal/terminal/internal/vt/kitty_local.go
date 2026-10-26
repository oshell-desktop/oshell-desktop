package vt

import (
	"os"
	"runtime"
	"unsafe"
)

// POSIX Kitty media use the same local namespace as OShell. The runtime
// validates canonical paths, regular files, ranges and temporary-file names.
func (t *Terminal) enableLocalImages() error {
	allow := true
	if err := t.set(16, unsafe.Pointer(&allow)); err != nil {
		return err
	}
	shared := true // Windows uses session-local named mappings; POSIX uses shm_open.
	if err := t.set(18, unsafe.Pointer(&shared)); err != nil {
		return err
	}
	b := []byte(os.TempDir())
	var pin runtime.Pinner
	defer pin.Unpin()
	pin.Pin(&b[0])
	s := cString{len: uintptr(len(b)), ptr: uintptr(unsafe.Pointer(&b[0]))}
	return t.set(17, unsafe.Pointer(&s))
}

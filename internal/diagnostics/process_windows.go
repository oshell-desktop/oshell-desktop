//go:build windows

package diagnostics

import (
	"golang.org/x/sys/windows"
	"os/exec"
	"syscall"
	"unsafe"
)

var memoryInfoProc = windows.NewLazySystemDLL("psapi.dll").NewProc("GetProcessMemoryInfo")
var handleCountProc = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetProcessHandleCount")

func processStats() map[string]any {
	fields := map[string]any{}
	var m struct {
		Size, Faults                                                                                          uint32
		PeakWorkingSet, WorkingSet, PeakPaged, Paged, PeakNonPaged, NonPaged, Pagefile, PeakPagefile, Private uintptr
	}
	m.Size = uint32(unsafe.Sizeof(m))
	proc := memoryInfoProc
	if proc.Find() == nil {
		if ok, _, _ := proc.Call(uintptr(windows.CurrentProcess()), uintptr(unsafe.Pointer(&m)), uintptr(m.Size)); ok != 0 {
			fields["working_set_bytes"] = m.WorkingSet
			fields["private_bytes"] = m.Private
		}
	}
	var count uint32
	handles := handleCountProc
	if handles.Find() == nil {
		if ok, _, _ := handles.Call(uintptr(windows.CurrentProcess()), uintptr(unsafe.Pointer(&count))); ok != 0 {
			fields["handle_count"] = count
		}
	}
	return fields
}

func hideMonitor(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
func observeProcess(pid int) (func() any, func()) {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return func() any { return nil }, func() {}
	}
	return func() any {
		windows.WaitForSingleObject(h, 1000)
		var code uint32
		if windows.GetExitCodeProcess(h, &code) != nil || code == 259 {
			return nil
		}
		return code
	}, func() { windows.CloseHandle(h) }
}

package core

import (
	"context"
	"errors"
	"golang.org/x/sys/windows"
	"runtime"
	"unsafe"
)

func lockLaunchStart(ctx context.Context, configuration string) (func(), error) {
	name, sid, err := launchWindowsIdentity(configuration)
	if err != nil {
		return nil, err
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;GA;;;" + sid + ")")
	if err != nil {
		return nil, err
	}
	attrs := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	p, _ := windows.UTF16PtrFromString(`Local\` + name)
	h, err := windows.CreateMutex(&attrs, false, p)
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return nil, err
	}
	runtime.LockOSThread()
	for {
		if err = ctx.Err(); err != nil {
			windows.CloseHandle(h)
			runtime.UnlockOSThread()
			return nil, err
		}
		state, err := windows.WaitForSingleObject(h, 100)
		if err != nil {
			windows.CloseHandle(h)
			runtime.UnlockOSThread()
			return nil, err
		}
		if state == windows.WAIT_OBJECT_0 || state == windows.WAIT_ABANDONED {
			return func() { _ = windows.ReleaseMutex(h); _ = windows.CloseHandle(h); runtime.UnlockOSThread() }, nil
		}
	}
}

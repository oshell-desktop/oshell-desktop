// The two registered Windows client names forward to the same OShell install.
package main

import (
	"os"
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
	"oshell-desktop/internal/clientalias"
)

func main() {
	exe, err := os.Executable()
	var target string
	if err == nil {
		target, err = clientalias.Target(exe)
	}
	if err == nil {
		args := os.Args[1:]
		if len(args) > 0 {
			args = append([]string{"--oshell-bastion-client"}, args...)
		}
		cmd := exec.Command(target, args...)
		err = cmd.Start()
		if err == nil {
			if len(args) == 0 {
				return
			}
			if err = cmd.Wait(); err != nil {
				os.Exit(1)
			}
			return
		}
	}
	// The operating system error may include a command line. Display only
	// recovery instructions; external SSH arguments may contain a ticket.
	title, _ := windows.UTF16PtrFromString("OShell Desktop")
	message, _ := windows.UTF16PtrFromString("无法启动 OShell。请打开 OShell 设置，重新启用对应的堡垒机客户端。")
	windows.NewLazySystemDLL("user32.dll").NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(message)), uintptr(unsafe.Pointer(title)), 0x10)
	os.Exit(1)
}

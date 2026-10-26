package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"path/filepath"
	"strings"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

func launchWindowsIdentity(configuration string) (name, sid string, err error) {
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", "", err
	}
	sid = u.User.Sid.String()
	abs, err := filepath.Abs(configuration)
	if err != nil {
		return "", "", err
	}
	h := sha256.Sum256([]byte(sid + "\n" + strings.ToLower(filepath.Clean(abs))))
	return "oshell-desktop-launch-" + hex.EncodeToString(h[:16]), sid, nil
}
func listenLaunch(configuration string) (net.Listener, func(), error) {
	name, sid, err := launchWindowsIdentity(configuration)
	if err != nil {
		return nil, nil, err
	}
	l, err := winio.ListenPipe(`\\.\pipe\`+name, &winio.PipeConfig{SecurityDescriptor: "D:P(A;;GA;;;" + sid + ")", InputBufferSize: 65536, OutputBufferSize: 65536})
	return l, nil, err
}
func dialLaunch(ctx context.Context, configuration string) (net.Conn, error) {
	name, _, err := launchWindowsIdentity(configuration)
	if err != nil {
		return nil, err
	}
	return winio.DialPipeContext(ctx, `\\.\pipe\`+name)
}
func verifyLaunchConnection(conn net.Conn, server bool) error {
	f, ok := conn.(interface{ Fd() uintptr })
	if !ok {
		return fmt.Errorf("无法核对启动通道身份")
	}
	var pid uint32
	var err error
	if server {
		err = windows.GetNamedPipeServerProcessId(windows.Handle(f.Fd()), &pid)
	} else {
		err = windows.GetNamedPipeClientProcessId(windows.Handle(f.Fd()), &pid)
	}
	if err != nil {
		return err
	}
	p, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(p)
	var token windows.Token
	if err = windows.OpenProcessToken(p, windows.TOKEN_QUERY, &token); err != nil {
		return err
	}
	defer token.Close()
	u, err := token.GetTokenUser()
	if err != nil {
		return err
	}
	own, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	if !u.User.Sid.Equals(own.User.Sid) {
		return fmt.Errorf("启动通道用户不匹配")
	}
	return nil
}

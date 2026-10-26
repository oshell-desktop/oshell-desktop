//go:build linux

package core

import (
	"fmt"
	"golang.org/x/sys/unix"
	"net"
	"os"
)

func verifyLaunchPeer(conn *net.UnixConn) error {
	raw, e := conn.SyscallConn()
	if e != nil {
		return e
	}
	var peerErr error
	e = raw.Control(func(fd uintptr) {
		cred, err := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		if err != nil {
			peerErr = err
		} else if cred.Uid != uint32(os.Geteuid()) {
			peerErr = fmt.Errorf("启动通道用户不匹配")
		}
	})
	if e != nil {
		return e
	}
	return peerErr
}

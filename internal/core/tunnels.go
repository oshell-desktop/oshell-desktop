// SPDX-License-Identifier: GPL-3.0-only
package core

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

func (t *transport) startTunnels(ctx context.Context, rules []TunnelRule) error {
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		addr := net.JoinHostPort(strings.Trim(r.BindHost, "[]"), strconv.Itoa(r.ListenPort))
		var listener net.Listener
		var e error
		if r.Kind == "remote" {
			listener, e = t.client.Listen("tcp", addr)
		} else {
			listener, e = (&net.ListenConfig{}).Listen(ctx, "tcp", addr)
		}
		if e != nil {
			return fmt.Errorf("无法启动 %s 隧道 %s: %w", r.Kind, addr, e)
		}
		if !t.track(listener) {
			return fmt.Errorf("连接已关闭")
		}
		go t.acceptTunnel(ctx, listener, r)
	}
	return nil
}
func (t *transport) acceptTunnel(ctx context.Context, l net.Listener, r TunnelRule) {
	defer func() {
		if err := l.Close(); err == nil || errors.Is(err, net.ErrClosed) {
			t.untrack(l)
		}
	}()
	for {
		in, e := l.Accept()
		if e != nil {
			return
		}
		if !t.track(in) {
			return
		}
		go func() {
			defer t.untrack(in)
			defer in.Close()
			target := net.JoinHostPort(strings.Trim(r.DestinationHost, "[]"), strconv.Itoa(r.DestinationPort))
			version := byte(0)
			if r.Kind == "dynamic" {
				in.SetDeadline(time.Now().Add(15 * time.Second))
				var e error
				target, version, e = readDynamicRequest(in)
				if e != nil {
					return
				}
			}
			dctx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			var out net.Conn
			var e error
			if r.Kind == "remote" {
				out, e = (&net.Dialer{}).DialContext(dctx, "tcp", target)
			} else {
				out, e = t.client.DialContext(dctx, "tcp", target)
			}
			if r.Kind == "dynamic" {
				replyDynamic(in, version, e == nil)
				in.SetDeadline(time.Time{})
			}
			if e != nil {
				return
			}
			if !t.track(out) {
				return
			}
			defer t.untrack(out)
			defer out.Close()
			bridge(in, out)
		}()
	}
}
func bridge(a, b net.Conn) {
	done := make(chan struct{})
	go func() {
		io.Copy(a, b)
		if c, ok := a.(interface{ CloseWrite() error }); ok {
			c.CloseWrite()
		} else {
			a.Close()
		}
		close(done)
	}()
	io.Copy(b, a)
	if c, ok := b.(interface{ CloseWrite() error }); ok {
		c.CloseWrite()
	} else {
		b.Close()
	}
	<-done
}
func readNull(r io.Reader) (string, error) {
	var b []byte
	for len(b) <= 255 {
		var v [1]byte
		if _, e := io.ReadFull(r, v[:]); e != nil {
			return "", e
		}
		if v[0] == 0 {
			return string(b), nil
		}
		b = append(b, v[0])
	}
	return "", fmt.Errorf("SOCKS 字段过长")
}
func readDynamicRequest(c net.Conn) (string, byte, error) {
	var v [1]byte
	if _, e := io.ReadFull(c, v[:]); e != nil {
		return "", 0, e
	}
	switch v[0] {
	case 5:
		var n [1]byte
		if _, e := io.ReadFull(c, n[:]); e != nil {
			return "", 5, e
		}
		methods := make([]byte, int(n[0]))
		if _, e := io.ReadFull(c, methods); e != nil {
			return "", 5, e
		}
		found := false
		for _, m := range methods {
			found = found || m == 0
		}
		if !found {
			writeAll(c, []byte{5, 255})
			return "", 5, fmt.Errorf("不支持的 SOCKS5 认证")
		}
		if e := writeAll(c, []byte{5, 0}); e != nil {
			return "", 5, e
		}
		var h [4]byte
		if _, e := io.ReadFull(c, h[:]); e != nil {
			return "", 5, e
		}
		if h[0] != 5 || h[1] != 1 || h[2] != 0 {
			replyDynamic(c, 5, false)
			return "", 5, fmt.Errorf("只支持 SOCKS CONNECT")
		}
		s, e := readSOCKSAddress(c, h[3])
		if e == nil {
			_, port, _ := net.SplitHostPort(s)
			if port == "0" {
				e = fmt.Errorf("目标端口无效")
			}
		}
		return s, 5, e
	case 4:
		var h [7]byte
		if _, e := io.ReadFull(c, h[:]); e != nil {
			return "", 4, e
		}
		if h[0] != 1 {
			return "", 4, fmt.Errorf("只支持 SOCKS CONNECT")
		}
		if _, e := readNull(c); e != nil {
			return "", 4, e
		}
		host := net.IP(h[3:]).String()
		if h[3] == 0 && h[4] == 0 && h[5] == 0 && h[6] != 0 {
			var e error
			host, e = readNull(c)
			if e != nil {
				return "", 4, e
			}
		}
		port := int(binary.BigEndian.Uint16(h[1:3]))
		if !validHost(host) || !validPort(port) {
			return "", 4, fmt.Errorf("目标地址无效")
		}
		return net.JoinHostPort(host, strconv.Itoa(port)), 4, nil
	}
	return "", v[0], fmt.Errorf("只支持 SOCKS4/5")
}
func replyDynamic(c net.Conn, version byte, ok bool) {
	if version == 4 {
		code := byte(91)
		if ok {
			code = 90
		}
		writeAll(c, []byte{0, code, 0, 0, 0, 0, 0, 0})
	} else {
		code := byte(1)
		if ok {
			code = 0
		}
		writeAll(c, []byte{5, code, 0, 1, 0, 0, 0, 0, 0, 0})
	}
}

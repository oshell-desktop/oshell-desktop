// SPDX-License-Identifier: GPL-3.0-only
package core

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *bufferedConn) Read(b []byte) (int, error) { return c.r.Read(b) }
func writeAll(w io.Writer, b []byte) error         { _, e := io.Copy(w, bytes.NewReader(b)); return e }
func proxyConnect(ctx context.Context, p ProxySettings, password, target string, tcpKeepAlive bool) (net.Conn, error) {
	keep := time.Duration(-1)
	if tcpKeepAlive {
		keep = 30 * time.Second
	}
	address := target
	if p.Kind != "" && p.Kind != "none" {
		address = net.JoinHostPort(strings.Trim(p.Host, "[]"), strconv.Itoa(p.Port))
	}
	c, e := (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: keep}).DialContext(ctx, "tcp", address)
	if e != nil {
		return nil, e
	}
	if p.Kind == "" || p.Kind == "none" {
		return c, nil
	}
	return proxyHandshake(ctx, c, p, password, target)
}
func proxyHandshake(ctx context.Context, c net.Conn, p ProxySettings, password, target string) (net.Conn, error) {
	var e error
	handshake, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	stop := context.AfterFunc(handshake, func() { c.Close() })
	defer stop()
	c.SetDeadline(time.Now().Add(20 * time.Second))
	var result net.Conn = c
	switch p.Kind {
	case "http":
		header := "CONNECT " + target + " HTTP/1.1\r\nHost: " + target + "\r\n"
		if p.Username != "" || password != "" {
			header += "Proxy-Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte(p.Username+":"+password)) + "\r\n"
		}
		e = writeAll(c, []byte(header+"\r\n"))
		if e == nil {
			r := bufio.NewReaderSize(c, 4096)
			var response *http.Response
			var header []byte
			header, e = readProxyHeader(r)
			if e == nil {
				response, e = http.ReadResponse(bufio.NewReader(bytes.NewReader(header)), &http.Request{Method: "CONNECT"})
			}
			if e == nil {
				if response.ProtoMajor != 1 || response.StatusCode != 200 {
					e = fmt.Errorf("HTTP 代理返回 %s", response.Status)
				} else {
					result = &bufferedConn{c, r}
				}
			}
		}
	case "socks5":
		e = connectSOCKS5(c, target, p.Username, password)
	case "socks4", "socks4a":
		e = connectSOCKS4(ctx, c, target, p.Kind == "socks4a", p.Username)
	default:
		e = fmt.Errorf("不支持的代理 %q", p.Kind)
	}
	if e != nil {
		c.Close()
		return nil, e
	}
	c.SetDeadline(time.Time{})
	return result, nil
}
func socksAddress(address string) ([]byte, error) {
	host, port, e := net.SplitHostPort(address)
	if e != nil {
		return nil, e
	}
	n, e := strconv.Atoi(port)
	if e != nil || !validPort(n) {
		return nil, fmt.Errorf("目标端口无效")
	}
	var b []byte
	if ip := net.ParseIP(host); ip != nil {
		if v := ip.To4(); v != nil {
			b = append([]byte{1}, v...)
		} else {
			b = append([]byte{4}, ip.To16()...)
		}
	} else {
		if len(host) > 255 || host == "" {
			return nil, fmt.Errorf("目标主机名过长或为空")
		}
		b = append([]byte{3, byte(len(host))}, []byte(host)...)
	}
	return append(b, byte(n>>8), byte(n)), nil
}
func readSOCKSAddress(r io.Reader, kind byte) (string, error) {
	var host string
	var n int
	switch kind {
	case 1:
		n = 4
	case 4:
		n = 16
	case 3:
		var b [1]byte
		if _, e := io.ReadFull(r, b[:]); e != nil {
			return "", e
		}
		n = int(b[0])
	default:
		return "", fmt.Errorf("SOCKS 地址类型无效")
	}
	if n == 0 {
		return "", fmt.Errorf("SOCKS 主机为空")
	}
	b := make([]byte, n+2)
	if _, e := io.ReadFull(r, b); e != nil {
		return "", e
	}
	if kind == 3 {
		host = string(b[:n])
	} else {
		host = net.IP(b[:n]).String()
	}
	port := int(binary.BigEndian.Uint16(b[n:]))
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}
func connectSOCKS5(c net.Conn, address, user, password string) error {
	methods := []byte{5, 1, 0}
	if user != "" || password != "" {
		if len(user) > 255 || len(password) > 255 {
			return fmt.Errorf("SOCKS5 用户名或密码过长")
		}
		methods = []byte{5, 1, 2}
	}
	if e := writeAll(c, methods); e != nil {
		return e
	}
	var r [2]byte
	if _, e := io.ReadFull(c, r[:]); e != nil {
		return e
	}
	if r[0] != 5 {
		return fmt.Errorf("代理未返回 SOCKS5")
	}
	if r[1] != methods[2] {
		return fmt.Errorf("SOCKS5 代理选择了未提议的认证方式")
	}
	if r[1] == 2 {
		if len(user) > 255 || len(password) > 255 {
			return fmt.Errorf("SOCKS5 用户名或密码过长")
		}
		b := append([]byte{1, byte(len(user))}, []byte(user)...)
		b = append(b, byte(len(password)))
		b = append(b, []byte(password)...)
		if e := writeAll(c, b); e != nil {
			return e
		}
		if _, e := io.ReadFull(c, r[:]); e != nil {
			return e
		}
		if r[0] != 1 || r[1] != 0 {
			return fmt.Errorf("SOCKS5 代理认证失败")
		}
	} else if r[1] != 0 {
		return fmt.Errorf("SOCKS5 代理不接受认证方式")
	}
	b, e := socksAddress(address)
	if e != nil {
		return e
	}
	if e = writeAll(c, append([]byte{5, 1, 0}, b...)); e != nil {
		return e
	}
	var h [4]byte
	if _, e = io.ReadFull(c, h[:]); e != nil {
		return e
	}
	if h[0] != 5 || h[1] != 0 {
		return fmt.Errorf("SOCKS5 代理连接失败（%d）", h[1])
	}
	_, e = readSOCKSAddress(c, h[3])
	return e
}
func connectSOCKS4(ctx context.Context, c net.Conn, address string, remoteDNS bool, user string) error {
	host, port, e := net.SplitHostPort(address)
	if e != nil {
		return e
	}
	n, _ := strconv.Atoi(port)
	ip := net.ParseIP(host).To4()
	domain := ""
	if ip == nil {
		if remoteDNS {
			ip = net.IPv4(0, 0, 0, 1).To4()
			domain = host
		} else {
			ips, e := net.DefaultResolver.LookupIP(ctx, "ip4", host)
			if e != nil {
				return e
			}
			if len(ips) == 0 {
				return fmt.Errorf("SOCKS4 仅支持 IPv4")
			}
			ip = ips[0].To4()
		}
	}
	if strings.ContainsRune(user, 0) || len(user) > 255 {
		return fmt.Errorf("SOCKS4 用户名无效")
	}
	b := append([]byte{4, 1, byte(n >> 8), byte(n)}, ip...)
	b = append(b, []byte(user)...)
	b = append(b, 0)
	if domain != "" {
		b = append(b, []byte(domain)...)
		b = append(b, 0)
	}
	if e = writeAll(c, b); e != nil {
		return e
	}
	var r [8]byte
	if _, e = io.ReadFull(c, r[:]); e != nil {
		return e
	}
	if r[1] != 90 {
		return fmt.Errorf("SOCKS4 代理连接失败（%d）", r[1])
	}
	return nil
}

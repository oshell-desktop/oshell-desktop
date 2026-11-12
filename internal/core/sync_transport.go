// SPDX-License-Identifier: GPL-3.0-only
package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type SharedObject struct {
	Data []byte
	ETag string
}
type SyncTransport interface {
	Fetch(context.Context) (*SharedObject, error)
	Put(context.Context, []byte, string) error
}
type SyncFailure struct {
	Kind   string
	Status int
}

func (e *SyncFailure) Error() string {
	switch e.Kind {
	case "remote-deleted":
		return "共享同步文件被删除，已停止自动重建"
	case "encryption":
		return "主密码或加密数据校验失败"
	case "conflict":
		return "共享版本已变化，未覆盖；请重新同步"
	case "authentication":
		return "WebDAV 身份验证失败"
	case "etag":
		return "服务器未提供可安全更新的版本标记，已停止覆盖并保留本机修改"
	case "occupied":
		return "另一台设备占用了坚果云写入标记，请稍后重新同步"
	case "uncertain-write":
		return "坚果云写入或释放标记的结果不确定，请核对远端文件和 .oshell-lock 标记后再同步"
	case "redirect":
		return "同步地址发生重定向，请填写最终 HTTP / HTTPS 地址"
	case "size":
		return "同步数据超过大小限制"
	case "tls":
		return "WebDAV TLS 证书校验失败，请核对服务端证书"
	case "tls-changed":
		return "WebDAV 服务端或证书已改变，请重新核对并确认"
	case "tls-validity":
		return "WebDAV 证书已过期或尚未生效，不能信任"
	case "network":
		return "同步网络请求失败或超时，本地数据保留"
	case "unencrypted":
		return "共享文件未加密或格式无效，未覆盖"
	case "directory":
		return "同步目录不可用或包含本机密钥，本地数据保留"
	case "webdav-directory":
		return "WebDAV 同步位置不是可访问的目录，或服务端目录响应无效；未写入同步数据"
	case "file-conflict":
		return "同步目录存在未解决的文件级冲突，请先保留并处理冲突副本"
	}
	return fmt.Sprintf("同步服务返回状态 %d", e.Status)
}
func safeSyncFailure(kind string) error { return &SyncFailure{Kind: kind} }
func ValidETag(tag string) bool {
	b := []byte(tag)
	if len(b) < 2 || len(b) > 1024 || b[0] != '"' || b[len(b)-1] != '"' {
		return false
	}
	for _, v := range b[1 : len(b)-1] {
		if v != 33 && (v < 35 || v == 127) {
			return false
		}
	}
	return true
}

type WebDAVTransport struct {
	resolveMu          sync.Mutex
	resolvedDirectory  *url.URL
	directory          *url.URL
	username, password string
	client             *http.Client
}

// IsHTTPWebDAV identifies an explicit plaintext draft for transport warnings.
func IsHTTPWebDAV(address string) bool {
	u, err := url.Parse(strings.TrimSpace(address))
	return err == nil && strings.EqualFold(u.Scheme, "http")
}

// The final argument is retained for callers of the former loopback-only HTTP API.
// An explicit HTTP address opts into plaintext transport for WebDAV alone.
func NewWebDAVTransport(address, username, password string, _ bool) (*WebDAVTransport, error) {
	u, e := url.Parse(strings.TrimSpace(address))
	invalid := func() (*WebDAVTransport, error) {
		return nil, fmt.Errorf("请输入有效 HTTP / HTTPS 目录地址和端口，账号密码填写在单独字段（最多 4096 字节，不含换行或空字符）")
	}
	if e != nil || u.Hostname() == "" || u.Opaque != "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || strings.Contains(address, "#") || len(username) > 4096 || len(password) > 4096 || strings.ContainsAny(username, ":\r\n\x00") || strings.ContainsAny(password, "\r\n\x00") {
		return invalid()
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if u.Scheme != "https" && u.Scheme != "http" || strings.HasSuffix(u.Host, ":") {
		return invalid()
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return invalid()
		}
	}
	if !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
	}
	if u.RawPath != "" && !strings.HasSuffix(u.RawPath, "/") {
		u.RawPath += "/"
	}
	client := &http.Client{Timeout: 45 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &WebDAVTransport{directory: u, username: username, password: password, client: client}, nil
}
func (w *WebDAVTransport) request(ctx context.Context, method string, target *url.URL, body []byte, headers map[string]string) (int, http.Header, []byte, error) {
	return w.requestLimit(ctx, method, target, body, headers, MaxSharedVault)
}
func (w *WebDAVTransport) requestLimit(ctx context.Context, method string, target *url.URL, body []byte, headers map[string]string, limit int) (int, http.Header, []byte, error) {
	req, e := http.NewRequestWithContext(ctx, method, target.String(), bytes.NewReader(body))
	if e != nil {
		return 0, nil, nil, safeSyncFailure("network")
	}
	req.SetBasicAuth(w.username, w.password)
	req.Header.Set("User-Agent", "OShell Desktop")
	req.Header.Set("Cache-Control", "no-cache")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	response, e := w.client.Do(req)
	if e != nil {
		if ctx.Err() != nil {
			return 0, nil, nil, ctx.Err()
		}
		return 0, nil, nil, webDAVRequestFailure(e)
	}
	defer response.Body.Close()
	if response.StatusCode == 401 || response.StatusCode == 403 {
		return 0, nil, nil, safeSyncFailure("authentication")
	}
	if response.StatusCode >= 300 && response.StatusCode < 400 {
		return 0, nil, nil, safeSyncFailure("redirect")
	}
	if response.ContentLength > int64(limit) {
		return 0, nil, nil, safeSyncFailure("size")
	}
	data, e := io.ReadAll(io.LimitReader(response.Body, int64(limit)+1))
	if e != nil {
		if ctx.Err() != nil {
			return 0, nil, nil, ctx.Err()
		}
		return 0, nil, nil, safeSyncFailure("network")
	}
	if len(data) > limit {
		return 0, nil, nil, safeSyncFailure("size")
	}
	return response.StatusCode, response.Header, data, nil
}
func (w *WebDAVTransport) Probe(ctx context.Context) error {
	_, e := w.ProbeStatus(ctx)
	return e
}
func (w *WebDAVTransport) Fetch(ctx context.Context) (*SharedObject, error) {
	directory, err := w.SyncDirectory(ctx)
	if err != nil {
		return nil, err
	}
	return w.fetchIn(ctx, directory)
}
func (w *WebDAVTransport) fetchIn(ctx context.Context, directory *url.URL) (*SharedObject, error) {
	object, _, e := w.readObject(ctx, webDAVChild(directory, "oshell-vault.json"), MaxSharedVault, "")
	if e != nil || object == nil {
		return nil, e
	}
	if !IsSharedVault(object.Data) {
		return nil, safeSyncFailure("unencrypted")
	}
	return object, nil
}
func (w *WebDAVTransport) Put(ctx context.Context, data []byte, etag string) error {
	if len(data) > MaxSharedVault {
		return safeSyncFailure("size")
	}
	if !IsSharedVault(data) {
		return safeSyncFailure("unencrypted")
	}
	directory, e := w.SyncDirectory(ctx)
	if e != nil {
		return e
	}
	return w.writeObject(ctx, webDAVChild(directory, "oshell-vault.json"), data, etag, "application/json", MaxSharedVault)
}

type DirectoryTransport struct{ Directory string }

func (d DirectoryTransport) check() error {
	info, e := os.Stat(d.Directory)
	if e != nil || !info.IsDir() {
		return safeSyncFailure("directory")
	}
	for _, name := range []string{"local-credential-key.json", "credential-key"} {
		if _, e = os.Lstat(filepath.Join(d.Directory, name)); e == nil {
			return safeSyncFailure("directory")
		}
	}
	return platformSyncConflictCheck(filepath.Join(d.Directory, "configuration.json"))
}
func (d DirectoryTransport) read() (*SharedObject, error) {
	file := filepath.Join(d.Directory, "configuration.json")
	input, e := openImportFile(file)
	if os.IsNotExist(e) {
		return nil, nil
	}
	if e != nil {
		return nil, safeSyncFailure("directory")
	}
	defer input.Close()
	info, e := input.Stat()
	if e != nil || !info.Mode().IsRegular() {
		return nil, safeSyncFailure("directory")
	}
	if info.Size() > MaxSharedVault {
		return nil, safeSyncFailure("size")
	}
	data, e := io.ReadAll(io.LimitReader(input, MaxSharedVault+1))
	if e != nil {
		return nil, safeSyncFailure("directory")
	}
	if len(data) > MaxSharedVault {
		return nil, safeSyncFailure("size")
	}
	if !IsSharedVault(data) {
		return nil, safeSyncFailure("unencrypted")
	}
	digest := sha256.Sum256(data)
	return &SharedObject{data, "\"" + hex.EncodeToString(digest[:]) + "\""}, nil
}
func (d DirectoryTransport) Fetch(ctx context.Context) (*SharedObject, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	if e := d.check(); e != nil {
		return nil, e
	}
	return d.read()
}
func (d DirectoryTransport) Put(ctx context.Context, data []byte, etag string) error {
	if !IsSharedVault(data) {
		return safeSyncFailure("unencrypted")
	}
	if len(data) > MaxSharedVault {
		return safeSyncFailure("size")
	}
	if e := d.check(); e != nil {
		return e
	}
	unlock, e := lockSyncDirectory(ctx, d.Directory)
	if e != nil {
		return e
	}
	defer unlock()
	if e = d.check(); e != nil {
		return e
	}
	current, e := d.read()
	if e != nil {
		return e
	}
	actual := ""
	if current != nil {
		actual = current.ETag
	}
	if actual != etag {
		return safeSyncFailure("conflict")
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	return coordinateSyncWrite(filepath.Join(d.Directory, "configuration.json"), func() error {
		latest, e := d.read()
		if e != nil {
			return e
		}
		tag := ""
		if latest != nil {
			tag = latest.ETag
		}
		if tag != etag {
			return safeSyncFailure("conflict")
		}
		if e = ctx.Err(); e != nil {
			return e
		}
		return AtomicWrite(filepath.Join(d.Directory, "configuration.json"), data)
	})
}

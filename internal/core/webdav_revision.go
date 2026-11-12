package core

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

func (w *WebDAVTransport) nutstore() bool {
	u := w.directory
	return u.Scheme == "https" && strings.EqualFold(u.Hostname(), "dav.jianguoyun.com") && (u.Port() == "" || u.Port() == "443")
}

func nutstoreTag(tag string) bool {
	if ValidETag(tag) {
		return true
	}
	if len(tag) == 0 || len(tag) > 1024 || strings.HasPrefix(tag, "W/") {
		return false
	}
	for _, c := range []byte(tag) {
		if c < 33 || c > 126 || c == '"' || c == ',' || c == '*' {
			return false
		}
	}
	return true
}

// A content revision identifies what was read even on servers with no strong
// ETag. Only the server's own verified version is ever sent in If-Match.
func (w *WebDAVTransport) readObject(ctx context.Context, target *url.URL, limit int, match string) (*SharedObject, string, error) {
	headers := map[string]string{"Accept-Encoding": "identity"}
	if match != "" {
		headers["If-Match"] = match
	}
	status, h, data, err := w.requestLimit(ctx, "GET", target, nil, headers, limit)
	if err != nil || status == 404 {
		return nil, "", err
	}
	if status == 412 {
		return nil, "", safeSyncFailure("conflict")
	}
	if status != 200 {
		return nil, "", &SyncFailure{Kind: "status", Status: status}
	}
	tag := h.Get("ETag")
	revision := tag
	if w.nutstore() || !ValidETag(tag) {
		revision = fmt.Sprintf("sha256:%x", sha256.Sum256(data))
	}
	if w.nutstore() && !nutstoreTag(tag) {
		revision = "readonly:" + revision
	}
	return &SharedObject{data, revision}, tag, nil
}

func (w *WebDAVTransport) propertyTag(ctx context.Context, target *url.URL, expected string, limit int) (string, error) {
	body := []byte(`<d:propfind xmlns:d="DAV:"><d:prop><d:getetag/></d:prop></d:propfind>`)
	status, _, data, err := w.requestLimit(ctx, "PROPFIND", target, body, map[string]string{"Depth": "0", "Content-Type": "application/xml; charset=utf-8"}, 1<<20)
	if err != nil {
		return "", err
	}
	if status == 404 {
		return "", safeSyncFailure("conflict")
	}
	if status == 405 || status == 501 {
		return "", safeSyncFailure("etag")
	}
	if status != 207 {
		return "", &SyncFailure{Kind: "status", Status: status}
	}
	_, tag, ok := davProperties(data, target)
	if !ok || !ValidETag(tag) {
		return "", safeSyncFailure("etag")
	}
	// PROPFIND may describe a newer object than the one used in the merge.
	current, returned, err := w.readObject(ctx, target, limit, tag)
	if err != nil {
		return "", err
	}
	if current == nil || ValidETag(returned) && returned != tag || fmt.Sprintf("sha256:%x", sha256.Sum256(current.Data)) != expected {
		return "", safeSyncFailure("conflict")
	}
	return tag, nil
}

func (w *WebDAVTransport) ProbeStatus(ctx context.Context) (string, error) {
	directory, err := w.SyncDirectory(ctx)
	if err != nil {
		return "", err
	}
	target := webDAVChild(directory, "oshell-vault.json")
	obj, tag, err := w.readObject(ctx, target, MaxSharedVault, "")
	if err != nil {
		return "", err
	}
	if obj == nil {
		return "远端尚无配置", nil
	}
	if !IsSharedVault(obj.Data) {
		return "", safeSyncFailure("unencrypted")
	}
	const readOnly = "远端配置可读取，缺少安全写入所需的版本标记（只读）"
	if w.nutstore() {
		if nutstoreTag(tag) {
			return "可通过坚果云写入标记安全更新", nil
		}
		return readOnly, nil
	}
	if !ValidETag(tag) {
		_, err = w.propertyTag(ctx, target, obj.ETag, MaxSharedVault)
		var failure *SyncFailure
		if errors.As(err, &failure) && failure.Kind == "etag" {
			return readOnly, nil
		}
		if err != nil {
			return "", err
		}
	}
	return "远端配置可按版本条件安全更新", nil
}

func (w *WebDAVTransport) writeObject(ctx context.Context, target *url.URL, data []byte, expected, contentType string, limit int) error {
	if w.nutstore() {
		return w.nutstoreWrite(ctx, target, data, expected, contentType, limit)
	}
	headers := map[string]string{"Content-Type": contentType}
	switch {
	case expected == "":
		headers["If-None-Match"] = "*"
	case ValidETag(expected):
		headers["If-Match"] = expected
	case strings.HasPrefix(expected, "sha256:"):
		tag, err := w.propertyTag(ctx, target, expected, limit)
		if err != nil {
			return err
		}
		headers["If-Match"] = tag
	default:
		return safeSyncFailure("etag")
	}
	status, _, _, err := w.requestLimit(ctx, "PUT", target, data, headers, 65536)
	if err != nil {
		return err
	}
	if status == 412 {
		return safeSyncFailure("conflict")
	}
	if status != 200 && status != 201 && status != 204 {
		return &SyncFailure{Kind: "status", Status: status}
	}
	return nil
}

// Nutstore does not reliably enforce create-only PUT. A non-overwriting MOVE
// publishes either the new payload or the shared OShell writer marker.
func (w *WebDAVTransport) publishNew(ctx context.Context, target *url.URL, data []byte, contentType string) error {
	stage := target.ResolveReference(&url.URL{Path: "oshell-stage-" + NewID() + ".tmp"})
	status, _, _, err := w.requestLimit(ctx, "PUT", stage, data, map[string]string{"Content-Type": contentType, "If-None-Match": "*"}, 65536)
	if err != nil {
		return err
	}
	if status != 201 {
		return &SyncFailure{Kind: "status", Status: status}
	}
	status, _, _, err = w.requestLimit(ctx, "MOVE", stage, nil, map[string]string{"Destination": target.String(), "Overwrite": "F"}, 65536)
	if err != nil || status >= 500 {
		return safeSyncFailure("uncertain-write")
	}
	if status == 201 {
		return nil
	}
	_, _, _, _ = w.requestLimit(ctx, "DELETE", stage, nil, nil, 65536)
	if status == 409 || status == 412 || status == 423 {
		return safeSyncFailure("conflict")
	}
	return &SyncFailure{Kind: "status", Status: status}
}

func (w *WebDAVTransport) nutstoreWrite(ctx context.Context, target *url.URL, data []byte, expected, contentType string, limit int) (err error) {
	if expected == "" {
		return w.publishNew(ctx, target, data, contentType)
	}
	if !strings.HasPrefix(expected, "sha256:") {
		return safeSyncFailure("etag")
	}
	marker := *target
	marker.Path += ".oshell-lock"
	if marker.RawPath != "" {
		marker.RawPath += ".oshell-lock"
	}
	if err = w.publishNew(ctx, &marker, []byte(NewID()), "text/plain"); err != nil {
		var failure *SyncFailure
		if errors.As(err, &failure) && failure.Kind == "conflict" {
			return safeSyncFailure("occupied")
		}
		return err
	}
	// A timed-out PUT can still complete remotely. Keep the marker until its
	// outcome is known instead of admitting a second writer.
	uncertain := false
	defer func() {
		if uncertain {
			return
		}
		status, _, _, e := w.requestLimit(ctx, "DELETE", &marker, nil, nil, 65536)
		if e != nil || status != 200 && status != 204 {
			err = safeSyncFailure("uncertain-write")
		}
	}()
	current, tag, err := w.readObject(ctx, target, limit, "")
	if err != nil {
		return err
	}
	if current == nil || current.ETag != expected {
		return safeSyncFailure("conflict")
	}
	if !nutstoreTag(tag) {
		return safeSyncFailure("etag")
	}
	status, _, _, err := w.requestLimit(ctx, "PUT", target, data, map[string]string{"If-Match": tag, "Content-Type": contentType}, 65536)
	if err != nil || status >= 500 {
		uncertain = true
		return safeSyncFailure("uncertain-write")
	}
	if status == 404 || status == 409 || status == 412 || status == 423 {
		return safeSyncFailure("conflict")
	}
	if status != 200 && status != 204 {
		return &SyncFailure{Kind: "status", Status: status}
	}
	return nil
}

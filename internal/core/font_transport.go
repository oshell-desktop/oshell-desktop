package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func fontAttachmentLimit(name string) (int, error) {
	if name == "library.json" {
		return 65536, nil
	}
	parts := strings.Split(name, ".")
	if len(parts) != 2 {
		return 0, fmt.Errorf("字体附件路径无效")
	}
	asset := FontAsset{SHA256: parts[0], Extension: parts[1], Size: 12}
	if err := (FontManifest{Format: "OShell.font-library", Version: 1, Fonts: []FontAsset{asset}}).validate(); err != nil {
		return 0, fmt.Errorf("字体附件路径无效")
	}
	return MaxFontFile, nil
}

func (d DirectoryTransport) fontPath(name string, create bool) (string, int, error) {
	limit, err := fontAttachmentLimit(name)
	if err != nil {
		return "", 0, err
	}
	if err = d.check(); err != nil {
		return "", 0, err
	}
	l := NewFontLibrary(d.Directory)
	if err = l.CheckDirectory(); err != nil {
		return "", 0, err
	}
	if create {
		if err = os.MkdirAll(l.Directory, 0700); err != nil {
			return "", 0, err
		}
	}
	path := filepath.Join(l.Directory, name)
	if err = platformSyncConflictCheck(path); err != nil {
		return "", 0, err
	}
	return path, limit, nil
}
func (d DirectoryTransport) FetchFont(ctx context.Context, name string) (*SharedObject, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, limit, err := d.fontPath(name, false)
	if err != nil {
		return nil, err
	}
	data, err := readFontFile(path, int64(limit))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	return &SharedObject{Data: data, ETag: "\"" + hex.EncodeToString(sum[:]) + "\""}, nil
}
func (d DirectoryTransport) PutFont(ctx context.Context, name string, data []byte, etag string) error {
	limit, err := fontAttachmentLimit(name)
	if err != nil {
		return err
	}
	if len(data) > limit {
		return safeSyncFailure("size")
	}
	if err = d.check(); err != nil {
		return err
	}
	unlock, err := lockSyncDirectory(ctx, d.Directory)
	if err != nil {
		return err
	}
	defer unlock()
	path, _, err := d.fontPath(name, true)
	if err != nil {
		return err
	}
	return coordinateSyncWrite(path, func() error {
		current, err := d.FetchFont(ctx, name)
		if err != nil {
			return err
		}
		actual := ""
		if current != nil {
			actual = current.ETag
		}
		if actual != etag {
			return safeSyncFailure("conflict")
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		return AtomicWrite(path, data)
	})
}

func (w *WebDAVTransport) FetchFont(ctx context.Context, name string) (*SharedObject, error) {
	limit, err := fontAttachmentLimit(name)
	if err != nil {
		return nil, err
	}
	directory, err := w.SyncDirectory(ctx)
	if err != nil {
		return nil, err
	}
	status, headers, data, err := w.requestLimit(ctx, "GET", webDAVChild(directory, "fonts/"+name), nil, nil, limit)
	if err != nil {
		return nil, err
	}
	if status == 404 {
		return nil, nil
	}
	if status != 200 {
		return nil, &SyncFailure{Kind: "status", Status: status}
	}
	etag := headers.Get("ETag")
	if !ValidETag(etag) {
		return nil, safeSyncFailure("etag")
	}
	return &SharedObject{Data: data, ETag: etag}, nil
}
func (w *WebDAVTransport) PutFont(ctx context.Context, name string, data []byte, etag string) error {
	limit, err := fontAttachmentLimit(name)
	if err != nil {
		return err
	}
	if len(data) > limit {
		return safeSyncFailure("size")
	}
	headers := map[string]string{"Content-Type": "application/octet-stream"}
	if name == "library.json" {
		headers["Content-Type"] = "application/json"
	}
	if etag == "" {
		headers["If-None-Match"] = "*"
	} else {
		if !ValidETag(etag) {
			return safeSyncFailure("etag")
		}
		headers["If-Match"] = etag
	}
	directory, err := w.SyncDirectory(ctx)
	if err != nil {
		return err
	}
	folder := webDAVChild(directory, "fonts/")
	exists, err := w.collectionExists(ctx, folder)
	if err != nil {
		return err
	}
	if !exists {
		status, _, _, err := w.request(ctx, "MKCOL", folder, nil, nil)
		if err != nil {
			return err
		}
		if status != 201 && status != 405 {
			return &SyncFailure{Kind: "status", Status: status}
		}
		if exists, err = w.collectionExists(ctx, folder); err != nil {
			return err
		} else if !exists {
			return safeSyncFailure("webdav-directory")
		}
	}
	status, _, _, err := w.requestLimit(ctx, "PUT", webDAVChild(folder, name), data, headers, 65536)
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

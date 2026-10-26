package core

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestClipboardImageCacheAndValidation(t *testing.T) {
	var buf bytes.Buffer
	_ = png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 4, 4)))
	data, err := NormalizeClipboardImage(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = NormalizeClipboardImage([]byte("not image")); err == nil {
		t.Fatal("invalid image accepted")
	}
	dir := t.TempDir()
	p, err := SaveClipboardImage(dir, data, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-8 * 24 * time.Hour)
	_ = os.Chtimes(p, old, old)
	unrelated := filepath.Join(filepath.Dir(p), "keep.png")
	_ = os.WriteFile(unrelated, data, 0600)
	_ = os.Chtimes(unrelated, old, old)
	next, err := SaveClipboardImage(dir, data, time.Now())
	if err != nil || next == p {
		t.Fatal(err)
	}
	if _, err = os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("old cache retained")
	}
	if _, err = os.Stat(unrelated); err != nil {
		t.Fatal("unrelated file removed")
	}
	if got, err := ClipboardImageShellPath("/tmp/a'b;$(x).png", false); err != nil || got != `/tmp/a\'b\;\$\(x\).png` {
		t.Fatal(got)
	}
}

func TestClipboardImageSSHUploadAndCancel(t *testing.T) {
	c := imageSFTPClient(t, false)
	base := "/"
	data := []byte("synthetic-image")
	first, err := UploadClipboardImage(context.Background(), c, base, data)
	if err != nil {
		t.Fatal(err)
	}
	second, err := UploadClipboardImage(context.Background(), c, base, data)
	if err != nil || second == first {
		t.Fatal("unique upload", err)
	}
	f, err := c.Open(first)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err := io.ReadAll(f)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatal("bad upload", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = UploadClipboardImage(ctx, c, base, data); err == nil {
		t.Fatal("cancel ignored")
	}
	if _, err = UploadClipboardImage(context.Background(), c, ".", data); err != nil {
		t.Fatal("relative existing remote path rejected", err)
	}
}

func TestClipboardImageRejectsIgnoredPermissionsAndCleansPartial(t *testing.T) {
	c := imageSFTPClient(t, true)
	if _, err := UploadClipboardImage(context.Background(), c, "/", []byte("synthetic")); err == nil {
		t.Fatal("ignored chmod accepted")
	}
	files, err := c.ReadDir("/")
	if err != nil || len(files) != 0 {
		t.Fatal("failed upload left data", files, err)
	}
}

func TestClipboardImageProgressCancellationAndPathSafety(t *testing.T) {
	c := imageSFTPClient(t, false)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	data := bytes.Repeat([]byte("x"), 100000)
	var sent int64
	_, err := UploadClipboardImage(ctx, c, "/", data, func(n, total int64) {
		sent = n
		if total != int64(len(data)) {
			t.Error(total)
		}
		cancel()
	})
	if err == nil || sent == 0 || sent >= int64(len(data)) {
		t.Fatal("chunk cancellation", sent, err)
	}
	files, err := c.ReadDir("/")
	if err != nil || len(files) != 0 {
		t.Fatal("cancelled upload left data", files, err)
	}
	for _, p := range []string{"", "/tmp/x\ncommand", "/tmp/x\x00"} {
		if _, err := ClipboardImageShellPath(p, false); err == nil {
			t.Fatal("unsafe shell path", p)
		}
	}
}

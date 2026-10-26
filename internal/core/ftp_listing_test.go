package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFTPListingFallbackPreservesUnknownEntries(t *testing.T) {
	for _, mode := range []string{"list", "partial", "nlst"} {
		t.Run(mode, func(t *testing.T) {
			profile, root := ftpTestServer(t, mode)
			if err := os.Mkdir(filepath.Join(root, "目录 with spaces"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "data.txt"), []byte("roundtrip"), 0600); err != nil {
				t.Fatal(err)
			}
			session, fs, err := LoginFTP(context.Background(), profile, "test-only")
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			defer fs.Close()
			entries, err := fs.ReadDir("/")
			if err != nil || len(entries) != 2 {
				t.Fatalf("lost fallback entries: %v %v", entries, err)
			}
			for _, entry := range entries {
				if RemoteFileTypeKnown(entry) != (mode != "nlst") {
					t.Fatal("invented server type", entry.Name())
				}
				if mode == "nlst" && (entry.Size() != 0 || !entry.ModTime().IsZero() || RemoteFileAttributes(entry).Permissions != "—") {
					t.Fatal("invented NLST metadata")
				}
			}
			if actual, err := fs.RealPath("/目录 with spaces"); err != nil || actual != "/目录 with spaces" {
				t.Fatal("unknown directory could not be entered", actual, err)
			}
			if entries, err := fs.ReadDir("/目录 with spaces"); err != nil || len(entries) != 0 {
				t.Fatal("empty directory", entries, err)
			}
			if _, err := fs.RealPath("/data.txt"); err == nil {
				t.Fatal("file accepted as directory")
			}
			local := filepath.Join(t.TempDir(), "download.txt")
			if err := fs.Download(context.Background(), "/data.txt", local, false, nil); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(local)
			if err != nil || string(data) != "roundtrip" {
				t.Fatal("unknown file download", string(data), err)
			}
		})
	}
}

func TestFTPNameFallbackRejectsInvalidUTF8(t *testing.T) {
	profile, _ := ftpTestServer(t, "invalid-utf8")
	session, fs, err := LoginFTP(context.Background(), profile, "test-only")
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	defer fs.Close()
	if _, err := fs.ReadDir("/"); err == nil || !strings.Contains(err.Error(), "UTF-8") {
		t.Fatal("invalid filename accepted", err)
	}
}

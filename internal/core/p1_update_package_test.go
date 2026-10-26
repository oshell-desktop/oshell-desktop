package core

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// An opt-in real package fixture exercises the production metadata -> download
// -> signature -> extraction chain without installation or real release access.
func TestP1SignedPortablePackagePreparation(t *testing.T) {
	archive := os.Getenv("OSHELL_P1_UPDATE_ARCHIVE")
	if archive == "" {
		t.Skip("set OSHELL_P1_UPDATE_ARCHIVE to a locally built Windows ZIP")
	}
	body, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	_, metadata, pub := signedPortableFixture(t, "windows-amd64", "0.3.4", body)
	setPortableSigningKey(t, pub)
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	for _, corrupt := range []bool{false, true} {
		downloads := 0
		http.DefaultTransport = updateRoundTrip(func(r *http.Request) (*http.Response, error) {
			var response []byte
			switch r.URL.String() {
			case "https://owner.github.io/repo/updates/latest.json":
				response = metadata
			case "https://github.com/owner/repo/releases/download/v0.3.4/OShell-Desktop-windows-amd64-single-file.zip":
				downloads++
				response = bytes.Clone(body)
				if corrupt {
					response[len(response)/2] ^= 1
				}
			default:
				t.Fatalf("unexpected network access: %s", r.URL)
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(response)), ContentLength: int64(len(response)), Header: make(http.Header)}, nil
		})
		u, err := CheckReleaseUpdate(context.Background(), &UpdateMetadataCache{}, UpdateSource{"owner/repo"}, "windows-amd64", "0.3.3")
		if err != nil || u == nil || downloads != 0 {
			t.Fatal("metadata check downloaded prematurely", err)
		}
		dir := t.TempDir()
		exe := filepath.Join(dir, "OShell Desktop.exe")
		os.WriteFile(exe, []byte("old synthetic program"), 0700)
		os.WriteFile(filepath.Join(dir, "config.json"), []byte("synthetic sentinel"), 0600)
		prepared, err := u.Prepare(context.Background(), exe, nil)
		if corrupt {
			if err == nil || prepared != nil {
				t.Fatal("corrupt package reached extraction")
			}
		} else {
			if err != nil || prepared == nil {
				t.Fatal("signed package preparation failed", err)
			}
			if _, err := os.Stat(filepath.Join(prepared.stage(), "payload", "OShell Desktop.exe")); err != nil {
				t.Fatal(err)
			}
			prepared.Discard()
		}
		if downloads != 1 {
			t.Fatal("unexpected download count")
		}
		files, _ := os.ReadDir(dir)
		if len(files) != 2 {
			t.Fatal("failed/discarded update left staged files")
		}
		if b, _ := os.ReadFile(exe); string(b) != "old synthetic program" {
			t.Fatal("prepare installed without authorization")
		}
		if b, _ := os.ReadFile(filepath.Join(dir, "config.json")); string(b) != "synthetic sentinel" {
			t.Fatal("configuration overwritten")
		}
	}
}

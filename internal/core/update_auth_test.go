package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUpdateTokenLocalEncryptionAndRepositoryIsolation(t *testing.T) {
	dir := t.TempDir()
	secret := "github_pat_synthetic_fixture_only"
	if e := SaveUpdateToken(dir, "Owner/Repo", secret); e != nil {
		t.Fatal(e)
	}
	if token, e := ReadUpdateToken(dir, "owner/repo"); e != nil || token != secret {
		t.Fatal("token unavailable", e)
	}
	if token, e := ReadUpdateToken(dir, "owner/other"); e != nil || token != "" {
		t.Fatal("token crossed repository", e)
	}
	data, e := os.ReadFile(filepath.Join(dir, "update-auth", "credentials.json"))
	if e != nil || bytes.Contains(data, []byte(secret)) {
		t.Fatal("token not encrypted", e)
	}
	if _, e := os.Stat(filepath.Join(dir, "credentials.json")); !os.IsNotExist(e) {
		t.Fatal("session credential store modified")
	}
	if e := SaveUpdateToken(dir, "owner/repo", "bad\r\nheader"); e == nil {
		t.Fatal("header injection accepted")
	}
	if e := SaveUpdateToken(dir, "owner/repo", ""); e != nil {
		t.Fatal(e)
	}
	if token, e := ReadUpdateToken(dir, "owner/repo"); e != nil || token != "" {
		t.Fatal("token not removed", e)
	}
}

type releaseRoundTripper func(*http.Request) (*http.Response, error)

func (f releaseRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestPrivateReleaseUsesAuthenticatedAssetAPI(t *testing.T) {
	source := UpdateSource{Repository: "owner/repo"}
	data := []byte("private release fixture")
	hash := sha256.Sum256(data)
	asset := releaseAsset{ID: 123, Name: releaseAssetName("windows-amd64"), URL: "https://github.com/owner/repo/releases/download/v0.3.2/" + releaseAssetName("windows-amd64"), State: "uploaded", Digest: "sha256:" + hex.EncodeToString(hash[:]), Size: int64(len(data))}
	_, signed, pub := signedPortableFixture(t, "windows-amd64", "0.3.2", data)
	setPortableSigningKey(t, pub)
	metadata, _ := json.Marshal(privateSignedMetadata{Release: githubRelease{Tag: "v0.3.2", Assets: []releaseAsset{asset}}, Signed: signed})
	cache := &UpdateMetadataCache{data: metadata, expires: time.Now().Add(time.Minute)}
	u, e := CheckReleaseUpdate(context.Background(), cache, source, "windows-amd64", "0.3.1", "synthetic-token")
	if e != nil || u == nil {
		t.Fatal(e)
	}
	client := &http.Client{Transport: releaseRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://api.github.com/repos/owner/repo/releases/assets/123" || r.Header.Get("Authorization") != "Bearer synthetic-token" || r.Header.Get("Accept") != "application/octet-stream" {
			t.Fatal("incorrect private download request")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data)), ContentLength: int64(len(data)), Header: make(http.Header)}, nil
	})}
	if _, e = u.download(context.Background(), t.TempDir(), nil, client); e != nil {
		t.Fatal(e)
	}
	redirect, _ := http.NewRequest("GET", "https://release-assets.githubusercontent.com/fixture", nil)
	redirect.Header.Set("Authorization", "Bearer synthetic-token")
	if e = updateDownloadClient().CheckRedirect(redirect, nil); e != nil || redirect.Header.Get("Authorization") != "" {
		t.Fatal("token leaked to asset CDN", e)
	}
	u.assetAPIURL = "https://evil.invalid/download"
	if _, e = u.download(context.Background(), t.TempDir(), nil, client); e == nil || !strings.Contains(e.Error(), "地址无效") {
		t.Fatal("token allowed on foreign host")
	}
}

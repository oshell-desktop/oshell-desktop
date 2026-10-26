package core

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json/v2"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func signedPortableFixture(t *testing.T, target, version string, body []byte) (*ReleaseUpdate, []byte, string) {
	t.Helper()
	m, key, pub, _ := updateFixture(t)
	digest := sha256.Sum256(body)
	m.Version = version
	m.Build = "100"
	m.Platforms = map[string]UpdateArchive{target: {URL: "https://github.com/owner/repo/releases/download/v" + version + "/" + releaseAssetName(target), Size: int64(len(body)), SHA256: hex.EncodeToString(digest[:]), Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(key, digest[:]))}}
	data, err := SignUpdateManifest(m, key)
	if err != nil {
		t.Fatal(err)
	}
	v, err := DecodeSignedUpdate(data, UpdateSource{"owner/repo"}, target, "dev.oshell.desktop", "0.3.1", "1", pub)
	if err != nil || v == nil {
		t.Fatal(err)
	}
	return portableSignedCandidate(v), data, pub
}

func setPortableSigningKey(t *testing.T, key string) {
	t.Helper()
	oldKey, oldBuild := UpdatePublicKey, UpdateBuild
	UpdatePublicKey, UpdateBuild = key, "1"
	t.Cleanup(func() { UpdatePublicKey, UpdateBuild = oldKey, oldBuild })
}

func TestP1PublicUpdateUsesStaticSignedMetadataAndCoalesces(t *testing.T) {
	_, data, pub := signedPortableFixture(t, "windows-amd64", "0.3.2", []byte("fixture-package"))
	setPortableSigningKey(t, pub)
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	var calls atomic.Int32
	http.DefaultTransport = updateRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if r.URL.String() != "https://owner.github.io/repo/updates/latest.json" || r.Header.Get("Authorization") != "" {
			t.Error("public check called API or leaked credentials", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data)), ContentLength: int64(len(data)), Header: make(http.Header)}, nil
	})
	cache := &UpdateMetadataCache{}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			u, err := CheckReleaseUpdate(context.Background(), cache, UpdateSource{"owner/repo"}, "windows-amd64", "0.3.1")
			if err != nil || u == nil || u.verifyAuthority() != nil {
				t.Error("signed production candidate unavailable", err)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal("duplicate prefetch", calls.Load())
	}
	if u, err := checkSignedPortableUpdate(context.Background(), cache, UpdateSource{"owner/repo"}, "windows-amd64", "0.3.2", "100", pub, ""); err != nil || u != nil {
		t.Fatal("same version offered without newer build context", u, err)
	}
}

func TestP1PortableUpdateRejectsUnsignedAndMismatchedMetadata(t *testing.T) {
	_, data, pub := signedPortableFixture(t, "windows-amd64", "0.3.2", []byte("fixture"))
	setPortableSigningKey(t, pub)
	for _, tc := range []struct {
		name        string
		data        []byte
		key, target string
	}{
		{"unsigned", []byte(`{"tag_name":"v0.3.2"}`), pub, "windows-amd64"},
		{"no key", data, "", "windows-amd64"},
		{"wrong key", data, base64.StdEncoding.EncodeToString(make([]byte, 32)), "windows-amd64"},
		{"wrong platform", data, pub, "linux-amd64"},
		{"oversize JSON", bytes.Repeat([]byte{'x'}, UpdateMetadataLimit+1), pub, "windows-amd64"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cache := &UpdateMetadataCache{data: tc.data, expires: time.Now().Add(time.Minute)}
			if _, err := checkSignedPortableUpdate(context.Background(), cache, UpdateSource{"owner/repo"}, tc.target, "0.3.1", "1", tc.key, ""); err == nil {
				t.Fatal("invalid metadata accepted")
			}
		})
	}
	var env signedUpdateEnvelope
	json.Unmarshal(data, &env)
	env.Payload = bytes.Repeat([]byte{' '}, 32769)
	oversize, _ := json.Marshal(env)
	if _, err := DecodeSignedUpdate(oversize, UpdateSource{"owner/repo"}, "windows-amd64", "dev.oshell.desktop", "0.3.1", "1", pub); err == nil {
		t.Fatal("oversized feed accepted")
	}
	u := &ReleaseUpdate{Version: "0.3.2"}
	if _, err := u.Prepare(context.Background(), "must-not-be-touched", nil); err == nil || !strings.Contains(err.Error(), "签名") {
		t.Fatal("unsigned prepare reached filesystem", err)
	}
}

func TestP1StaticMetadataRejectsRedirectAndLength(t *testing.T) {
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	for _, mode := range []string{"redirect", "declared limit", "stream limit"} {
		calls := 0
		http.DefaultTransport = updateRoundTrip(func(r *http.Request) (*http.Response, error) {
			calls++
			resp := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header), ContentLength: -1}
			switch mode {
			case "redirect":
				resp.StatusCode = 302
				resp.Header.Set("Location", "https://other.invalid/metadata")
			case "declared limit":
				resp.ContentLength = UpdateMetadataLimit + 1
			case "stream limit":
				resp.Body = io.NopCloser(bytes.NewReader(bytes.Repeat([]byte{'x'}, UpdateMetadataLimit+1)))
			}
			return resp, nil
		})
		if _, err := FetchUpdateMetadata(context.Background(), UpdateSource{"owner/repo"}); err == nil || calls != 1 {
			t.Fatal("redirect/length not rejected", mode, err, calls)
		}
	}
}

func TestP1PrivateReleaseRequiresSignedAttachment(t *testing.T) {
	u, signed, pub := signedPortableFixture(t, "windows-amd64", "0.3.2", []byte("private-fixture"))
	setPortableSigningKey(t, pub)
	asset := u.asset
	asset.ID = 7
	asset.Name = releaseAssetName("windows-amd64")
	asset.State = "uploaded"
	m := privateSignedMetadata{Release: githubRelease{Tag: "v0.3.2", Assets: []releaseAsset{asset}}, Signed: signed}
	for _, kind := range []string{"valid", "missing signature", "wrong asset", "duplicate"} {
		copy := m
		copy.Release.Assets = append([]releaseAsset(nil), m.Release.Assets...)
		switch kind {
		case "missing signature":
			copy.Signed = []byte(`{}`)
		case "wrong asset":
			copy.Release.Assets[0].Size++
		case "duplicate":
			copy.Release.Assets = append(copy.Release.Assets, asset)
		}
		data, _ := json.Marshal(copy)
		cache := &UpdateMetadataCache{data: data, expires: time.Now().Add(time.Minute)}
		got, err := CheckReleaseUpdate(context.Background(), cache, UpdateSource{"owner/repo"}, "windows-amd64", "0.3.1", "fixture-token")
		if (err == nil) != (kind == "valid") {
			t.Fatal(kind, err)
		}
		if err == nil && (got == nil || got.verified == nil) {
			t.Fatal("private path dropped signature")
		}
	}
}

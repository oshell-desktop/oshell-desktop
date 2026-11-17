package core

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWebDAVDigestRFCAndChallenges(t *testing.T) {
	d, err := parseWebDAVDigest(`Digest realm="testrealm@host.com", qop="auth,auth-int", nonce="dcd98b7102dd2f0e8b11d0f600bfb0c093", opaque="5ccc069c403ebaf9f0171e9517f40e41"`)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse("http://host/dir/index.html")
	auth, err := d.authorization("GET", u, "Mufasa", "Circle Of Life", "0a4f113b")
	if err != nil || !strings.Contains(auth, `response="6629fae49393a05397450978507c4ef1"`) {
		t.Fatal(auth, err)
	}
	// RFC 7616 section 3.9.1 published SHA-256 test vector.
	d, err = parseWebDAVDigest(`Digest realm="http-auth@example.org", qop="auth, auth-int", nonce="7ypf/xlj9XXwfDPEoM4URrv/xwf94BcCAzFZH4GiTo0v", algorithm=SHA-256`)
	if err != nil {
		t.Fatal(err)
	}
	auth, err = d.authorization("GET", u, "Mufasa", "Circle of Life", "f2/wE4q74E6zIJEtWaHKaf5wv/H5QzzpXusqGemxURZJ")
	if err != nil || !strings.Contains(auth, `response="753927fa0e85d155564e2e272a28d1802ca10daf4496794697cf8db5856cb6c1"`) {
		t.Fatal(auth, err)
	}
	d, err = parseWebDAVDigest(`Basic realm="first", Digest realm="a,\"b", nonce="n", algorithm=MD5, Digest realm="strong", nonce="s", algorithm=SHA-256-sess, qop=auth`)
	if err != nil || d.algorithm != "SHA-256-SESS" {
		t.Fatal(d, err)
	}
	for _, invalid := range []string{`Basic realm="a"`, `Digest realm="a", nonce="n", nonce="other"`, `Digest realm="a", nonce="n", qop="auth-int"`, `Digest realm="a", nonce="n", algorithm=SHA-512`, `Digest realm="a", nonce="n", userhash=true`} {
		if _, err := parseWebDAVDigest(invalid); err == nil {
			t.Fatal(invalid)
		}
	}
}

func TestWebDAVDigestSettingsAndRefusesBasicFallback(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "sync-settings.json"), []byte(`{"version":1,"backend":"webdav"}`), 0600); err != nil {
		t.Fatal(err)
	}
	options, err := LoadSyncOptions(root)
	if err != nil || options.Authentication != "" {
		t.Fatal(options, err)
	}
	options.Authentication = "digest"
	options.Enabled = true
	options.Address = "https://fixture.invalid/vault"
	master := []byte("synthetic-master")
	if err := SaveSyncOptions(root, options, master); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSyncOptions(root); err != ErrSyncSettingsLocked {
		t.Fatal(err)
	}
	reopened, err := LoadSyncOptions(root, master)
	if err != nil || reopened.Authentication != "digest" {
		t.Fatal(reopened, err)
	}
	for _, challenge := range []string{`Basic realm="fixture"`, `Digest realm="fixture",nonce="same",qop=auth`} {
		t.Run(challenge, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if strings.HasPrefix(r.Header.Get("Authorization"), "Basic ") {
					t.Error("fell back to Basic")
				}
				w.Header().Set("WWW-Authenticate", challenge)
				w.WriteHeader(http.StatusUnauthorized)
			}))
			defer server.Close()
			transport, err := (SyncOptions{Backend: "webdav", Address: server.URL, Authentication: "digest"}).Transport("fixture", "")
			if err != nil {
				t.Fatal(err)
			}
			dav := transport.(*WebDAVTransport)
			if _, _, _, err := dav.request(context.Background(), "GET", dav.directory, nil, nil); err == nil || requests > 2 {
				t.Fatal("unbounded or accepted authentication", requests, err)
			}
		})
	}
}

func TestWebDAVDigestTransportReplaysBodyAndStops(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		data, _ := io.ReadAll(r.Body)
		if string(data) != "body" || r.Header.Get("If-Match") != `"v1"` {
			t.Error("lost body/condition")
		}
		if strings.HasPrefix(r.Header.Get("Authorization"), "Basic ") {
			t.Error("digest sent basic credentials")
		}
		if requests < 3 {
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Digest realm="test", nonce="%d", algorithm=SHA-256, qop=auth, stale=true`, requests))
			w.WriteHeader(401)
			return
		}
		if !strings.Contains(r.Header.Get("Authorization"), `nonce="2"`) {
			t.Error("stale nonce not refreshed")
		}
		w.Write([]byte("ok"))
	}))
	defer server.Close()
	w, err := (SyncOptions{Backend: "webdav", Address: server.URL, Username: "user", Authentication: "digest"}).Transport("pass", "")
	if err != nil {
		t.Fatal(err)
	}
	dav := w.(*WebDAVTransport)
	status, _, data, err := dav.request(context.Background(), "PUT", dav.directory, []byte("body"), map[string]string{"If-Match": `"v1"`})
	if err != nil || status != 200 || string(data) != "ok" || requests != 3 {
		t.Fatal(status, string(data), requests, err)
	}
	if (SyncOptions{Backend: "webdav", Authentication: "unsupported"}).validateAuthentication() == nil {
		t.Fatal("unknown auth accepted")
	}
}

func TestQuickHistoryPersistencePagingDedupAndRetention(t *testing.T) {
	store := QuickHistoryStore{Directory: t.TempDir()}
	for i := 0; i < 60; i++ {
		if err := store.Remember(fmt.Sprint(i), true, 500); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Remember("0", true, 500); err != nil {
		t.Fatal(err)
	}
	page, more, err := (QuickHistoryStore{Directory: store.Directory}).Page("")
	if err != nil || !more || len(page) != 50 || page[0].Text != "0" {
		t.Fatal(page, more, err)
	}
	rest, more, err := store.Page(page[49].ID)
	if err != nil || more || len(rest) != 10 {
		t.Fatal(len(rest), more, err)
	}
	store.Remember("0", false, 500)
	if err := store.Trim(2); err != nil {
		t.Fatal(err)
	}
	page, _, _ = store.Page("")
	if len(page) != 2 || page[0].AppendReturn || !page[1].AppendReturn {
		t.Fatal(page)
	}
	if err := store.Trim(0); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{" ", "a\x00b", strings.Repeat("x", 65537)} {
		store.Remember(text, true, 500)
	}
	page, _, _ = store.Page("")
	if len(page) != 0 {
		t.Fatal(page)
	}
	store.Remember(strings.Repeat("x", 65536), true, 500)
	store.Remember("latest", true, 500)
	page, more, err = store.Page("")
	if err != nil || !more || len(page) != 1 {
		t.Fatal(len(page), more, err)
	}
	rest, more, err = store.Page(page[0].ID)
	if err != nil || more || len(rest) != 1 || len(rest[0].Text) != 65536 {
		t.Fatal(len(rest), more, err)
	}
}

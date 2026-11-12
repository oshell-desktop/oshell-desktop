package core

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
)

func TestFontSyncWebDAVWireMergeAndFailure(t *testing.T) {
	ctx := context.Background()
	storage := DirectoryTransport{t.TempDir()}
	var mu sync.Mutex
	folder, weak, rejectManifest := false, false, false
	var puts []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		u, p, ok := r.BasicAuth()
		if !ok || u != "font-fixture" || p != "synthetic-password" {
			w.WriteHeader(401)
			return
		}
		if r.Method == "PROPFIND" {
			if r.URL.Path == "/oshell/" || r.URL.Path == "/oshell/fonts/" && folder {
				writeDAVCollection(w, r.URL.Path)
			} else {
				w.WriteHeader(404)
			}
			return
		}
		if r.Method == "MKCOL" && r.URL.Path == "/oshell/fonts/" {
			folder = true
			w.WriteHeader(201)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/oshell/fonts/") {
			http.NotFound(w, r)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/oshell/fonts/")
		switch r.Method {
		case "GET":
			object, err := storage.FetchFont(r.Context(), name)
			if err != nil {
				w.WriteHeader(500)
				return
			}
			if object == nil {
				w.WriteHeader(404)
				return
			}
			tag := object.ETag
			if weak {
				tag = "W/" + tag
			}
			w.Header().Set("ETag", tag)
			w.Write(object.Data)
		case "PUT":
			if name == "library.json" && rejectManifest {
				w.WriteHeader(412)
				return
			}
			wantType := "application/octet-stream"
			if name == "library.json" {
				wantType = "application/json"
			}
			if r.Header.Get("Content-Type") != wantType {
				t.Error("incorrect content type")
				w.WriteHeader(400)
				return
			}
			tag := r.Header.Get("If-Match")
			if tag == "" && r.Header.Get("If-None-Match") != "*" {
				t.Error("unconditional PUT")
				w.WriteHeader(400)
				return
			}
			data, err := io.ReadAll(io.LimitReader(r.Body, MaxFontFile+1))
			if err != nil {
				w.WriteHeader(500)
				return
			}
			if err = storage.PutFont(r.Context(), name, data, tag); err != nil {
				w.WriteHeader(412)
				return
			}
			puts = append(puts, name)
			w.WriteHeader(204)
		default:
			w.WriteHeader(405)
		}
	}))
	defer server.Close()
	transport, err := NewWebDAVTransport(server.URL+"/oshell", "font-fixture", "synthetic-password", true)
	if err != nil {
		t.Fatal(err)
	}
	a, b := NewFontLibrary(t.TempDir()), NewFontLibrary(t.TempDir())
	baseA, baseB := FontBaselinePath(t.TempDir(), "dav"), FontBaselinePath(t.TempDir(), "dav")
	// Font attachments have a 20 MiB limit, independently of the 16 MiB vault.
	large := make([]byte, 17<<20)
	copy(large, gomono.TTF)
	asset := importSyncFont(t, a, large)
	if err = SynchronizeFonts(ctx, a, transport, baseA); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	ordered := len(puts) == 2 && puts[0] == asset.Name() && puts[1] == "library.json"
	mu.Unlock()
	if !ordered {
		t.Fatal("manifest published before object")
	}
	if err = SynchronizeFonts(ctx, b, transport, baseB); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(b.Directory, asset.Name()))
	if err != nil || !bytes.Equal(got, large) {
		t.Fatal("large font download differs", err)
	}
	mu.Lock()
	writes := len(puts)
	mu.Unlock()
	if err = SynchronizeFonts(ctx, b, transport, baseB); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	unchanged := len(puts) == writes
	mu.Unlock()
	if !unchanged {
		t.Fatal("no-op uploaded again")
	}
	regular := importSyncFont(t, b, goregular.TTF)
	mu.Lock()
	rejectManifest = true
	mu.Unlock()
	if err = SynchronizeFonts(ctx, b, transport, baseB); err == nil {
		t.Fatal("412 accepted")
	}
	if len(fontNames(t, NewFontLibrary(storage.Directory))) != 1 {
		t.Fatal("412 replaced manifest")
	}
	mu.Lock()
	rejectManifest = false
	mu.Unlock()
	if err = SynchronizeFonts(ctx, b, transport, baseB); err != nil {
		t.Fatal(err)
	}
	if err = SynchronizeFonts(ctx, a, transport, baseA); err != nil {
		t.Fatal(err)
	}
	removeSyncFont(t, b, regular)
	if err = SynchronizeFonts(ctx, b, transport, baseB); err != nil {
		t.Fatal(err)
	}
	if err = SynchronizeFonts(ctx, a, transport, baseA); err != nil {
		t.Fatal(err)
	}
	if len(fontNames(t, a)) != 1 {
		t.Fatal("WebDAV deletion not merged")
	}
	mu.Lock()
	weak = true
	mu.Unlock()
	object, err := transport.FetchFont(ctx, "library.json")
	if err != nil || object == nil || ValidETag(object.ETag) {
		t.Fatal("expected a readable manifest with a content revision", err)
	}
	if err = transport.PutFont(ctx, "library.json", object.Data, object.ETag); err == nil {
		t.Fatal("unversioned manifest overwritten")
	}
	mu.Lock()
	weak = false
	mu.Unlock()
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err = SynchronizeFonts(cancelled, b, transport, baseB); err == nil {
		t.Fatal("cancelled sync ran")
	}
	for _, name := range []string{"../configuration.json", "sub/library.json", strings.Repeat("A", 64) + ".ttf", strings.Repeat("0", 64) + ".exe"} {
		if _, err := transport.FetchFont(ctx, name); err == nil {
			t.Fatal("invalid attachment path accepted", name)
		}
	}
}

func TestFontWebDAVRejectsOversizedResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PROPFIND" {
			writeDAVCollection(w, r.URL.Path)
			return
		}
		w.Header().Set("ETag", `"fixture"`)
		limit := MaxFontFile
		if strings.HasSuffix(r.URL.Path, "library.json") {
			limit = 65536
		}
		w.Write(bytes.Repeat([]byte(" "), limit+1))
	}))
	defer server.Close()
	transport, err := NewWebDAVTransport(server.URL+"/oshell", "", "", true)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"library.json", strings.Repeat("0", 64) + ".ttf"} {
		if _, err := transport.FetchFont(context.Background(), name); err == nil {
			t.Fatal("oversized response accepted", name)
		}
	}
}

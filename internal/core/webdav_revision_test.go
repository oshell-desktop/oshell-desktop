package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestNutstoreCoordinatedWrites(t *testing.T) {
	ctx := context.Background()
	w, err := NewWebDAVTransport("https://dav.jianguoyun.com/dav/oshell/", "test", "test", false)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	const vault = "/dav/oshell/oshell-vault.json"
	const marker = vault + ".oshell-lock"
	const first = `{"format":"OShell.shared-vault","v":1}`
	const second = `{"format":"OShell.shared-vault","v":2}`
	failWrite := false
	w.client.Transport = updateRoundTrip(func(r *http.Request) (*http.Response, error) {
		out := httptest.NewRecorder()
		path := r.URL.Path
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}
		switch r.Method {
		case "PROPFIND":
			writeDAVCollection(out, path)
		case "GET":
			if text, ok := files[path]; ok {
				out.Header().Set("ETag", "bare-revision")
				fmt.Fprint(out, text)
			} else {
				out.WriteHeader(404)
			}
		case "PUT":
			if strings.Contains(path, "/oshell-stage-") {
				if r.Header.Get("If-None-Match") != "*" {
					t.Error("missing staged create condition")
				}
				files[path] = string(body)
				out.WriteHeader(201)
			} else {
				if _, ok := files[path+".oshell-lock"]; !ok || r.Header.Get("If-Match") != "bare-revision" {
					t.Error("uncoordinated write")
				}
				if failWrite {
					out.WriteHeader(500)
				} else {
					files[path] = string(body)
					out.WriteHeader(204)
				}
			}
		case "MOVE":
			dst, err := url.Parse(r.Header.Get("Destination"))
			if err != nil {
				return nil, err
			}
			if r.Header.Get("Overwrite") != "F" || dst.Host != r.URL.Host {
				t.Error("unsafe publication")
			}
			if _, ok := files[dst.Path]; ok {
				out.WriteHeader(412)
			} else {
				files[dst.Path] = files[path]
				delete(files, path)
				out.WriteHeader(201)
			}
		case "DELETE":
			delete(files, path)
			out.WriteHeader(204)
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
		return out.Result(), nil
	})
	requireKind := func(err error, kind string) {
		t.Helper()
		var f *SyncFailure
		if !errors.As(err, &f) || f.Kind != kind {
			t.Fatalf("want %s, got %v", kind, err)
		}
	}
	if err = w.Put(ctx, []byte(first), ""); err != nil {
		t.Fatal(err)
	}
	requireKind(w.Put(ctx, []byte(second), ""), "conflict")
	obj, err := w.Fetch(ctx)
	if err != nil || obj == nil {
		t.Fatal(err)
	}
	if status, err := w.ProbeStatus(ctx); err != nil || !strings.Contains(status, "坚果云") {
		t.Fatal(status, err)
	}
	files[marker] = "another OShell writer"
	requireKind(w.Put(ctx, []byte(second), obj.ETag), "occupied")
	if files[marker] != "another OShell writer" || files[vault] != first {
		t.Fatal("foreign marker or original object changed")
	}
	delete(files, marker)
	files[vault] = second
	requireKind(w.Put(ctx, []byte(first), obj.ETag), "conflict")
	if _, ok := files[marker]; ok {
		t.Fatal("known conflict retained marker")
	}
	files[vault] = first
	if err = w.Put(ctx, []byte(second), obj.ETag); err != nil || files[vault] != second {
		t.Fatal(err)
	}
	if err = w.PutFont(ctx, "library.json", []byte("library 1"), ""); err != nil {
		t.Fatal(err)
	}
	font, err := w.FetchFont(ctx, "library.json")
	if err != nil || font == nil {
		t.Fatal(err)
	}
	if err = w.PutFont(ctx, "library.json", []byte("library 2"), font.ETag); err != nil || files["/dav/oshell/fonts/library.json"] != "library 2" {
		t.Fatal(err)
	}
	failWrite = true
	obj, err = w.Fetch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	requireKind(w.Put(ctx, []byte(first), obj.ETag), "uncertain-write")
	if _, ok := files[marker]; !ok {
		t.Fatal("unknown PUT result admitted another writer")
	}
}

func TestWebDAVPropertyTagBindsMergedBytes(t *testing.T) {
	ctx := context.Background()
	for _, change := range []bool{false, true} {
		t.Run(fmt.Sprint(change), func(t *testing.T) {
			body := `{"format":"OShell.shared-vault","v":1}`
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case "PROPFIND":
					if strings.HasSuffix(r.URL.Path, "/") {
						writeDAVCollection(w, r.URL.Path)
						return
					}
					w.WriteHeader(207)
					fmt.Fprintf(w, `<d:multistatus xmlns:d="DAV:"><d:response><d:href>%s</d:href><d:propstat><d:prop><d:getetag>"rev"</d:getetag></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response></d:multistatus>`, r.URL.Path)
				case "GET":
					if tag := r.Header.Get("If-Match"); tag != "" {
						if tag != `"rev"` {
							t.Error(tag)
						}
						if change {
							body = `{"format":"OShell.shared-vault","v":2}`
						}
					}
					fmt.Fprint(w, body)
				case "PUT":
					if r.Header.Get("If-Match") != `"rev"` {
						t.Error("wrong conditional version")
					}
					writes++
					w.WriteHeader(204)
				}
			}))
			defer server.Close()
			tr, err := NewWebDAVTransport(server.URL+"/oshell/", "", "", false)
			if err != nil {
				t.Fatal(err)
			}
			obj, err := tr.Fetch(ctx)
			if err != nil || obj == nil {
				t.Fatal(err)
			}
			err = tr.Put(ctx, obj.Data, obj.ETag)
			if change && (err == nil || writes != 0) || !change && (err != nil || writes != 1) {
				t.Fatalf("writes=%d err=%v", writes, err)
			}
		})
	}
}

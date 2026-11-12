package core

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebDAVProbeExistingCollectionAndFailures(t *testing.T) {
	for _, scenario := range []string{"empty", "authentication", "redirect", "weak-etag"} {
		t.Run(scenario, func(t *testing.T) {
			requests := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				user, pass, ok := r.BasicAuth()
				if !ok || user != "fixture-user" || pass != "fixture-secret" {
					t.Error("credentials missing")
				}
				if r.Method != "PROPFIND" && r.Method != "GET" {
					t.Error("probe wrote to server")
				}
				if scenario == "authentication" {
					w.WriteHeader(401)
					return
				}
				if scenario == "redirect" {
					w.Header().Set("Location", "/other/")
					w.WriteHeader(302)
					return
				}
				if r.Method == "PROPFIND" {
					if scenario == "weak-etag" && r.URL.Path == "/dav/oshell-vault.json" {
						w.WriteHeader(405)
						return
					}
					if r.URL.Path != "/dav/oshell/" || r.Header.Get("Depth") != "0" {
						t.Error("wrong directory probe")
					}
					writeDAVCollection(w, r.URL.Path)
					return
				}
				if r.URL.Path != "/dav/oshell-vault.json" && r.URL.Path != "/dav/oshell/oshell-vault.json" {
					t.Error("wrong file")
				}
				if scenario == "weak-etag" {
					w.Header().Set("ETag", `W/"weak"`)
					w.Write([]byte(`{"format":"OShell.shared-vault"}`))
					return
				}
				w.WriteHeader(404)
			}))
			defer server.Close()
			transport, err := NewWebDAVTransport(server.URL+"/dav", "fixture-user", "fixture-secret", false)
			if err != nil {
				t.Fatal(err)
			}
			transport.client.Transport = server.Client().Transport
			status, err := transport.ProbeStatus(context.Background())
			if scenario == "weak-etag" {
				if err != nil || !strings.Contains(status, "只读") {
					t.Fatal(status, err)
				}
				return
			}
			if scenario == "empty" {
				if err != nil || requests != 3 {
					t.Fatal(err, requests)
				}
				return
			}
			var failure *SyncFailure
			if !errors.As(err, &failure) {
				t.Fatal(err)
			}
			want := scenario
			if failure.Kind != want {
				t.Fatal(failure.Kind)
			}
			if scenario == "redirect" && requests != 1 {
				t.Fatal("followed redirect")
			}
		})
	}
}

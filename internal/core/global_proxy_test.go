package core

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/pkg/sftp"
)

func TestGlobalProxyLocalPolicyAndSharedIsolation(t *testing.T) {
	dir := t.TempDir()
	s, err := LoadGlobalProxy(dir)
	if err != nil || s.Enabled || s.Version != 1 {
		t.Fatal(s, err)
	}
	cfg := DefaultConfig()
	first, last := NewProxyProfile(), NewProxyProfile()
	first.Settings.Host, last.Settings.Host = "first.example", "last.example"
	last.UpstreamID = first.ID()
	cfg.Proxies = []ProxyProfile{first, last}
	p := NewProfile()
	p.Host = "target.example"
	p.ProxyID = last.ID()
	cfg.Profiles = []Profile{p}
	before, _ := json.Marshal(cfg)
	s.Enabled, s.ProxyID = true, last.ID()
	if err := s.Save(dir); err != nil {
		t.Fatal(err)
	}
	s, err = LoadGlobalProxy(dir)
	if err != nil {
		t.Fatal(err)
	}
	routed, err := s.ConnectionProfile(cfg, p, nil)
	if err != nil || len(routed.RuntimeProxyRoute) != 2 {
		t.Fatal(routed, err)
	}
	runtimeJSON, _ := json.Marshal(routed)
	storedJSON, _ := json.Marshal(p)
	if !bytes.Equal(runtimeJSON, storedJSON) {
		t.Fatal("runtime route leaked into shared profile")
	}
	r := ResolvedSSH{Profile: routed}
	r.Profile.ProxyID = "" // A session without its own proxy still suppresses ssh_config routes.
	applyResolvedSSH(&r, "hostname resolved.example\nproxyjump other.example\nproxycommand invalid-command\n")
	if r.ProxyCommand != "" || r.Profile.JumpHost != "" || r.Profile.Host != "resolved.example" {
		t.Fatal("system SSH route overrode runtime route")
	}
	s.Enabled = false
	restored, err := s.ConnectionProfile(cfg, routed, nil)
	if err != nil || len(restored.RuntimeProxyRoute) != 2 || restored.RuntimeProxyRoute[0].ID != first.ID() {
		t.Fatal("did not restore session route", err)
	}
	for _, kind := range []string{"ftp", "local", "blank"} {
		p.Kind, p.ProxyID = kind, ""
		s.Enabled, s.ProxyID = true, ""
		got, err := s.ConnectionProfile(cfg, p, nil)
		if err != nil || !reflect.DeepEqual(got, p) {
			t.Fatal("affected excluded kind", kind, err)
		}
	}
	after, _ := json.Marshal(cfg)
	if !bytes.Equal(before, after) {
		t.Fatal("shared configuration changed")
	}
}

func TestGlobalProxyInvalidPolicyFailsClosed(t *testing.T) {
	dir := t.TempDir()
	for _, data := range []string{`null`, `{}`, `{"version":2,"enabled":false}`, `{"version":1,"enabled":null}`, `{"version":1,"enabled":true,"proxyID":"bad"}`, `{"version":1,"enabled":false,"unknown":1}`, `{"version":1,"enabled":false} {}`} {
		if err := os.WriteFile(filepath.Join(dir, "global-proxy.json"), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadGlobalProxy(dir); err == nil {
			t.Fatalf("accepted corrupt settings %s", data)
		}
	}
	cfg := DefaultConfig()
	p := NewProfile()
	p.Host = "target.example"
	s := GlobalProxySettings{Version: 1, Enabled: true}
	if _, err := s.ConnectionProfile(cfg, p, nil); err == nil {
		t.Fatal("empty selection fell back")
	}
	s.ProxyID = NewID()
	if _, err := s.ConnectionProfile(cfg, p, nil); err == nil {
		t.Fatal("deleted selection fell back")
	}
	hop := NewProxyProfile()
	hop.Settings.Host = "proxy.example"
	hop.UpstreamID = hop.ID()
	cfg.Proxies = []ProxyProfile{hop}
	s.ProxyID = hop.ID()
	if _, err := s.ConnectionProfile(cfg, p, nil); err == nil {
		t.Fatal("cycle accepted")
	}
	cfg.Proxies[0].UpstreamID = ""
	cfg.Proxies[0].Settings.Kind = "none"
	if _, err := s.ConnectionProfile(cfg, p, nil); err == nil {
		t.Fatal("empty hop accepted")
	}
}

func TestGlobalProxyComposesMixedSessionRoutes(t *testing.T) {
	c := DefaultConfig()
	a, b := NewProxyProfile(), NewProxyProfile()
	a.Settings.Host, b.Settings.Host = "gateway.example", "edge.example"
	b.UpstreamID = a.ID()
	c.Proxies = []ProxyProfile{a, b}
	p := NewProfile()
	policy := GlobalProxySettings{Enabled: true, ProxyID: b.ID()}
	// A legacy hop with an explicit port requires no SSH config lookup.
	p.JumpHost = "alice@jump.example:2202"
	joined, err := policy.ConnectionProfile(c, p, nil)
	if err != nil || len(joined.RuntimeProxyRoute) != 3 {
		t.Fatal(joined.RuntimeProxyRoute, err)
	}
	hop := joined.RuntimeProxyRoute[2]
	if hop.Kind != "jump" || hop.Port != 2202 || hop.Host != "jump.example" || hop.Username != "alice" {
		t.Fatal(hop)
	}
	p.JumpHost = ""
	p.Proxy = a.Settings
	if _, err = policy.ConnectionProfile(c, p, nil); err == nil {
		t.Fatal("reordered repeat accepted")
	}
	p.Proxy = b.Settings
	p.Proxy.Port++
	if _, err = policy.ConnectionProfile(c, p, nil); err == nil {
		t.Fatal("conflicting ID accepted")
	}
	p.Proxy = b.Settings
	p.Proxy.ID = NewID()
	v, err := OpenVault(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	joined, err = policy.ConnectionProfile(c, p, v)
	if err != nil || len(joined.RuntimeProxyRoute) != 2 {
		t.Fatal("equivalent boundary hops", err)
	}
	if err = v.Put(ProxyCredentialProfile(p.Proxy), "synthetic"); err != nil {
		t.Fatal(err)
	}
	joined, err = policy.ConnectionProfile(c, p, v)
	if err != nil || len(joined.RuntimeProxyRoute) != 3 || joined.RuntimeProxyRoute[2].ID != p.Proxy.ID {
		t.Fatal("credentials lost", err)
	}
	p.Proxy = ProxySettings{}
	p.JumpHost = "u@a:22,u@b:22,u@c:22,u@d:22,u@e:22,u@f:22,u@g:22"
	if _, err = policy.ConnectionProfile(c, p, nil); err == nil {
		t.Fatal("accepted nine hops")
	}
}

func TestGlobalProxySSHFilesSCPAndRouteRetention(t *testing.T) {
	t.Setenv("OSHELL_TEST_IGNORE_SSH_CONFIG", "1")
	target, gateway := startSSH(t, "target"), startSSH(t, "gateway")
	p, hop := testProfile(target.address), testProfile(gateway.address)
	proxy := NewProxyProfile()
	proxy.Settings.Kind = "jump"
	proxy.Settings.Host = hop.Host
	proxy.Settings.Port = hop.Port
	proxy.Settings.Username = hop.Username
	proxy.Settings.SSHAuthentication = "password"
	cfg := DefaultConfig()
	cfg.Proxies = []ProxyProfile{proxy}
	s := GlobalProxySettings{Version: 1, Enabled: true, ProxyID: proxy.ID()}
	cred := Credentials{Password: "target", ProxyCredentials: func(context.Context, ProxySettings, SSHIdentity) (Credentials, error) {
		return Credentials{Password: "gateway"}, nil
	}}
	for _, enabled := range []bool{true, false} {
		for _, kind := range []string{"ssh", "sftp"} {
			t.Run(kind+map[bool]string{true: "-enabled", false: "-disabled"}[enabled], func(t *testing.T) {
				s.Enabled = enabled
				p.Kind = kind
				route, err := s.ConnectionProfile(cfg, p, nil)
				if err != nil {
					t.Fatal(err)
				}
				conn, err := Dial(context.Background(), route, cred, testTrust(t), acceptHost)
				if err != nil {
					t.Fatal(err)
				}
				defer conn.Close()
				_, port := conn.ObservedPeer()
				want := p.Port
				if enabled {
					want = hop.Port
				}
				if port != want {
					t.Fatal("wrong TCP route", port, want)
				}
				if kind == "ssh" {
					echoConn(t, conn, "global-proxy\n")
				}
				files, err := sftp.NewClient(conn.Client)
				if err != nil {
					t.Fatal(err)
				}
				defer files.Close()
				if _, err := files.ReadDir(filepath.ToSlash(t.TempDir())); err != nil {
					t.Fatal(err)
				}
				// Changing policy cannot reroute an authenticated transport or its clones.
				s.Enabled = !enabled
				clone, err := conn.CloneFiles(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				defer clone.Close()
				_, clonePort := clone.ObservedPeer()
				if clonePort != want || clone.Client != conn.Client {
					t.Fatal("clone rerouted")
				}
				if _, err := exec.LookPath("scp"); err != nil {
					t.Skip("OpenSSH scp missing: SCP pending")
				}
				source := filepath.Join(t.TempDir(), "payload.txt")
				payload := []byte("C08 SCP 合成数据\n")
				if err := os.WriteFile(source, payload, 0600); err != nil {
					t.Fatal(err)
				}
				remote := filepath.ToSlash(filepath.Join(t.TempDir(), "remote.txt"))
				dest := filepath.Join(t.TempDir(), "download.txt")
				if err := clone.SCPUpload(context.Background(), source, remote, nil); err != nil {
					t.Fatal(err)
				}
				if err := clone.SCPDownload(context.Background(), remote, dest, false, nil); err != nil {
					t.Fatal(err)
				}
				got, err := os.ReadFile(dest)
				if err != nil || !bytes.Equal(got, payload) {
					t.Fatal("SCP contents differ", err)
				}
			})
		}
	}
	// A failed selected proxy must never authenticate directly to the target.
	before := target.auth.Load()
	cfg.Proxies[0].Settings.Port = unusedPort(t)
	s.Enabled = true
	p.Kind = "ssh"
	route, err := s.ConnectionProfile(cfg, p, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := Dial(ctx, route, cred, testTrust(t), acceptHost)
	if conn != nil {
		conn.Close()
	}
	if err == nil || target.auth.Load() != before {
		t.Fatal("failed proxy fell back to direct")
	}
}

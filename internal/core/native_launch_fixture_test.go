package core

import (
	"encoding/json"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Opt-in loopback-only native acceptance fixture. Never uses a user's server or
// credentials. Normal test runs skip it; create STOP in its directory to end it.
func TestNativeLaunchFixture(t *testing.T) {
	dir := os.Getenv("OSHELL_NATIVE_LAUNCH_FIXTURE_DIR")
	if dir == "" {
		t.Skip("native acceptance fixture only")
	}
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	password := "fixture-only-password"
	if os.Getenv("OSHELL_NATIVE_SSO_TOOL") == "1" {
		password = "fixture-中文-é"
	}
	server := startSSH(t, password)
	line := knownhosts.Normalize(server.address) + " " + string(ssh.MarshalAuthorizedKey(server.hostKey))
	if e := os.WriteFile(filepath.Join(dir, "known_hosts"), []byte(line), 0600); e != nil {
		t.Fatal(e)
	}
	store := &Store{Dir: dir}
	cfg := DefaultConfig()
	checkKeepAlive := os.Getenv("OSHELL_NATIVE_LAUNCH_KEEPALIVE") == "1"
	if checkKeepAlive {
		cfg.SessionDefaults.KeepAlive.Enabled = false
		pref := DefaultKeepAlive()
		pref.Interval = 1
		if e := SaveBastionKeepAlive(dir, server.address, pref); e != nil {
			t.Fatal(e)
		}
	}
	if e := store.Save(cfg); e != nil {
		t.Fatal(e)
	}
	if os.Getenv("OSHELL_NATIVE_LAUNCH_LOCKED") == "1" {
		vault, e := OpenVault(dir)
		if e != nil {
			t.Fatal(e)
		}
		if e = vault.SetMaster("fixture-only-master"); e != nil {
			t.Fatal(e)
		}
	}
	ready, _ := json.Marshal(map[string]string{"address": server.address})
	if e := os.WriteFile(filepath.Join(dir, "fixture-ready.json"), ready, 0600); e != nil {
		t.Fatal(e)
	}
	timer := time.NewTicker(100 * time.Millisecond)
	defer timer.Stop()
	timeout := time.NewTimer(20 * time.Minute)
	defer timeout.Stop()
	terms := []string{}
	for {
		select {
		case term := <-server.terminalTypes:
			terms = append(terms, term)
			data, _ := json.Marshal(map[string]any{"terminalTypes": terms, "authentications": server.auth.Load()})
			if e := AtomicWrite(filepath.Join(dir, "fixture-observed.json"), data); e != nil {
				t.Fatal(e)
			}
		case <-timer.C:
			if checkKeepAlive && server.keepAliveRequests.Load() > 0 {
				if e := AtomicWrite(filepath.Join(dir, "fixture-keepalive.json"), []byte(`{"received":true}`)); e != nil {
					t.Fatal(e)
				}
				checkKeepAlive = false
			}
			if _, e := os.Stat(filepath.Join(dir, "STOP")); e == nil {
				return
			}
		case <-timeout.C:
			return
		}
	}
}

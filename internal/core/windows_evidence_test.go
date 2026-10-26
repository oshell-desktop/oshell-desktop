package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

type evidenceFailingListener struct {
	net.Listener
	allow atomic.Bool
}

func (l *evidenceFailingListener) Close() error {
	if !l.allow.Load() {
		return errors.New("synthetic listener close failure")
	}
	return l.Listener.Close()
}

func TestEvidenceC10CloseFailureKeepsOwnership(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	wrapped := &evidenceFailingListener{Listener: listener}
	ctx, cancel := context.WithCancel(context.Background())
	owner := &transport{done: make(chan struct{}), cancel: cancel, closers: map[io.Closer]bool{}}
	id := NewID()
	if !owner.claimTunnels(id) {
		t.Fatal("initial claim")
	}
	contender := &transport{}
	defer func() {
		wrapped.allow.Store(true)
		listener.Close()
		owner.releaseTunnels()
		contender.releaseTunnels()
		cancel()
	}()
	owner.track(wrapped)
	finished := make(chan struct{})
	go func() { owner.acceptTunnel(ctx, wrapped, NewTunnel()); close(finished) }()
	owner.shutdown()
	if contender.claimTunnels(strings.ToLower(id)) {
		t.Fatal("failed Close released ownership while listener still owns port")
	}
	wrapped.allow.Store(true)
	listener.Close()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("listener did not exit")
	}
	if !contender.claimTunnels(id) {
		t.Fatal("actually released listener did not clear ownership")
	}
}

func TestEvidenceC10PendingAuthenticationAndReopen(t *testing.T) {
	server := startSSH(t, "fixture")
	p := testProfile(server.address)
	dest := testProfile(startEcho(t).Addr().String())
	rule := NewTunnel()
	rule.ListenPort = unusedPort(t)
	rule.DestinationHost = dest.Host
	rule.DestinationPort = dest.Port
	p.Tunnels = []TunnelRule{rule}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	entered, release := make(chan struct{}), make(chan struct{})
	var released atomic.Bool
	defer func() {
		if released.CompareAndSwap(false, true) {
			close(release)
		}
	}()
	firstReady := make(chan *Connection, 1)
	errorsCh := make(chan error, 1)
	go func() {
		conn, err := Dial(ctx, p, Credentials{Password: "fixture"}, testTrust(t), func(HostPrompt) bool {
			close(entered)
			select {
			case <-release:
				return true
			case <-ctx.Done():
				return false
			}
		})
		firstReady <- conn
		errorsCh <- err
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("first authentication not pending")
	}
	second, err := Dial(ctx, p, Credentials{Password: "fixture"}, testTrust(t), acceptHost)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if second.TunnelPolicy() != "skipped" {
		t.Fatal("pending owner was not reserved")
	}
	released.Store(true)
	close(release)
	first := <-firstReady
	if err = <-errorsCh; err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if first.TunnelPolicy() != "owned" {
		t.Fatal("first did not bind")
	}
	probe, err := net.Dial("tcp", net.JoinHostPort(rule.BindHost, fmt.Sprint(rule.ListenPort)))
	if err != nil {
		t.Fatal(err)
	}
	probe.SetDeadline(time.Now().Add(time.Second))
	probe.Write([]byte("tunnel"))
	got := make([]byte, 6)
	_, err = io.ReadFull(probe, got)
	probe.Close()
	if err != nil || string(got) != "tunnel" {
		t.Fatal("listener traffic", err)
	}
	first.Close()
	third, err := Dial(ctx, p, Credentials{Password: "fixture"}, testTrust(t), acceptHost)
	if err != nil {
		t.Fatal(err)
	}
	defer third.Close()
	if third.TunnelPolicy() != "owned" {
		t.Fatal("released owner not reacquired")
	}
}

func TestEvidenceC02RealHandshakeTrustLifecycle(t *testing.T) {
	var cfg *ssh.ServerConfig
	server := startSSH(t, "fixture", func(c *ssh.ServerConfig) { cfg = c })
	profile := testProfile(server.address)
	trust := testTrust(t)
	approvals := 0
	dial := func(ctx context.Context, approve func(HostPrompt) bool) error {
		conn, err := Dial(ctx, profile, Credentials{Password: "fixture"}, trust, approve)
		if conn != nil {
			conn.Close()
		}
		return err
	}
	approve := func(HostPrompt) bool { approvals++; return true }
	if err := dial(context.Background(), approve); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(trust.Path)
	if err != nil || len(first) == 0 || approvals != 1 {
		t.Fatal("first trust", err)
	}
	if err := dial(context.Background(), approve); err != nil || approvals != 1 {
		t.Fatal("same key reprompted", err)
	}
	cfg.AddHostKey(key(t))
	if err := dial(context.Background(), approve); err == nil || !strings.Contains(err.Error(), "主机密钥不匹配") || approvals != 1 {
		t.Fatal("changed key accepted or reprompted", err)
	}
	after, _ := os.ReadFile(trust.Path)
	if !bytes.Equal(first, after) {
		t.Fatal("changed key overwrote trust")
	}
	trust = testTrust(t)
	if err := dial(context.Background(), func(HostPrompt) bool { return false }); err == nil {
		t.Fatal("rejection accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := dial(ctx, func(HostPrompt) bool { cancel(); return true }); err == nil {
		t.Fatal("cancelled approval accepted")
	}
	after, _ = os.ReadFile(trust.Path)
	if len(after) != 0 {
		t.Fatal("rejection/cancellation saved key")
	}
}

func TestEvidenceM07ImageStorageBoundaries(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	data := make([]byte, ClipboardImageLimit)
	file, err := SaveClipboardImage(root, data, now)
	if err != nil {
		t.Fatal("20 MiB boundary", err)
	}
	if info, err := os.Stat(file); err != nil || info.Size() != ClipboardImageLimit {
		t.Fatal("boundary file", err)
	}
	if _, err := SaveClipboardImage(root, append(data, 1), now); err == nil {
		t.Fatal("20 MiB+1 accepted")
	}
	if _, err := ReadClipboardImageFile(file); err != nil {
		t.Fatal("file boundary", err)
	}
	extra := filepath.Join(root, "too-large.png")
	f, err := os.Create(extra)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Truncate(ClipboardImageLimit + 1); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if _, err := ReadClipboardImageFile(extra); err == nil {
		t.Fatal("oversize file accepted")
	}
	cache := filepath.Dir(file)
	filler := filepath.Join(cache, "oshell-clipboard-boundary.png")
	f, err = os.Create(filler)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Truncate(180<<20 - 1); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if _, err := SaveClipboardImage(root, []byte{1}, now); err != nil {
		t.Fatal("200 MiB boundary", err)
	}
	entries, _ := os.ReadDir(cache)
	if _, err := SaveClipboardImage(root, []byte{2}, now); err == nil {
		t.Fatal("200 MiB+1 accepted")
	}
	next, _ := os.ReadDir(cache)
	if len(next) != len(entries) {
		t.Fatal("failed save created cache file")
	}
	keep := filepath.Join(cache, "unrelated.png")
	if err = os.WriteFile(keep, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	old := now.Add(-8 * 24 * time.Hour)
	os.Chtimes(keep, old, old)
	os.Chtimes(filler, old, old)
	if _, err := SaveClipboardImage(root, []byte{3}, now); err != nil {
		t.Fatal("expired own cache not reclaimed", err)
	}
	if got, _ := os.ReadFile(keep); string(got) != "keep" {
		t.Fatal("unrelated data changed")
	}
}

func TestEvidenceM07DecodeAndOutputBoundaries(t *testing.T) {
	encode := func(im image.Image) []byte {
		var b bytes.Buffer
		if err := png.Encode(&b, im); err != nil {
			t.Fatal(err)
		}
		return b.Bytes()
	}
	small := encode(image.NewGray(image.Rect(0, 0, 1, 1)))
	input := make([]byte, ClipboardImageInputLimit+1)
	copy(input, small)
	if _, err := NormalizeClipboardImage(input[:ClipboardImageInputLimit]); err != nil {
		t.Fatal("160 MiB valid PNG input boundary", err)
	}
	if _, err := NormalizeClipboardImage(input); err == nil {
		t.Fatal("160 MiB+1 input accepted")
	}
	for _, tc := range []struct {
		w, h int
		ok   bool
	}{{20000, 1, true}, {1, 20000, true}, {20001, 1, false}, {1, 20001, false}, {10000, 4000, true}, {10000, 4001, false}} {
		data := encode(image.NewGray(image.Rect(0, 0, tc.w, tc.h)))
		_, err := NormalizeClipboardImage(data)
		if (err == nil) != tc.ok {
			t.Fatalf("dimension boundary %dx%d: %v", tc.w, tc.h, err)
		}
	}
	for _, width := range []int{3200, 6400} {
		im := image.NewNRGBA(image.Rect(0, 0, width, 1000))
		var state uint32 = 0x12345678
		for i := range im.Pix {
			state ^= state << 13
			state ^= state >> 17
			state ^= state << 5
			im.Pix[i] = byte(state)
		}
		encoded := encode(im)
		_, err := NormalizeClipboardImage(encoded)
		if width == 3200 && (err != nil || len(encoded) >= ClipboardImageLimit) {
			t.Fatal("under-limit encoded output rejected", err)
		}
		if width == 6400 && (err == nil || len(encoded) <= ClipboardImageLimit) {
			t.Fatal("over-limit encoded PNG output accepted", err)
		}
	}
}

func TestEvidenceE05MissingPasswordMatrix(t *testing.T) {
	cipher := XshellPassword{encoded: "Rrm3P3AL0iDV7nBbS2bHvh7ZAvuN1NSJl8ZFL11+UJ+82+KAixa89O3OTAfRTg=="}
	for _, kind := range []string{"unique", "none", "ambiguous", "duplicate-source", "existing", "directory", "decrypt-failure"} {
		t.Run(kind, func(t *testing.T) {
			cfg := DefaultConfig()
			p := NewProfile()
			p.Name = "target"
			p.Host = "fixture.invalid"
			p.Username = "fixture"
			p.Group = "one"
			cfg.Profiles = []Profile{p}
			source := p
			source.ID = NewID()
			source.Group = "source"
			r := ThirdPartyReport{Profiles: []Profile{source}, Passwords: map[string]XshellPassword{source.ID: cipher}}
			v, err := OpenVault(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			directory := ""
			want, unmatched, preserved := 1, 0, 0
			wantErr := false
			switch kind {
			case "none":
				r.Profiles[0].Host = "other.invalid"
				want = 0
				unmatched = 1
			case "ambiguous", "directory":
				q := p
				q.ID = NewID()
				q.Group = "two"
				cfg.Profiles = append(cfg.Profiles, q)
				if kind == "ambiguous" {
					wantErr = true
				} else {
					directory = "one"
				}
			case "duplicate-source":
				q := source
				q.ID = NewID()
				r.Profiles = append(r.Profiles, q)
				r.Passwords[q.ID] = cipher
				wantErr = true
			case "existing":
				if err = v.PutFor(p, "existing-synthetic", p.Identity()); err != nil {
					t.Fatal(err)
				}
				want = 0
				preserved = 1
			case "decrypt-failure":
				q := p
				q.ID = NewID()
				q.Name = "second"
				cfg.Profiles = append(cfg.Profiles, q)
				q.ID = NewID()
				r.Profiles = append(r.Profiles, q)
				r.Passwords[q.ID] = XshellPassword{encoded: "invalid"}
				wantErr = true
			}
			before := cfg.Clone()
			revision := v.Revision()
			next, updates, stats, err := r.Merge(context.Background(), cfg, v, directory, true, true, "123123")
			if (err != nil) != wantErr {
				t.Fatal("error contract", err)
			}
			if !reflect.DeepEqual(next, before) || !reflect.DeepEqual(cfg, before) || v.Revision() != revision {
				t.Fatal("merge changed original identities, properties or vault")
			}
			if wantErr {
				if len(updates) != 0 {
					t.Fatal("partial updates escaped failure")
				}
				return
			}
			if len(updates) != want || stats.Unmatched != unmatched || stats.Preserved != preserved {
				t.Fatal("matching counts", stats)
			}
			if want == 1 && (updates[0].Profile.ID != p.ID || updates[0].Password != "This is a test" || updates[0].Identity != p.Identity()) {
				t.Fatal("target binding")
			}
		})
	}
}

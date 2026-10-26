package core

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"
)

func TestProxyCatalogCyclesDepthAndPortableReferences(t *testing.T) {
	var proxies []ProxyProfile
	for i := 0; i < 8; i++ {
		p := NewProxyProfile()
		p.Settings.Host = "127.0.0.1"
		if i > 0 {
			p.UpstreamID = proxies[i-1].ID()
		}
		proxies = append(proxies, p)
	}
	route, e := ProxyRoute(proxies[7].ID(), proxies)
	if e != nil || len(route) != 8 || route[0].ID() != proxies[0].ID() {
		t.Fatal(route, e)
	}
	ninth := NewProxyProfile()
	ninth.Settings.Host = "127.0.0.1"
	ninth.UpstreamID = proxies[7].ID()
	if _, e = ProxyRoute(ninth.ID(), append(proxies, ninth)); e == nil {
		t.Fatal("ninth hop accepted")
	}
	cyclic := append([]ProxyProfile(nil), proxies...)
	cyclic[0].UpstreamID = cyclic[7].ID()
	if _, e = ProxyRoute(cyclic[0].ID(), cyclic); e == nil {
		t.Fatal("cycle accepted")
	}
	p := NewProfile()
	p.Host = "target.example"
	p.Username = "ops"
	p.ProxyID = proxies[7].ID()
	data, e := json.Marshal(p)
	if e != nil || !strings.Contains(string(data), `"shared-reference"`) {
		t.Fatal(string(data), e)
	}
	cfg := DefaultConfig()
	cfg.Profiles = []Profile{p}
	cfg.Proxies = proxies
	archive, e := ExportArchive(cfg)
	if e != nil {
		t.Fatal(e)
	}
	decoded, e := DecodePortableArchive(archive)
	if e != nil || decoded.Version != 4 || len(decoded.Proxies) != 8 {
		t.Fatal(e)
	}
	merged, _, e := decoded.Merge(context.Background(), DefaultConfig(), "", false, nil)
	if e != nil {
		t.Fatal(e)
	}
	newRoute, e := ProxyRoute(merged.Profiles[0].ProxyID, merged.Proxies)
	if e != nil || len(newRoute) != 8 || newRoute[0].ID() == proxies[0].ID() {
		t.Fatal("proxy IDs not rebound", e)
	}
	if _, e = Dial(context.Background(), p, Credentials{}, testTrust(t), acceptHost); e == nil {
		t.Fatal("unresolved proxy silently connected")
	}
}
func TestMixedSSHAndSOCKSRoute(t *testing.T) {
	target := startSSH(t, "target-password")
	gateway := startSSH(t, "gateway-password")
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer listener.Close()
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		socket, e := listener.Accept()
		if e != nil {
			return
		}
		address, version, e := readDynamicRequest(socket)
		if e != nil {
			socket.Close()
			return
		}
		replyDynamic(socket, version, true)
		out, e := net.Dial("tcp", address)
		if e != nil {
			socket.Close()
			return
		}
		relayTest(socket, out)
	}()
	gatewayProfile := testProfile(gateway.address)
	socksProfile := testProfile(listener.Addr().String())
	first := NewProxyProfile()
	first.Settings.Kind = "jump"
	first.Settings.Host = gatewayProfile.Host
	first.Settings.Port = gatewayProfile.Port
	first.Settings.Username = gatewayProfile.Username
	first.Settings.SSHAuthentication = "password"
	second := NewProxyProfile()
	second.Settings.Host = socksProfile.Host
	second.Settings.Port = socksProfile.Port
	second.UpstreamID = first.ID()
	p := testProfile(target.address)
	p.ProxyID = second.ID()
	cfg := DefaultConfig()
	cfg.Proxies = []ProxyProfile{first, second}
	p, e = cfg.ConnectionProfile(p)
	if e != nil {
		t.Fatal(e)
	}
	called := map[string]bool{}
	c, e := Dial(context.Background(), p, Credentials{Password: "target-password", ProxyCredentials: func(_ context.Context, settings ProxySettings, _ SSHIdentity) (Credentials, error) {
		called[settings.ID] = true
		if settings.ID == first.ID() {
			return Credentials{Password: "gateway-password"}, nil
		}
		return Credentials{}, nil
	}}, testTrust(t), acceptHost)
	if e != nil {
		t.Fatal(e)
	}
	host, port := c.ObservedPeer()
	if host != gatewayProfile.Host || port != gatewayProfile.Port {
		t.Fatalf("observed peer must be first local TCP hop: %s:%d", host, port)
	}
	echoConn(t, c, "mixed-route\n")
	c.Close()
	if !called[first.ID()] || !called[second.ID()] || target.auth.Load() != 1 || gateway.auth.Load() != 1 {
		t.Fatal("hop authentication not isolated")
	}
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("proxy route did not close")
	}
}
func TestTunnelsFollowTransportAcrossClonedChannels(t *testing.T) {
	server := startSSH(t, "test")
	echo := startEcho(t)
	p := testProfile(server.address)
	destination := testProfile(echo.Addr().String())
	tunnel := NewTunnel()
	tunnel.ListenPort = unusedPort(t)
	tunnel.DestinationHost = destination.Host
	tunnel.DestinationPort = destination.Port
	p.Tunnels = []TunnelRule{tunnel}
	first, e := Dial(context.Background(), p, Credentials{Password: "test"}, testTrust(t), acceptHost)
	if e != nil {
		t.Fatal(e)
	}
	defer first.Close()
	clone, e := first.Clone(context.Background(), p.KeepAlive)
	if e != nil {
		t.Fatal(e)
	}
	defer clone.Close()
	second, e := Dial(context.Background(), p, Credentials{Password: "test"}, testTrust(t), acceptHost)
	if e != nil {
		t.Fatal(e)
	}
	defer second.Close()
	if first.TunnelPolicy() != "owned" || clone.TunnelPolicy() != "owned" || second.TunnelPolicy() != "skipped" {
		t.Fatal("incorrect exposed tunnel ownership policy")
	}
	if second.transport.tunnelProfile != "" {
		t.Fatal("duplicate transport claimed tunnels")
	}
	first.Close()
	if clone.transport.tunnelProfile == "" {
		t.Fatal("original pane close released clone ownership")
	}
	clone.Close()
	third, e := Dial(context.Background(), p, Credentials{Password: "test"}, testTrust(t), acceptHost)
	if e != nil {
		t.Fatal(e)
	}
	defer third.Close()
	if third.transport.tunnelProfile == "" || !p.Tunnels[0].Enabled {
		t.Fatal("tunnels not reacquired or profile mutated")
	}
}

func TestProxyAndTargetInteractiveAuthenticationRemainIsolated(t *testing.T) {
	for _, mode := range []string{"success", "cancel-proxy", "cancel-target"} {
		t.Run(mode, func(t *testing.T) {
			target := startSSH(t, "target-password")
			gateway := startSSH(t, "gateway-password")
			hop := testProfile(gateway.address)
			proxy := NewProxyProfile()
			proxy.Settings.Kind = "jump"
			proxy.Settings.Host = hop.Host
			proxy.Settings.Port = hop.Port
			proxy.Settings.Username = hop.Username
			proxy.Settings.SSHAuthentication = "password"
			profile := testProfile(target.address)
			profile.ProxyID = proxy.ID()
			cfg := DefaultConfig()
			cfg.Proxies = []ProxyProfile{proxy}
			profile, err := cfg.ConnectionProfile(profile)
			if err != nil {
				t.Fatal(err)
			}
			proxyPrompts, targetPrompts := 0, 0
			published := ""
			conn, err := Dial(context.Background(), profile, Credentials{
				Challenge: func(_ context.Context, request AuthenticationPrompt) ([]string, error) {
					proxyPrompts++
					if request.Identity != hop.Identity() || request.Title != "SSH 跳板机认证" {
						t.Error("proxy challenge lost its identity/title")
					}
					if mode == "cancel-proxy" {
						return nil, context.Canceled
					}
					return []string{"gateway-password"}, nil
				},
				PasswordChallenge: func(_ context.Context, request AuthenticationPrompt) (string, error) {
					targetPrompts++
					if request.Identity != profile.Identity() || request.Title != "" {
						t.Error("target callback used for another endpoint")
					}
					if mode == "cancel-target" {
						return "", context.Canceled
					}
					return "target-password", nil
				},
				PasswordAuthenticated: func(password string) { published = password },
			}, testTrust(t), acceptHost)
			if conn != nil {
				defer conn.Close()
			}
			if mode == "success" {
				if err != nil {
					t.Fatal(err)
				}
				echoConn(t, conn, "isolated-route\n")
				if proxyPrompts != 1 || targetPrompts != 1 || published != "target-password" || !conn.PasswordVerified {
					t.Fatal("proxy credentials entered target save flow")
				}
			} else {
				if err == nil || published != "" {
					t.Fatal("cancel allowed connection/password publication")
				}
				if mode == "cancel-proxy" && (targetPrompts != 0 || target.auth.Load() != 0) {
					t.Fatal("target authentication ran after proxy cancellation")
				}
			}
		})
	}
}

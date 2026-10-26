package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

type jumpFixtureConfig struct {
	Host, User, Jump, Command, Identity string
	Port                                int
}

// The actual test executable supplies synthetic ssh -G and byte-stream proxy
// helpers. No user's SSH config, executable or credentials are modified.
func init() {
	if os.Getenv("OSHELL_TEST_JUMP_HELPER") != "1" {
		return
	}
	name := strings.TrimSuffix(strings.ToLower(filepath.Base(os.Args[0])), ".exe")
	if name == "ssh" {
		var configs map[string]jumpFixtureConfig
		if json.Unmarshal([]byte(os.Getenv("OSHELL_TEST_JUMP_CONFIG")), &configs) != nil {
			os.Exit(91)
		}
		cfg, ok := configs[os.Args[len(os.Args)-1]]
		if !ok {
			os.Exit(92)
		}
		for i := 1; i+1 < len(os.Args); i++ {
			if os.Args[i] == "-p" {
				cfg.Port, _ = strconv.Atoi(os.Args[i+1])
			}
			if os.Args[i] == "-l" {
				cfg.User = os.Args[i+1]
			}
		}
		fmt.Printf("hostname %s\nuser %s\nport %d\n", cfg.Host, cfg.User, cfg.Port)
		if cfg.Identity != "" {
			fmt.Println("identityfile", cfg.Identity)
		}
		if cfg.Jump != "" {
			fmt.Println("proxyjump", cfg.Jump)
		}
		if cfg.Command != "" {
			fmt.Println("proxycommand", cfg.Command)
		}
		os.Exit(0)
	}
	if name == "proxy-peer" {
		if len(os.Args) != 3 {
			os.Exit(93)
		}
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(os.Args[1], os.Args[2]), 5*time.Second)
		if err != nil {
			os.Exit(94)
		}
		go func() {
			io.Copy(conn, os.Stdin)
			if tcp, ok := conn.(*net.TCPConn); ok {
				tcp.CloseWrite()
			}
		}()
		io.Copy(os.Stdout, conn)
		conn.Close()
		os.Exit(0)
	}
}

func installJumpFixtureHelper(t *testing.T, dir, name string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	input, err := os.Open(exe)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	file := filepath.Join(dir, name)
	output, err := os.OpenFile(file, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if err != nil {
		t.Fatal(err)
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil || closeErr != nil {
		t.Fatal(copyErr, closeErr)
	}
	return file
}

func TestSSHConfiguredNestedRouteRoundtrip(t *testing.T) {
	for _, mode := range []string{"nested", "explicit", "proxy-command"} {
		t.Run(mode, func(t *testing.T) {
			target, inner, outer := startSSH(t, "target-password"), startSSH(t, "inner-password"), startSSH(t, "outer-password")
			dir := filepath.Join(t.TempDir(), "helper files")
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			installJumpFixtureHelper(t, dir, "ssh")
			proxy := installJumpFixtureHelper(t, dir, "proxy-peer")
			p, i, o := testProfile(target.address), testProfile(inner.address), testProfile(outer.address)
			configs := map[string]jumpFixtureConfig{
				"target-alias": {Host: p.Host, User: "target-user", Port: p.Port, Jump: "inner-alias"},
				"inner-alias":  {Host: i.Host, User: "inner-user", Port: i.Port, Jump: "outer-alias"},
				"outer-alias":  {Host: o.Host, User: "outer-user", Port: o.Port},
			}
			p.Host, p.Username = "target-alias", "target-user"
			if mode == "explicit" {
				p.JumpHost = "outer-alias,inner-alias"
				cfg := configs["inner-alias"]
				cfg.Jump = "must-not-resolve"
				configs["inner-alias"] = cfg
			}
			if mode == "proxy-command" {
				cfg := configs["outer-alias"]
				cfg.Command = `"` + proxy + `" %h %p`
				configs["outer-alias"] = cfg
			}
			raw, _ := json.Marshal(configs)
			t.Setenv("OSHELL_TEST_JUMP_CONFIG", string(raw))
			t.Setenv("OSHELL_TEST_JUMP_HELPER", "1")
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			credentials := Credentials{Password: "target-password", Challenge: func(_ context.Context, prompt AuthenticationPrompt) ([]string, error) {
				if prompt.Identity.Port == i.Port && prompt.Identity.User == "inner-user" {
					return []string{"inner-password"}, nil
				}
				if prompt.Identity.Port == o.Port && prompt.Identity.User == "outer-user" {
					return []string{"outer-password"}, nil
				}
				return nil, fmt.Errorf("unexpected authentication identity")
			}}
			conn, err := Dial(ctx, p, credentials, testTrust(t), acceptHost)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			echoConn(t, conn, "nested jump roundtrip\n")
			if target.auth.Load() != 1 || inner.auth.Load() != 1 || outer.auth.Load() != 1 {
				t.Fatal("route bypassed a hop or repeated authentication")
			}
			if len(conn.transport.clients) != 3 {
				t.Fatal("missing owned SSH transports")
			}
		})
	}
}

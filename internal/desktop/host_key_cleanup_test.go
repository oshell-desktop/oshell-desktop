package desktop

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
	"oshell-desktop/internal/core"
)

func TestHostKeyCleanupFailurePageAndConfirmation(t *testing.T) {
	for _, files := range []bool{false, true} {
		for _, reconnect := range []bool{false, true} {
			t.Run(fmt.Sprintf("files=%t/reconnect=%t", files, reconnect), func(t *testing.T) {
				a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
				newKey := func() ssh.PublicKey {
					_, private, err := ed25519.GenerateKey(rand.Reader)
					if err != nil {
						t.Fatal(err)
					}
					k, err := ssh.NewPublicKey(private.Public())
					if err != nil {
						t.Fatal(err)
					}
					return k
				}
				old, next := newKey(), newKey()
				before := []byte(knownhosts.Line([]string{"jump.invalid"}, old) + "\n" + knownhosts.Line([]string{"other.invalid"}, old) + "\n")
				if err := os.WriteFile(a.trust.Path, before, 0600); err != nil {
					t.Fatal(err)
				}
				err := a.trust.Callback(context.Background(), nil)("jump.invalid:22", &net.TCPAddr{Port: 22}, next)
				var mismatch *core.HostKeyMismatch
				if !errors.As(fmt.Errorf("ssh handshake: %w", err), &mismatch) {
					t.Fatal(err)
				}
				profile := core.NewProfile()
				profile.Name = "合成会话"
				profile.Host = "target.invalid"
				if files {
					profile.Kind = "sftp"
				}
				p := &pane{id: core.NewID(), profile: profile, status: "连接失败：" + err.Error(), hostKeyMismatch: mismatch}
				p.owner.Store(a)
				a.tabs = []*tab{{id: core.NewID(), name: profile.Name, active: p, root: &node{id: core.NewID(), pane: p}}}
				a.current = 0
				view := a.view
				if files {
					view = a.fileWindowView
				}
				tt := ui.NewTester(view, 1000, 720)
				if err := tt.Click("清理旧主机密钥…"); err != nil {
					t.Fatal(err)
				}
				if !a.nonAuthenticationModalActive() || !tt.HasText("实际冲突地址：jump.invalid:22") {
					t.Fatal("wrong cleanup target", tt.Texts())
				}
				if err := tt.Click("取消"); err != nil {
					t.Fatal(err)
				}
				after, _ := os.ReadFile(a.trust.Path)
				if !bytes.Equal(before, after) {
					t.Fatal("cancel changed trust")
				}
				if err := tt.Click("清理旧主机密钥…"); err != nil {
					t.Fatal(err)
				}
				snapshot(t, tt, fmt.Sprintf("host-key-cleanup-%t.png", files))
				action := "确认清理"
				if reconnect {
					action = "清理并重新连接"
				}
				if err := tt.Click(action); err != nil {
					t.Fatal(err)
				}
				after, _ = os.ReadFile(a.trust.Path)
				if strings.Contains(string(after), "jump.invalid") || !strings.Contains(string(after), "other.invalid") || p.hostKeyMismatch != nil || a.hostKeyCleanupOpen {
					t.Fatal("cleanup did not complete")
				}
				if reconnect {
					if !a.authOpen || a.pendingProfile.ID != profile.ID || a.features.reconnectTarget != p {
						t.Fatal("reconnection did not target the failed pane")
					}
					a.features.authCancel()
				} else if !tt.HasText("重新连接") || tt.HasText("清理旧主机密钥…") || p.conn != nil || a.authOpen || len(a.challenges) != 0 {
					t.Fatal("cleanup automatically connected/trusted")
				}
				if err := a.trust.Callback(context.Background(), nil)("jump.invalid:22", &net.TCPAddr{Port: 22}, next); err == nil {
					t.Fatal("new key implicitly accepted")
				}
			})
		}
	}
}

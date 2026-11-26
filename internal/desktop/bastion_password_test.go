package desktop

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"golang.org/x/crypto/ssh"
	"oshell-desktop/internal/core"
)

func TestBastionTerminalPasswordAutomaticOnce(t *testing.T) {
	t.Run("saved", func(t *testing.T) { exerciseBastionTargetLogin(t, "fixture-target-password") })
	t.Run("missing", func(t *testing.T) { exerciseBastionTargetLogin(t, "") })
}

func exerciseBastionTargetLogin(t *testing.T, target string) {
	t.Helper()
	const routed = "operator#UTF-8#fixture-asset#SSH#fixture-user"
	const gateway = "fixture-gateway-password"
	received := make(chan string, 4)
	allowRejection := make(chan struct{})
	reject := sync.OnceFunc(func() { close(allowRejection) })
	defer reject()
	a, p, _ := savedConnectionFixtureWithShell(t, nil, func(ch ssh.Channel) {
		fmt.Fprint(ch, "Login: fixture-user\r\nPassword:")
		buf := make([]byte, 256)
		for {
			n, err := ch.Read(buf)
			if err != nil {
				return
			}
			received <- string(buf[:n])
			<-allowRejection
			fmt.Fprint(ch, "\r\nPermission denied, please try again.\r\nPassword:")
		}
	}, func(c *ssh.ServerConfig, _ ssh.Signer) {
		c.KeyboardInteractiveCallback = func(meta ssh.ConnMetadata, ask ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
			ans, err := ask("", "", []string{"Password:"}, []bool{false})
			if err != nil || meta.User() != routed || len(ans) != 1 || ans[0] != gateway {
				return nil, fmt.Errorf("wrong fixture login")
			}
			return nil, nil
		}
	})
	p.Kind, p.Username, p.TitleMode = bastionSSH, "", "passive"
	a.editManagerProfile(p)
	editor := &a.features.sessionPassword
	editor.password, editor.remember, editor.targetPassword = gateway, true, target
	a.draft.Username, editor.serverAccount = strings.TrimSuffix(routed, "fixture-user"), "fixture-user"
	a.saveDraft(false)
	waitOperator(t, a, func() bool { return !editor.busy })
	if a.editing || len(a.tabs) != 0 {
		t.Fatal("offline save failed", a.formError)
	}
	config, err := a.store.Load()
	if err != nil {
		t.Fatal(err)
	}
	a.config = config
	a.vault, err = core.OpenVault(a.store.Dir)
	if err != nil {
		t.Fatal(err)
	}
	p, ok := a.profileByID(p.ID)
	if !ok || p.Kind != "ssh" || p.Username != routed || a.sessionType(p) != bastionSSH {
		t.Fatal("invalid persisted route")
	}
	a.managerProtocol = bastionSSH
	a.managerSelection.Add(p.ID)
	a.connectManagerSelection()
	waitOperator(t, a, func() bool {
		pane := a.activePane()
		if pane == nil || pane.term == nil {
			return false
		}
		prompt, _ := pane.term.HostPrompt()
		return prompt == "Password:"
	})
	pane := a.activePane()
	view := ui.NewTester(a.drawBastionPassword, 800, 600)
	view.Frame()
	if target == "" {
		if d := a.bastionPassword; d == nil || d.prompt != "Password:" {
			t.Fatal("missing password did not open the editor")
		}
		select {
		case <-received:
			t.Fatal("unexpected automatic input without a saved target password")
		default:
		}
		return
	}
	if a.bastionPassword != nil {
		t.Fatal("saved password still required confirmation")
	}
	select {
	case got := <-received:
		if got != target+"\r" {
			t.Fatal("unexpected submitted password")
		}
	case <-time.After(3 * time.Second):
		a.drain()
		if d := a.bastionPassword; d != nil {
			t.Fatalf("fill dialog still open, prompt=%q feedback=%q", d.prompt, d.message)
		}
		t.Fatal("saved password did not reach the remote channel", a.status)
	}
	reject()
	waitOperator(t, a, func() bool { return pane.term.TargetLoginRetry("fixture-user") })
	view.Frame()
	d := a.bastionPassword
	if d == nil || !strings.Contains(d.message, "再次请求密码") {
		t.Fatal("missing manual retry after rejection")
	}
	view.ClickAt(1, 1)
	view.Key(0, ui.KeyEscape)
	view.Frame()
	if a.bastionPassword != d {
		t.Fatal("retry dialog dismissed without clicking a button")
	}
	select {
	case <-received:
		t.Fatal("rejected credential sent a second time")
	default:
	}
	d.typed = "replacement-fixture"
	view.Frame()
	if err := view.Click("保存密码"); err != nil {
		t.Fatal(err)
	}
	if d.typed != "" {
		t.Fatal("password remained in editor")
	}
	view.Frame()
	if err := view.Click("忘记密码"); err != nil {
		t.Fatal(err)
	}
	b, _ := p.BastionTarget()
	if a.vault.HasStoredCredential(b.Credential.ID) {
		t.Fatal("target password retained")
	}
	if got, err := a.vault.GetFor(p, p.Identity()); err != nil || got != gateway {
		t.Fatal("gateway password changed")
	}
}

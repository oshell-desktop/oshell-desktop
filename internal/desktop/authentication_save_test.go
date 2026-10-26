package desktop

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"golang.org/x/crypto/ssh"
	"net"
	"oshell-desktop/internal/core"
	"strings"
	"testing"
)

func TestDynamicLoginPasswordSaving(t *testing.T) {
	for _, mode := range []string{"remember", "forget", "cancel", "rejected", "vault-changed", "profile-changed"} {
		t.Run(mode, func(t *testing.T) {
			a, p, _ := savedConnectionFixture(t)
			a.request(p, 0)
			waitOperator(t, a, func() bool { return len(a.challenges) == 1 })
			q := a.challenges[0]
			if !q.allowSave || q.remember || q.request.Name != "SSH 密码认证" {
				t.Fatal("login prompt must offer explicit, unchecked saving")
			}
			q.answers[0] = "saved-fixture-password"
			if mode == "rejected" {
				q.answers[0] = "incorrect-fixture-password"
			}
			tt := ui.NewTester(a.view, 1000, 700)
			if mode != "forget" && mode != "cancel" {
				if err := tt.Click("密码认证成功后保存登录密码"); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "vault-changed" {
				other := core.NewProfile()
				other.Host = "changed.fixture.invalid"
				if err := a.vault.Put(other, "other-fixture-password"); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "profile-changed" {
				a.config.Profiles[0].Host = "changed.fixture.invalid"
			}
			action := "继续认证"
			if mode == "cancel" {
				action = "取消"
			}
			if err := tt.Click(action); err != nil {
				t.Fatal(err)
			}
			if mode == "rejected" {
				for attempt := 0; attempt < 2; attempt++ {
					waitOperator(t, a, func() bool { return len(a.challenges) == 1 })
					a.challenges[0].answers[0] = "incorrect-fixture-password"
					tt = ui.NewTester(a.view, 1000, 700)
					if err := tt.Click("继续认证"); err != nil {
						t.Fatal(err)
					}
				}
			}
			if mode == "cancel" || mode == "rejected" {
				waitOperator(t, a, func() bool { return len(a.tabs) == 1 && strings.Contains(a.tabs[0].active.status, "连接失败") })
			} else {
				waitOperator(t, a, func() bool {
					return len(a.tabs) == 1 && a.tabs[0].active.term != nil && strings.Contains(a.tabs[0].active.term.Text(), "SAVED_CREDENTIAL_READY")
				})
			}
			if a.vault.HasStoredCredential(p.ID) != (mode == "remember") {
				t.Fatal("password saved without consent, complete authentication, or unchanged destination/vault")
			}
			if mode == "remember" {
				password, err := a.vault.GetFor(p, p.Identity())
				if err != nil || password != "saved-fixture-password" {
					t.Fatal("saved password lost target binding", err)
				}
			}
			if len(a.challenges) != 0 || q.answers[0] != "" {
				t.Fatal("prompt retained password")
			}
		})
	}
}

func TestRejectedStoredPasswordCanBeReplacedWithExplicitConsent(t *testing.T) {
	for _, remember := range []bool{false, true} {
		t.Run(map[bool]string{false: "keep-stored", true: "replace-stored"}[remember], func(t *testing.T) {
			a, p, accepted := savedConnectionFixture(t)
			if err := a.vault.PutFor(p, "stale-fixture-password", p.Identity()); err != nil {
				t.Fatal(err)
			}
			a.request(p, 0)
			waitOperator(t, a, func() bool { return len(a.challenges) == 1 })
			q := a.challenges[0]
			if q.remember || !q.allowSave || !strings.Contains(q.request.Instruction, "未通过") {
				t.Fatal("invalid rejected-password retry prompt")
			}
			q.answers[0] = "saved-fixture-password"
			tt := ui.NewTester(a.view, 1000, 700)
			if remember {
				if err := tt.Click("密码认证成功后保存登录密码"); err != nil {
					t.Fatal(err)
				}
			}
			if err := tt.Click("继续认证"); err != nil {
				t.Fatal(err)
			}
			waitOperator(t, a, func() bool {
				return len(a.tabs) == 1 && a.tabs[0].active.term != nil && strings.Contains(a.tabs[0].active.term.Text(), "SAVED_CREDENTIAL_READY")
			})
			got, err := a.vault.GetFor(p, p.Identity())
			want := "stale-fixture-password"
			if remember {
				want = "saved-fixture-password"
			}
			if err != nil || got != want || accepted.Load() != 1 {
				t.Fatal("retry lost consent or opened another connection", err, accepted.Load())
			}
		})
	}
}

func interactiveMFAFixture(config *ssh.ServerConfig, _ ssh.Signer) {
	config.PasswordCallback = nil
	config.KeyboardInteractiveCallback = func(metadata ssh.ConnMetadata, ask ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
		host, _, _ := net.SplitHostPort(metadata.LocalAddr().String())
		for _, factor := range []struct{ prompt, answer string }{{"(" + metadata.User() + "@" + host + ") Password:", "saved-fixture-password"}, {"Verification code:", "fixture-otp"}} {
			answers, err := ask("MFA", "", []string{factor.prompt}, []bool{false})
			if err != nil {
				return nil, err
			}
			if len(answers) != 1 || answers[0] != factor.answer {
				return nil, fmt.Errorf("wrong factor")
			}
		}
		return nil, nil
	}
}

func TestInteractiveMFAKeepsPasswordConsentSeparateFromOTP(t *testing.T) {
	a, p, _ := savedConnectionFixture(t, interactiveMFAFixture)
	a.request(p, 0)
	waitOperator(t, a, func() bool { return len(a.challenges) == 1 })
	passwordPrompt := a.challenges[0]
	if !passwordPrompt.allowSave {
		t.Fatal("qualified login prompt cannot save")
	}
	passwordPrompt.answers[0] = "saved-fixture-password"
	tt := ui.NewTester(a.view, 1000, 700)
	if err := tt.Click("密码认证成功后保存登录密码"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("继续认证"); err != nil {
		t.Fatal(err)
	}
	waitOperator(t, a, func() bool { return len(a.challenges) == 1 })
	otpPrompt := a.challenges[0]
	if otpPrompt == passwordPrompt || otpPrompt.allowSave || otpPrompt.remember || a.vault.HasStoredCredential(p.ID) {
		t.Fatal("OTP inherited consent or password saved before MFA finished")
	}
	otpPrompt.answers[0] = "fixture-otp"
	tt = ui.NewTester(a.view, 1000, 700)
	if err := tt.Click("继续认证"); err != nil {
		t.Fatal(err)
	}
	waitOperator(t, a, func() bool {
		return len(a.tabs) == 1 && a.tabs[0].active.term != nil && strings.Contains(a.tabs[0].active.term.Text(), "SAVED_CREDENTIAL_READY")
	})
	got, err := a.vault.GetFor(p, p.Identity())
	if err != nil || got != "saved-fixture-password" {
		t.Fatal("password replaced by OTP or lost", err)
	}
}

func mixedMFAFixture(config *ssh.ServerConfig, _ ssh.Signer) {
	config.PasswordCallback = nil
	config.KeyboardInteractiveCallback = func(metadata ssh.ConnMetadata, ask ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
		host, _, _ := net.SplitHostPort(metadata.LocalAddr().String())
		answers, err := ask("MFA", "", []string{"Account label:", "(" + metadata.User() + "@" + host + ") Password:", "Verification code:"}, []bool{true, false, false})
		if err != nil {
			return nil, err
		}
		if len(answers) != 3 || answers[0] != "fixture-label" || answers[1] != "saved-fixture-password" || answers[2] != "fixture-otp" {
			return nil, fmt.Errorf("wrong factor ordering")
		}
		return nil, nil
	}
}

// SF-P07: a mixed challenge is fully manual even with a saved password.
func TestMixedMFAKeepsAllFieldsManualAndNeverSaves(t *testing.T) {
	a, p, accepted := savedConnectionFixture(t, mixedMFAFixture)
	for connection := 1; connection <= 2; connection++ {
		if connection == 2 {
			if err := a.vault.PutFor(p, "saved-fixture-password", p.Identity()); err != nil {
				t.Fatal(err)
			}
		}
		before := a.vault.Revision()
		a.request(p, 0)
		waitOperator(t, a, func() bool { return len(a.challenges) == 1 })
		q := a.challenges[0]
		if q.allowSave || len(q.answers) != 3 || !q.request.Echo[0] || q.request.Echo[1] || q.request.Echo[2] {
			t.Fatal("mixed challenge must retain all manual fields without saving")
		}
		copy(q.answers, []string{"fixture-label", "saved-fixture-password", "fixture-otp"})
		tt := ui.NewTester(a.view, 1000, 700)
		if tt.HasText("密码认证成功后保存登录密码") {
			t.Fatal("mixed challenge offered persistence")
		}
		if err := tt.Click("继续认证"); err != nil {
			t.Fatal(err)
		}
		waitOperator(t, a, func() bool {
			return len(a.tabs) == connection && a.tabs[connection-1].active.term != nil && strings.Contains(a.tabs[connection-1].active.term.Text(), "SAVED_CREDENTIAL_READY")
		})
		if a.vault.Revision() != before {
			t.Fatal("mixed challenge changed credentials")
		}
	}
	if accepted.Load() != 2 {
		t.Fatal("unexpected reconnects")
	}
}

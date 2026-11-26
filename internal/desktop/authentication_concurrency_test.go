package desktop

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

func TestHostAuthenticationUsesCancellableQueue(t *testing.T) {
	for _, action := range []string{"trust", "cancel", "close"} {
		t.Run(action, func(t *testing.T) {
			a, profile, _ := savedConnectionFixture(t)
			if err := os.WriteFile(a.trust.Path, nil, 0600); err != nil {
				t.Fatal(err)
			}
			// Hold a different session's password prompt ahead of the host check.
			hold, _ := operatorPane(t, a, "pending password")
			holdCtx, cancel := context.WithCancel(context.Background())
			defer cancel()
			holding := &authenticationChallenge{pane: hold, id: core.NewID(), ctx: holdCtx, open: true, done: make(chan authenticationAnswer, 1)}
			a.enqueueChallenge(holding)
			a.open(profile, core.Credentials{Password: "saved-fixture-password"}, 0)
			waitOperator(t, a, func() bool { return len(a.challenges) == 2 })
			q := a.challenges[1]
			if q.host == nil || a.nextChallenge() != holding {
				t.Fatal("host approval bypassed password queue")
			}
			cancel()
			view := ui.NewTester(a.view, 1000, 700)
			if !view.HasText(q.host.Fingerprint) || !view.HasText(q.host.Address) {
				t.Fatal("host identity missing from confirmation")
			}
			if action == "close" {
				a.dispose(q.pane)
			} else {
				button := "取消"
				if action == "trust" {
					button = "信任并连接"
				}
				if err := view.Click(button); err != nil {
					t.Fatal(err)
				}
			}
			if action == "trust" {
				waitOperator(t, a, func() bool { return q.pane.term != nil })
			} else {
				waitOperator(t, a, func() bool { return q.finished })
			}
			data, err := os.ReadFile(a.trust.Path)
			if err != nil || (len(data) > 0) != (action == "trust") {
				t.Fatal("host trusted without explicit acceptance", err)
			}
		})
	}
}

func TestConcurrentAuthenticationAcrossWindows(t *testing.T) {
	a, profile, _ := savedConnectionFixture(t)
	b := newApp(a.store, a.config.Clone())
	b.vault, b.trust = a.vault, a.trust
	h := &applicationHub{store: a.store, vault: a.vault, windows: []*App{a, b}}
	a.hub, b.hub = h, h
	t.Cleanup(func() {
		for _, tab := range b.tabs {
			b.walk(tab.root, b.dispose)
		}
	})
	wait := func(condition func() bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for !condition() && time.Now().Before(deadline) {
			a.drain()
			b.drain()
			time.Sleep(time.Millisecond)
		}
		if !condition() {
			t.Fatal("concurrent authentication timed out")
		}
	}
	profile.Name = "First connection"
	a.open(profile, core.Credentials{}, 0)
	wait(func() bool { return len(a.challenges) == 1 })
	first := a.challenges[0]
	profile.Name = "Second connection"
	b.open(profile, core.Credentials{}, 0)
	wait(func() bool { return len(b.challenges) == 1 })
	second := b.challenges[0]
	profile.Name = "Cancelled pending connection"
	b.open(profile, core.Credentials{}, 0)
	wait(func() bool { return len(b.challenges) == 2 })
	pending := b.challenges[1]
	left := ui.NewTester(a.view, 1000, 700)
	right := ui.NewTester(b.view, 1000, 700)
	if !left.HasText("First connection") {
		t.Fatal("prompt does not identify its session")
	}
	if _, shown := right.Find("继续认证"); shown {
		t.Fatal("second window opened a concurrent password modal")
	}
	// A fourth connection can finish without waiting for the first password.
	b.open(profile, core.Credentials{Password: "saved-fixture-password"}, 0)
	wait(func() bool { return b.tabs[2].active.term != nil })
	if a.nextChallenge() != first {
		t.Fatal("ready connection disturbed active authentication")
	}
	b.dispose(pending.pane)
	if len(b.challenges) != 1 || b.challenges[0] != second {
		t.Fatal("pending cancellation affected another connection")
	}
	if err := left.Click("取消"); err != nil {
		t.Fatal(err)
	}
	wait(func() bool { return len(a.challenges) == 0 })
	right.Frame()
	if _, shown := right.Find("继续认证"); !shown {
		t.Fatal("next window did not receive the prompt")
	}
	second.answers[0] = "saved-fixture-password"
	if err := right.Click("继续认证"); err != nil {
		t.Fatal(err)
	}
	wait(func() bool { return b.tabs[0].active.term != nil })
	if second.answers[0] != "" || len(b.challenges) != 0 {
		t.Fatal("finished prompt retained password or queued entry")
	}
	// Reconnecting a cancelled session gets an independent, usable prompt.
	a.open(profile, core.Credentials{}, 0)
	wait(func() bool { return len(a.challenges) == 1 })
	a.challenges[0].answers[0] = "saved-fixture-password"
	left.Frame()
	if err := left.Click("继续认证"); err != nil {
		t.Fatal(err)
	}
	wait(func() bool { return a.tabs[1].active.term != nil })
}

func TestAuthenticationQueueCancellationAndMove(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	b := newApp(a.store, a.config.Clone())
	h := &applicationHub{windows: []*App{a}, fileWindow: b}
	a.hub, b.hub = h, h
	p, _ := operatorPane(t, a, "moved session")
	p2, _ := operatorPane(t, b, "other session")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q := &authenticationChallenge{pane: p, id: core.NewID(), ctx: ctx, open: true, answers: []string{"secret"}, done: make(chan authenticationAnswer, 1), request: core.AuthenticationPrompt{Questions: []string{"Password"}}}
	q2 := &authenticationChallenge{pane: p2, id: core.NewID(), ctx: context.Background(), open: true, done: make(chan authenticationAnswer, 1)}
	a.enqueueChallenge(q)
	b.enqueueChallenge(q2)
	a.editing = true
	if b.nextChallenge() != nil {
		t.Fatal("authentication stacked over another editor")
	}
	a.editing = false
	// Moving a tab keeps the original global queue order even when appended
	// behind a newer challenge in the destination window's list.
	a.transferTab(a.tabs[0], b)
	if b.nextChallenge() != q || p.owner.Load() != b {
		t.Fatal("moving a tab lost its authentication position")
	}
	view := ui.NewTester(b.authenticationChallengeDialog, 700, 500)
	view.ClickAt(1, 1)
	view.Key(0, ui.KeyEscape)
	if b.nextChallenge() != q || !q.open || q.answers[0] != "secret" {
		t.Fatal("outside click or Escape dismissed authentication or lost input")
	}
	if !view.HasText("moved session") {
		t.Fatal("prompt did not move with the session")
	}
	cancel()
	if b.nextChallenge() != q2 {
		t.Fatal("cancelled prompt blocked the file workspace")
	}
	result := <-q.done
	if !errors.Is(result.err, context.Canceled) || len(result.values) != 0 || q.answers[0] != "" {
		t.Fatal("cancelled prompt leaked answers")
	}
	b.finishChallenge(q, false)
	select {
	case <-q.done:
		t.Fatal("prompt completed twice")
	default:
	}
	b.dispose(p2)
	if b.nextChallenge() != nil {
		t.Fatal("closed session left an abandoned prompt")
	}
}

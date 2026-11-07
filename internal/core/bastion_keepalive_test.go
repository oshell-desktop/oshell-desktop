package core

import "testing"

func TestBastionKeepAliveAcrossWebLaunches(t *testing.T) {
	dir := t.TempDir()
	open := func(host, user, port string) Profile {
		t.Helper()
		r, err := ParseBastionSSHLaunch([]string{"-ssh", "-l", user, "-P", port, host})
		if err != nil {
			t.Fatal(err)
		}
		p, err := r.ProfileValue()
		if err != nil {
			t.Fatal(err)
		}
		d := DefaultKeepAlive()
		d.Interval = 19
		if err = LoadBastionKeepAlive(dir, &p, d); err != nil {
			t.Fatal(err)
		}
		return p
	}
	p := open("Gateway.example", "first-ticket", "2222")
	k := p.KeepAlive
	k.IdleText = "pwd\\r"
	k.IdleInterval = 71
	k.IdleEnabled = true
	k.Interval = 99
	if err := SaveBastionKeepAlive(dir, p.RuntimeBastionEndpoint, k); err != nil {
		t.Fatal(err)
	}
	second := open("gateway.example", "second-ticket", "2222").KeepAlive
	if second != k {
		t.Fatal("web launch did not inherit saved keepalive settings")
	}
	if open("gateway.example", "u", "22").KeepAlive.IdleEnabled || open("other.example", "u", "2222").KeepAlive.IdleEnabled {
		t.Fatal("preference crossed endpoints")
	}
	k.IdleEnabled = false
	if err := SaveBastionKeepAlive(dir, p.RuntimeBastionEndpoint, k); err != nil {
		t.Fatal(err)
	}
	if open("gateway.example", "new", "2222").KeepAlive.IdleEnabled {
		t.Fatal("could not persist disabled preference")
	}
}

package core

import (
	"strings"
	"testing"
)

func TestBastionActualUSMTemplates(t *testing.T) {
	const ticket = "test%40#@+:ticket"
	tests := []struct {
		name, host, user, title string
		args                    []string
	}{
		{"PuTTY", "example.test", "job", "", []string{"-load", "USMSSO Default", "-ssh", "-l", "job", "-pw", ticket, "-P", "2202", "example.test"}},
		{"Xshell", "2001:db8::1", "job", "系统 A:job", []string{"-url", "ssh://job:" + ticket + "@[2001:db8::1]:2202", "-newtab", "系统", "A:job"}},
		{"SSH URI", "example.test", "job", "", []string{"ssh://job:test%2540%23%40%2B%3Aticket@example.test:2202"}},
		{"SecureCRT", "example.test", "job", "", []string{"/SSH2", "/AUTH", "password", "/L", "job", "/PASSWORD", ticket, "/P", "2202", "example.test"}},
		{"SecureCRT session plus endpoint", "example.test", "job", "", []string{"/F", "C:\\USM Config", "/T", "/S", "USM\\asset", "/L", "job", "/PASSWORD", ticket, "/P", "2202", "example.test"}},
		{"MobaXterm", "example.test", "job_SSO_asset", "", []string{"-newtab", "sshpass -p " + ticket + " ssh -p 2202 'job_SSO_asset'@example.test"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if !IsBastionSSHLaunch(tc.args) {
				t.Fatal("did not route launch")
			}
			r, err := ParseBastionSSHLaunch(tc.args)
			if err != nil {
				t.Fatal(err)
			}
			if !r.BastionSSH || r.Host != tc.host || r.Username != tc.user || r.Port != 2202 || r.Password == nil || *r.Password != ticket {
				t.Fatal("connection parameters did not round trip")
			}
			p, err := r.ProfileValue()
			if err != nil || tc.title != "" && p.Name != tc.title {
				t.Fatal("session title mismatch")
			}
		})
	}
}

func TestBastionRejectsInvalidAndUnsupportedRequests(t *testing.T) {
	for _, args := range [][]string{
		nil, {"-load", "other", "-ssh", "host"}, {"-load", "USMSSO Default", "-telnet", "host"},
		{"/SCRIPT", "file.vbs"}, {"/F", "config", "/S", "USM\\asset"},
		{"-url", "ftp://user:test-ticket@host"}, {"-ssh", "-pw", "test-ticket", "host"},
		{"-ssh", "-l", "test", "-pw", "test-ticket", "-P", "65536", "host"},
		{"-url", "ssh://job:test-ticket@example.test:22", "-pw", "again"},
		{"-newtab", "sshpass -p test-ticket ssh -p 22 'job'@host; echo injected"},
		{"-url", "ssh://job:test-ticket@host/invalid"},
	} {
		r, err := ParseBastionSSHLaunch(args)
		if err == nil || strings.Contains(err.Error(), "test-ticket") || r.Password != nil {
			t.Fatal("invalid launch accepted or disclosed")
		}
	}
	for _, args := range [][]string{nil, {"--external-launch-service"}, {"-SSH", "host", "-SSHUSER", "test"}, {"--site", "example"}, {"sftp://example.test"}} {
		if IsBastionSSHLaunch(args) {
			t.Fatal("intercepted existing launcher")
		}
	}
}

package core

import (
	"errors"
	"strings"
	"testing"
)

func TestSSOToolLaunchRouting(t *testing.T) {
	path := `D:\SSO Tools\CBH_SSH_test asset_22_abcd1234.xsh`
	if ssoToolPath(strings.Fields(path)) != path || !IsBastionSSHLaunch([]string{path}) {
		t.Fatal("unquoted SsoTool session path was not recognized")
	}
	for _, args := range [][]string{{`D:\file.xsh`}, {"-url", "ssh://user@host"}, {"--external-launch-service"}, {"/SSH:example.test"}} {
		if IsSSOToolLaunch(args) {
			t.Fatal("unexpected SsoTool route")
		}
	}
	if _, err := readSSOToolSession(`\\example.invalid\share\CBH_SSH_test.xsh`); !errors.Is(err, errSSOTool) {
		t.Fatal("accepted network session path")
	}
}

func TestSSOToolRejectsUnverifiedSession(t *testing.T) {
	base := "[SessionInfo]\nVersion=6.0\n[CONNECTION]\nPort=2222\nHost=localhost\nProtocol=SSH\n[CONNECTION:AUTHENTICATION]\nUserName=#sso\nPassword=fixture-only\n"
	for _, s := range []string{base, strings.Replace(base, "Protocol=SSH", "Protocol=TELNET", 1), base + "Script=test\n", base + "Password=duplicate\n", strings.Replace(base, "2222", "65536", 1)} {
		r, err := parseSSOToolSession([]byte(s))
		if !errors.Is(err, errSSOTool) || r.Password != nil || strings.Contains(err.Error(), "fixture-only") {
			t.Fatal("malformed or unverified ticket accepted or disclosed")
		}
	}
}

package desktop

import (
	"oshell-desktop/internal/core"
	"slices"
	"testing"
)

func TestEditorProtocolDefaultsAndCustomValues(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	a.config.SessionDefaults.SSHPort = 2222
	a.config.SessionDefaults.FTPPort = 2121
	a.edit(a.newProfile("ssh"))
	a.draft.Kind = "ftp"
	a.syncEditorProtocol()
	if a.port != "2121" || a.draft.Username != "anonymous" || a.draft.InitialDirectory != "/" {
		t.Fatal("new protocol defaults not applied")
	}
	a.port = "9000"
	a.draft.Username = "custom"
	a.draft.InitialDirectory = "/custom"
	a.draft.Kind = "sftp"
	a.syncEditorProtocol()
	if a.port != "9000" || a.draft.Username != "custom" || a.draft.InitialDirectory != "/custom" {
		t.Fatal("custom values overwritten")
	}
	pages, _ := editorProtocolPages("ftp")
	if len(pages) != 1 {
		t.Fatal("FTP exposes SSH pages")
	}
	pages, sections := editorProtocolPages("sftp")
	if slices.Contains(pages, "端口转发") || !slices.Contains(sections, 3) {
		t.Fatal("SFTP pages wrong")
	}
}
func TestSavingFTPDiscardsSSHOnlyConfiguration(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	p := core.NewProfile()
	p.Host = "fixture.invalid"
	p.Username = "tester"
	p.IdentityFile = "key"
	p.LegacySSH = true
	p.SSHAuthenticationMethods = []string{"publickey"}
	p.ProxyID = core.NewID()
	p.JumpHost = "jump.invalid"
	a.config.Profiles = []core.Profile{p}
	a.edit(p)
	a.draft.Kind = "ftp"
	a.features.interval = "bad hidden value"
	a.features.proxyPort = "bad hidden value"
	if err := a.applyConnectionEditor(); err != nil {
		t.Fatal(err)
	}
	if a.port != "21" || a.draft.InitialDirectory != "/" || a.draft.ProxyID != "" || a.draft.IdentityFile != "" || a.draft.JumpHost != "" || a.draft.LegacySSH || len(a.draft.Tunnels) != 0 || a.draft.SSHAuthenticationMethods != nil {
		t.Fatal("FTP retained SSH-only fields")
	}
}

func TestEditorPreservesSharedSSHAuthenticationMethods(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	p := core.NewProfile()
	p.Host = "fixture.invalid"
	p.SSHAuthenticationMethods = []string{"gssapi-with-mic", "keyboard-interactive"}
	a.config.Profiles = []core.Profile{p}
	a.edit(p)
	a.draft.Name = "Renamed on Desktop"
	if err := a.applyConnectionEditor(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(a.draft.SSHAuthenticationMethods, p.SSHAuthenticationMethods) {
		t.Fatal("editing lost shared authentication policy")
	}
	a.draft.Kind = "sftp"
	if err := a.applyConnectionEditor(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(a.draft.SSHAuthenticationMethods, p.SSHAuthenticationMethods) {
		t.Fatal("SFTP lost SSH authentication policy")
	}
}

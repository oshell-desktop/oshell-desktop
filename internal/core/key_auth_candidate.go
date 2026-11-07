package core

import (
	"context"
	"golang.org/x/crypto/ssh/agent"
	"os"
	"time"
)

// SSHKeyAuthenticationCandidate probes only local key-file metadata or the
// agent's public key list. Actual signing/decryption remains in authentication.
func SSHKeyAuthenticationCandidate(ctx context.Context, p Profile) bool {
	if p.Kind != "ssh" && p.Kind != "sftp" {
		return false
	}
	paths := append([]string(nil), p.resolvedIdentityFiles...)
	if os.Getenv("OSHELL_TEST_EXPLICIT_KEYS_ONLY") == "1" {
		paths = nil
	}
	if p.IdentityFile != "" {
		paths = append(paths, expandHome(p.IdentityFile))
	}
	for _, path := range paths {
		if ctx.Err() != nil {
			return false
		}
		if info, err := os.Stat(expandHome(path)); err == nil && info.Mode().IsRegular() {
			return true
		}
	}
	if p.resolvedAgentDisabled || p.resolvedIdentitiesOnly {
		return false
	}
	probe, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	connection, err := dialSSHAgent(probe)
	if err != nil {
		return false
	}
	defer connection.Close()
	stop := context.AfterFunc(probe, func() { connection.Close() })
	defer stop()
	connection.SetDeadline(time.Now().Add(2 * time.Second))
	keys, err := agent.NewClient(connection).Signers()
	return err == nil && len(keys) > 0 && probe.Err() == nil
}

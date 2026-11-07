package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/crypto/ssh"
)

// Match OpenSSH's identity selection: configured identities available in the
// agent first, then other agent keys unless IdentitiesOnly, then local keys.
// Agent lookup is separate so tests never need to access a user's real agent.
func loadSSHSigners(ctx context.Context, p Profile, cred Credentials, agents []ssh.Signer) ([]ssh.Signer, error) {
	if p.resolvedAgentDisabled {
		agents = nil
	}
	identities := append([]string(nil), p.resolvedIdentityFiles...)
	if os.Getenv("OSHELL_TEST_EXPLICIT_KEYS_ONLY") == "1" {
		identities = nil
	}
	explicit := expandHome(p.IdentityFile)
	if explicit != "" {
		identities = append([]string{explicit}, identities...)
	} else if !p.resolvedIdentityFilesSet && len(identities) == 0 && os.Getenv("OSHELL_TEST_EXPLICIT_KEYS_ONLY") != "1" {
		if home, err := os.UserHomeDir(); err == nil {
			for _, name := range []string{"id_ed25519", "id_ecdsa", "id_rsa"} {
				identities = append(identities, filepath.Join(home, ".ssh", name))
			}
		}
	}
	agentByKey := map[string]ssh.Signer{}
	for _, signer := range agents {
		agentByKey[string(signer.PublicKey().Marshal())] = signer
	}
	var preferred, local []ssh.Signer
	seenPaths := map[string]bool{}
	for _, path := range identities {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		path = expandHome(path)
		if seenPaths[path] {
			continue
		}
		seenPaths[path] = true
		data, err := ReadBounded(path)
		if err != nil {
			if path == explicit {
				return nil, err
			}
			continue
		}
		// Parse unencrypted keys first: an old saved passphrase must not make
		// an otherwise valid, unencrypted identity unusable.
		signer, err := ssh.ParsePrivateKey(data)
		var missing *ssh.PassphraseMissingError
		encrypted := errors.As(err, &missing)
		var public ssh.PublicKey
		if signer != nil {
			public = signer.PublicKey()
		} else if encrypted {
			public = missing.PublicKey
		} else if key, _, _, _, e := ssh.ParseAuthorizedKey(data); e == nil {
			public = key
		}
		if public != nil {
			if agentSigner := agentByKey[string(public.Marshal())]; agentSigner != nil {
				preferred = append(preferred, agentSigner)
				continue
			}
		}
		if encrypted && cred.KeyPassphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(data, []byte(cred.KeyPassphrase))
		}
		for attempt := 0; encrypted && err != nil && cred.Challenge != nil && attempt < 3; attempt++ {
			if e := ctx.Err(); e != nil {
				return nil, e
			}
			instruction := filepath.Base(path)
			if attempt > 0 || cred.KeyPassphrase != "" {
				instruction += "（口令不正确，请重试）"
			}
			answers, e := cred.Challenge(ctx, AuthenticationPrompt{Identity: p.Identity(), Name: "解锁 SSH 私钥", Instruction: instruction, Questions: []string{"私钥口令"}, Echo: []bool{false}})
			if e != nil {
				return nil, e
			}
			if e := ctx.Err(); e != nil {
				return nil, e
			}
			if len(answers) != 1 {
				return nil, fmt.Errorf("私钥口令回答无效")
			}
			signer, err = ssh.ParsePrivateKeyWithPassphrase(data, []byte(answers[0]))
		}
		if err != nil {
			if path == explicit {
				return nil, fmt.Errorf("私钥读取失败: %w", err)
			}
			continue
		}
		local = append(local, signer)
	}
	if !p.resolvedIdentitiesOnly && cred.Method != "privateKey" {
		preferred = append(preferred, agents...)
	}
	preferred = append(preferred, local...)
	var result []ssh.Signer
	seenKeys := map[string]bool{}
	for _, signer := range preferred {
		key := string(signer.PublicKey().Marshal())
		if !seenKeys[key] {
			seenKeys[key] = true
			result = append(result, signer)
		}
	}
	return result, nil
}

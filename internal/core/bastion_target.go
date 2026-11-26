package core

import (
	"strings"
	"unicode"
)

const bastionTargetPrefix = "bastion-target/"

// Routed SSH logins identify both the asset and its account in the gateway username.
// The separate vault entry is local to this profile, outside shared session credentials.
type BastionTarget struct {
	Asset, Account string
	Credential     Profile
}

// SplitBastionLogin also accepts a quick-login string awaiting its final account.
func SplitBastionLogin(login string) (asset, account string, ok bool) {
	fields := strings.Split(login, "#")
	if len(fields) != 5 || !strings.EqualFold(fields[3], "SSH") || strings.ContainsFunc(login, unicode.IsControl) {
		return "", "", false
	}
	for _, field := range fields[:4] {
		if strings.TrimSpace(field) == "" {
			return "", "", false
		}
	}
	return fields[2], fields[4], true
}

func (p Profile) BastionTarget() (BastionTarget, bool) {
	if p.Kind != "ssh" || !uuidText.MatchString(p.ID) {
		return BastionTarget{}, false
	}
	asset, account, ok := SplitBastionLogin(p.Username)
	if !ok || strings.TrimSpace(account) == "" {
		return BastionTarget{}, false
	}
	return BastionTarget{Asset: asset, Account: account, Credential: Profile{
		ID: bastionTargetPrefix + strings.ToLower(p.ID), Kind: "bastion-target",
		Host: p.Host, Port: p.Port, Username: p.Username,
	}}, true
}

// PasswordPrompt recognizes the current line for manual filling. Automatic
// submission also requires the matching account in the initial login dialogue.
func (b BastionTarget) PasswordPrompt(line string) bool {
	text := strings.ReplaceAll(strings.TrimSpace(line), "：", ":")
	prompt, ok := strings.CutSuffix(text, ":")
	if !ok {
		return false
	}
	prompt = strings.TrimSpace(prompt)
	switch strings.ToLower(prompt) {
	case "password", "server password", "target password", "target account password", "密码", "请输入密码", "服务器密码", "请输入服务器密码", "目标服务器密码", "请输入目标服务器密码":
		return true
	}
	userHost, ok := strings.CutSuffix(prompt, "'s password")
	if !ok {
		userHost, ok = strings.CutSuffix(prompt, ") Password")
		if !ok || !strings.HasPrefix(userHost, "(") {
			return false
		}
		userHost = strings.TrimPrefix(userHost, "(")
	}
	user, host, ok := strings.Cut(userHost, "@")
	return ok && user == b.Account && validHost(strings.Trim(host, "[]"))
}

func pruneBastionTargets(entries map[string]vaultEntry, profiles []Profile) {
	valid := make(map[string]string)
	for _, p := range profiles {
		if target, ok := p.BastionTarget(); ok {
			valid[target.Credential.ID] = credentialIdentity(target.Credential)
		}
	}
	for id, entry := range entries {
		if strings.HasPrefix(id, bastionTargetPrefix) && entry.Identity != valid[id] {
			delete(entries, id)
		}
	}
}

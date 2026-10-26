package core

import "strings"

// loginPasswordPrompt is scoped to the resolved SSH endpoint. Account-qualified
// prompts must match that endpoint exactly; arbitrary password-like text is not
// sufficient to disclose or remember a login password.
func loginPasswordPrompt(identity SSHIdentity, prompt string) bool {
	clean := strings.TrimSpace(prompt)
	expected := identity.User + "@" + strings.Trim(identity.Host, "[]")
	return clean == expected+"'s password:" || clean == "("+expected+") Password:" || clean == "("+expected+") password:"
}

// loginPasswordQuestion only recognizes a single, identity-qualified challenge.
// Mixed challenges remain manual even when one field resembles a login prompt.
func loginPasswordQuestion(identity SSHIdentity, questions []string, echo []bool) int {
	if len(questions) != 1 || len(echo) != 1 {
		return -1
	}
	index := -1
	for i, question := range questions {
		if !echo[i] && loginPasswordPrompt(identity, question) {
			if index >= 0 {
				return -1
			}
			index = i
		}
	}
	return index
}

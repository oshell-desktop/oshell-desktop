package core

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strings"
	"unicode"
)

// A challenge is scoped to one request. Nonces and credentials are never cached
// across destinations or concurrent configuration/font transfers.
type webDAVDigest struct {
	fields    map[string]string
	algorithm string
}

func parseWebDAVDigest(header string) (webDAVDigest, error) {
	invalid := func() (webDAVDigest, error) { return webDAVDigest{}, safeSyncFailure("authentication") }
	if len(header) > 65536 || strings.ContainsAny(header, "\r\n\x00") {
		return invalid()
	}
	token := func(c byte) bool {
		return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", rune(c))
	}
	type challenge struct {
		scheme string
		fields map[string]string
	}
	var candidates []challenge
	var current *challenge
	for i := 0; i < len(header); {
		for i < len(header) && (header[i] == ' ' || header[i] == '\t' || header[i] == ',') {
			i++
		}
		if i == len(header) {
			break
		}
		start := i
		for i < len(header) && token(header[i]) {
			i++
		}
		if start == i {
			return invalid()
		}
		name := strings.ToLower(header[start:i])
		for i < len(header) && (header[i] == ' ' || header[i] == '\t') {
			i++
		}
		if i == len(header) || header[i] != '=' {
			candidates = append(candidates, challenge{name, map[string]string{}})
			current = &candidates[len(candidates)-1]
			continue
		}
		i++
		for i < len(header) && (header[i] == ' ' || header[i] == '\t') {
			i++
		}
		if current == nil || i == len(header) {
			return invalid()
		}
		if _, exists := current.fields[name]; exists {
			return invalid()
		}
		var value strings.Builder
		if header[i] == '"' {
			i++
			for i < len(header) && header[i] != '"' {
				if header[i] == '\\' {
					i++
					if i == len(header) {
						return invalid()
					}
				}
				value.WriteByte(header[i])
				i++
			}
			if i == len(header) {
				return invalid()
			}
			i++
		} else {
			start = i
			for i < len(header) && token(header[i]) {
				i++
			}
			if start == i {
				return invalid()
			}
			value.WriteString(header[start:i])
		}
		current.fields[name] = value.String()
	}
	for _, algorithm := range []string{"SHA-256-SESS", "SHA-256", "MD5-SESS", "MD5"} {
		for _, candidate := range candidates {
			f := candidate.fields
			a := strings.ToUpper(f["algorithm"])
			if a == "" {
				a = "MD5"
			}
			_, realm := f["realm"]
			if candidate.scheme != "digest" || !realm || f["nonce"] == "" || a != algorithm || f["charset"] != "" && !strings.EqualFold(f["charset"], "UTF-8") || strings.EqualFold(f["userhash"], "true") {
				continue
			}
			if qop, exists := f["qop"]; exists {
				auth := false
				for _, option := range strings.Split(qop, ",") {
					auth = auth || strings.EqualFold(strings.TrimSpace(option), "auth")
				}
				if !auth {
					continue
				}
			}
			return webDAVDigest{f, a}, nil
		}
	}
	return invalid()
}

func (d webDAVDigest) authorization(method string, target *url.URL, username, password, cnonce string) (string, error) {
	if cnonce == "" {
		nonce := make([]byte, 16)
		if _, err := rand.Read(nonce); err != nil {
			return "", err
		}
		cnonce = hex.EncodeToString(nonce)
	}
	hash := func(s string) string {
		if strings.HasPrefix(d.algorithm, "SHA-256") {
			sum := sha256.Sum256([]byte(s))
			return hex.EncodeToString(sum[:])
		}
		sum := md5.Sum([]byte(s))
		return hex.EncodeToString(sum[:])
	}
	uri := target.RequestURI()
	ha1 := hash(username + ":" + d.fields["realm"] + ":" + password)
	if strings.HasSuffix(d.algorithm, "-SESS") {
		ha1 = hash(ha1 + ":" + d.fields["nonce"] + ":" + cnonce)
	}
	_, qop := d.fields["qop"]
	middle := ""
	if qop {
		middle = "00000001:" + cnonce + ":auth:"
	}
	response := hash(ha1 + ":" + d.fields["nonce"] + ":" + middle + hash(method+":"+uri))
	pairs := [][2]string{{"username", username}, {"realm", d.fields["realm"]}, {"nonce", d.fields["nonce"]}, {"uri", uri}, {"response", response}}
	if opaque, exists := d.fields["opaque"]; exists {
		pairs = append(pairs, [2]string{"opaque", opaque})
	}
	if qop || strings.HasSuffix(d.algorithm, "-SESS") {
		pairs = append(pairs, [2]string{"cnonce", cnonce})
	}
	parts := []string{"algorithm=" + d.algorithm}
	for _, pair := range pairs {
		if strings.ContainsFunc(pair[1], unicode.IsControl) {
			return "", safeSyncFailure("authentication")
		}
		parts = append(parts, pair[0]+"=\""+strings.NewReplacer("\\", "\\\\", "\"", "\\\"").Replace(pair[1])+"\"")
	}
	if qop {
		parts = append(parts, "qop=auth", "nc=00000001")
	}
	return "Digest " + strings.Join(parts, ", "), nil
}

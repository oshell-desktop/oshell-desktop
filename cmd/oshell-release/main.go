// oshell-release signs the exact archives that will be attached to a GitHub Release.
package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"oshell-desktop/internal/core"
)

func main() {
	repository := flag.String("repo", "", "GitHub owner/repo")
	tag := flag.String("tag", "", "release tag, e.g. v1.2.3")
	build := flag.String("build", "", "positive build number")
	notes := flag.String("notes", "", "release notes file")
	dir := flag.String("dir", ".", "directory containing the release archives")
	flag.Parse()
	err := sign(*dir, *repository, *tag, *build, *notes, os.Stdin, core.UpdatePublicKey)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func sign(dir, repository, tag, build, notesPath string, secret io.Reader, pinned string) error {
	source, err := core.ParseUpdateSource(repository)
	if err != nil {
		return err
	}
	version, ok := strings.CutPrefix(tag, "v")
	if _, err := core.CompareUpdateVersions(version, "0.0.0"); !ok || err != nil {
		return fmt.Errorf("invalid release tag %q", tag)
	}
	notes, err := os.ReadFile(notesPath)
	if err != nil {
		return err
	}
	keyPEM, err := io.ReadAll(secret)
	if err != nil {
		return fmt.Errorf("cannot read signing key")
	}
	block, _ := pem.Decode(keyPEM)
	if block == nil || block.Type != "PRIVATE KEY" {
		return fmt.Errorf("OSHELL_UPDATE_SIGNING_KEY must contain a PKCS8 PEM private key")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	key, ok := parsed.(ed25519.PrivateKey)
	pub, pubErr := core.ParseUpdatePublicKey(pinned)
	if err != nil || !ok || pubErr != nil || !pub.Equal(key.Public()) {
		return fmt.Errorf("signing key does not match the application's pinned Ed25519 key")
	}
	manifest := core.UpdateManifest{ApplicationID: "dev.oshell.desktop", Repository: source.Repository, Version: version, Build: build, Notes: strings.TrimSpace(string(notes)), Platforms: map[string]core.UpdateArchive{}}
	for _, target := range []string{"windows-amd64", "linux-amd64"} {
		name := core.ReleaseAssetName(target)
		archive, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		hash := sha256.New()
		size, err := io.Copy(hash, archive)
		archive.Close()
		if err != nil {
			return err
		}
		digest := hash.Sum(nil)
		manifest.Platforms[target] = core.UpdateArchive{
			URL:  "https://github.com/" + source.Repository + "/releases/download/" + tag + "/" + name,
			Size: size, SHA256: hex.EncodeToString(digest), Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(key, digest)),
		}
	}
	data, err := core.SignUpdateManifest(manifest, key)
	if err != nil {
		return err
	}
	for target := range manifest.Platforms {
		if _, err := core.DecodeSignedUpdate(data, source, target, manifest.ApplicationID, "0.0.0", "", pinned); err != nil {
			return err
		}
	}
	return os.WriteFile(filepath.Join(dir, "latest.json"), data, 0644)
}

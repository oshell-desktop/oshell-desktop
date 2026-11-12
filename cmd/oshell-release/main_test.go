package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"oshell-desktop/internal/core"
)

func TestSignReleaseArchives(t *testing.T) {
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	secret := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	pinned := base64.StdEncoding.EncodeToString(pub)
	dir := t.TempDir()
	notes := filepath.Join(dir, "notes")
	if err := os.WriteFile(notes, []byte("更新说明"), 0600); err != nil {
		t.Fatal(err)
	}
	targets := []string{"windows-amd64", "linux-amd64"}
	for _, target := range targets {
		if err := os.WriteFile(filepath.Join(dir, core.ReleaseAssetName(target)), []byte(target), 0600); err != nil {
			t.Fatal(err)
		}
	}
	source := core.UpdateSource{Repository: "example/desktop"}
	wrongKey := base64.StdEncoding.EncodeToString(make([]byte, ed25519.PublicKeySize))
	if sign(dir, source.Repository, "v1.2.3", "9", notes, bytes.NewReader(secret), wrongKey) == nil {
		t.Fatal("accepted signing key different from the application's pinned key")
	}
	if err := sign(dir, source.Repository, "v1.2.3", "9", notes, bytes.NewReader(secret), pinned); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "latest.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range targets {
		update, err := core.DecodeSignedUpdate(data, source, target, "dev.oshell.desktop", "1.2.2", "", pinned)
		digest := sha256.Sum256([]byte(target))
		if err != nil || update == nil || update.Archive.SHA256 != hex.EncodeToString(digest[:]) || update.Manifest.Notes != "更新说明" {
			t.Fatalf("client could not verify published %s archive: %v", target, err)
		}
	}
}

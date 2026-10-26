package core

import (
	"crypto/ed25519"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json/v2"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const UpdateMetadataLimit = 1 << 20
const UpdateArchiveLimit = 1 << 30
const updateSignatureDomain = "OShellDesktop update metadata v1\n"

// Pinned release key. The private PKCS8 key is kept outside tracked sources
// and never shipped. Repository responses cannot choose their own trust key.
//
//go:embed update_public_key.txt
var UpdatePublicKey string
var UpdateBuild string
var updateBuildPattern = regexp.MustCompile(`^[1-9][0-9]{0,17}$`)
var updateVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

type UpdateArchive struct {
	URL       string `json:"url"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256"`
	Signature string `json:"signature"`
	MinimumOS string `json:"minimumOS"`
}
type UpdateManifest struct {
	Repository    string                   `json:"repository"`
	ApplicationID string                   `json:"applicationID"`
	Version       string                   `json:"version"`
	Build         string                   `json:"build"`
	Notes         string                   `json:"notes"`
	Platforms     map[string]UpdateArchive `json:"platforms"`
}
type signedUpdateEnvelope struct {
	Format    string `json:"format"`
	Payload   []byte `json:"payload"`
	Signature []byte `json:"signature"`
}
type VerifiedUpdate struct {
	Manifest UpdateManifest
	Archive  UpdateArchive
	Target   string
	// Only produced after signature validation; retained for the download check.
	publicKey                             ed25519.PublicKey
	archive                               UpdateArchive
	applicationID, version, build, target string
}

func ParseUpdatePublicKey(value string) (ed25519.PublicKey, error) {
	key, e := base64.StdEncoding.DecodeString(strings.TrimSpace(value))
	if e != nil || len(key) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("此构建缺少有效的更新签名公钥")
	}
	return ed25519.PublicKey(key), nil
}
func CompareUpdateVersions(a, b string) (int, error) {
	if !updateVersion.MatchString(a) || !updateVersion.MatchString(b) {
		return 0, fmt.Errorf("更新版本必须为正式三段版本号")
	}
	left, right := strings.Split(a, "."), strings.Split(b, ".")
	for i := range left {
		x, e := strconv.ParseUint(left[i], 10, 64)
		if e != nil {
			return 0, fmt.Errorf("版本号超出范围")
		}
		y, e := strconv.ParseUint(right[i], 10, 64)
		if e != nil {
			return 0, fmt.Errorf("版本号超出范围")
		}
		if x < y {
			return -1, nil
		}
		if x > y {
			return 1, nil
		}
	}
	return 0, nil
}
func DecodeSignedUpdate(data []byte, source UpdateSource, target, applicationID, currentVersion, currentBuild, keyText string) (*VerifiedUpdate, error) {
	if len(data) > UpdateMetadataLimit {
		return nil, fmt.Errorf("更新信息超过大小限制")
	}
	key, e := ParseUpdatePublicKey(keyText)
	if e != nil {
		return nil, e
	}
	var envelope signedUpdateEnvelope
	if e = json.Unmarshal(data, &envelope); e != nil || envelope.Format != "OShellDesktop.signed-updates.v1" || len(envelope.Payload) == 0 || len(envelope.Payload) > 32768 || len(envelope.Signature) != ed25519.SignatureSize {
		return nil, fmt.Errorf("签名更新信息格式无效")
	}
	message := append([]byte(updateSignatureDomain), envelope.Payload...)
	if !ed25519.Verify(key, message, envelope.Signature) {
		return nil, fmt.Errorf("更新元数据签名验证失败")
	}
	var manifest UpdateManifest
	if e = json.Unmarshal(envelope.Payload, &manifest); e != nil {
		return nil, fmt.Errorf("签名更新内容无效")
	}
	if manifest.ApplicationID != applicationID || !strings.EqualFold(manifest.Repository, source.Repository) {
		return nil, fmt.Errorf("更新信息与当前应用或仓库不匹配")
	}
	build, e := strconv.ParseUint(manifest.Build, 10, 64)
	if e != nil || build == 0 || !updateBuildPattern.MatchString(manifest.Build) {
		return nil, fmt.Errorf("更新构建号无效")
	}
	comparison, e := CompareUpdateVersions(manifest.Version, currentVersion)
	if e != nil {
		return nil, e
	}
	archive, ok := manifest.Platforms[target]
	if !ok || !source.acceptsArchive(archive.URL, manifest.Version, target) || archive.Size <= 0 || archive.Size > UpdateArchiveLimit {
		return nil, fmt.Errorf("更新安装包与系统、架构或仓库不匹配")
	}
	if archive.MinimumOS != "13.0" && (target == "darwin-arm64" || target == "darwin-amd64") {
		return nil, fmt.Errorf("更新安装包最低系统版本不匹配")
	}
	digest, e := hex.DecodeString(archive.SHA256)
	signature, se := base64.StdEncoding.DecodeString(archive.Signature)
	if e != nil || len(digest) != sha256.Size || se != nil || len(signature) != ed25519.SignatureSize || !ed25519.Verify(key, digest, signature) {
		return nil, fmt.Errorf("安装包摘要或签名无效")
	}
	// Even correctly signed old artifacts may not roll back the installed build.
	if currentBuild != "" {
		current, e := strconv.ParseUint(currentBuild, 10, 64)
		if e != nil {
			return nil, fmt.Errorf("当前构建号无效")
		}
		if build <= current {
			return nil, nil
		}
	}
	if comparison < 0 || (comparison == 0 && currentBuild == "") {
		return nil, nil
	}
	return &VerifiedUpdate{Manifest: manifest, Archive: archive, Target: target, publicKey: append(ed25519.PublicKey(nil), key...), archive: archive, applicationID: manifest.ApplicationID, version: manifest.Version, build: manifest.Build, target: target}, nil
}

// SignUpdateManifest is used by release tooling with a separately held private
// key. It never creates or persists a release signing key.
func SignUpdateManifest(manifest UpdateManifest, key ed25519.PrivateKey) ([]byte, error) {
	if len(key) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("签名私钥无效")
	}
	payload, e := json.Marshal(manifest)
	if e != nil || len(payload) > 32768 {
		return nil, fmt.Errorf("更新信息过大或无效")
	}
	signature := ed25519.Sign(key, append([]byte(updateSignatureDomain), payload...))
	return json.Marshal(signedUpdateEnvelope{Format: "OShellDesktop.signed-updates.v1", Payload: payload, Signature: signature})
}

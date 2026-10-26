package core

// Portable updates require pinned Ed25519 metadata and archive signatures.
// The private-repository transport is optional; it never establishes trust.
import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const DefaultUpdateRepository = "oshell-desktop/oshell-desktop"

type ReleaseUpdate struct {
	verified           *VerifiedUpdate
	Version, Notes     string
	asset              releaseAsset
	target             string
	token, assetAPIURL string
}
type releaseAsset struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
	State  string `json:"state"`
}
type githubRelease struct {
	Tag        string         `json:"tag_name"`
	Body       string         `json:"body"`
	Draft      bool           `json:"draft"`
	Prerelease bool           `json:"prerelease"`
	Assets     []releaseAsset `json:"assets"`
}

func (u *ReleaseUpdate) Size() int64 { return u.asset.Size }

func releaseAssetName(target string) string {
	switch target {
	case "windows-amd64":
		return "OShell-Desktop-windows-amd64-single-file.zip"
	case "linux-amd64":
		return "OShell-Desktop-linux-amd64.tar.gz"
	}
	return ""
}
func decodeRelease(data []byte, source UpdateSource, target, current string) (*ReleaseUpdate, error) {
	var release githubRelease
	if len(data) > UpdateMetadataLimit || json.Unmarshal(data, &release) != nil {
		return nil, fmt.Errorf("GitHub Release 信息无效")
	}
	if release.Draft || release.Prerelease {
		return nil, fmt.Errorf("不接受草稿或预发布更新")
	}
	version := strings.TrimPrefix(release.Tag, "v")
	comparison, err := CompareUpdateVersions(version, current)
	if err != nil {
		return nil, err
	}
	if comparison <= 0 {
		return nil, nil
	}
	name := releaseAssetName(target)
	if name == "" {
		return nil, fmt.Errorf("当前系统或架构不支持自更新")
	}
	var found *ReleaseUpdate
	for _, asset := range release.Assets {
		if asset.Name != name {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("Release 包含重复更新附件")
		}
		suffix := "/releases/download/" + release.Tag + "/" + name
		if asset.State != "uploaded" || asset.Size <= 0 || asset.Size > UpdateArchiveLimit || !strings.HasSuffix(asset.URL, suffix) || !strings.EqualFold(strings.TrimSuffix(asset.URL, suffix), "https://github.com/"+source.Repository) {
			return nil, fmt.Errorf("更新附件地址或大小无效")
		}
		digest, err := hex.DecodeString(strings.TrimPrefix(asset.Digest, "sha256:"))
		if !strings.HasPrefix(asset.Digest, "sha256:") || err != nil || len(digest) != sha256.Size {
			return nil, fmt.Errorf("Release 附件缺少 GitHub SHA-256 摘要，请重新上传更新包")
		}
		found = &ReleaseUpdate{Version: version, Notes: release.Body, asset: asset, target: target}
	}
	if found == nil {
		return nil, fmt.Errorf("Release %s 尚未提供 %s", version, name)
	}
	return found, nil
}
func CheckReleaseUpdate(ctx context.Context, cache *UpdateMetadataCache, source UpdateSource, target, current string, credentials ...string) (*ReleaseUpdate, error) {
	token := ""
	if len(credentials) > 0 {
		token = credentials[0]
	}
	return checkSignedPortableUpdate(ctx, cache, source, target, current, UpdateBuild, UpdatePublicKey, token)
}
func (u *ReleaseUpdate) download(ctx context.Context, directory string, progress func(int64, int64), client *http.Client) (path string, err error) {
	if err = u.verifyAuthority(); err != nil {
		return "", err
	}
	url := u.asset.URL
	if u.token != "" {
		url = u.assetAPIURL
	}
	request, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	if u.token != "" {
		if request.URL.Scheme != "https" || request.URL.Host != "api.github.com" {
			return "", fmt.Errorf("私有附件下载地址无效")
		}
		request.Header.Set("Authorization", "Bearer "+u.token)
		request.Header.Set("Accept", "application/octet-stream")
	}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return "", fmt.Errorf("更新下载返回 HTTP %d", response.StatusCode)
	}
	if response.ContentLength >= 0 && response.ContentLength != u.asset.Size {
		return "", fmt.Errorf("更新包大小不匹配")
	}
	file, err := os.CreateTemp(directory, "download-")
	if err != nil {
		return "", err
	}
	defer func() {
		file.Close()
		if err != nil {
			os.Remove(file.Name())
		}
	}()
	hash := sha256.New()
	reader := io.LimitReader(response.Body, u.asset.Size+1)
	var count int64
	buffer := make([]byte, 64<<10)
	last := time.Time{}
	for {
		if err = ctx.Err(); err != nil {
			return "", err
		}
		var n int
		n, err = reader.Read(buffer)
		if n > 0 {
			count += int64(n)
			if count > u.asset.Size {
				return "", fmt.Errorf("更新包超过声明大小")
			}
			if _, e := file.Write(buffer[:n]); e != nil {
				return "", e
			}
			hash.Write(buffer[:n])
			if progress != nil && time.Since(last) > 100*time.Millisecond {
				progress(count, u.asset.Size)
				last = time.Now()
			}
		}
		if err == io.EOF {
			err = nil
			break
		}
		if err != nil {
			return "", err
		}
	}
	if count != u.asset.Size || !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), strings.TrimPrefix(u.asset.Digest, "sha256:")) {
		return "", fmt.Errorf("更新包不完整或 SHA-256 校验失败")
	}
	signature, _ := base64.StdEncoding.DecodeString(u.verified.archive.Signature)
	if !ed25519.Verify(u.verified.publicKey, hash.Sum(nil), signature) {
		return "", fmt.Errorf("安装包签名验证失败")
	}
	if err = file.Sync(); err != nil {
		return "", err
	}
	if err = file.Close(); err != nil {
		return "", err
	}
	return file.Name(), nil
}

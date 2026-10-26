package core

import (
	"context"
	"crypto/ed25519"
	"encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (u *ReleaseUpdate) verifyAuthority() error {
	if u == nil || u.verified == nil || len(u.verified.publicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("更新尚未通过签名验证")
	}
	v := u.verified
	if u.Version != v.version || u.target != v.target || u.asset.URL != v.archive.URL || u.asset.Size != v.archive.Size || u.asset.Digest != "sha256:"+v.archive.SHA256 {
		return fmt.Errorf("更新附件与签名信息不一致")
	}
	return nil
}

func portableSignedCandidate(v *VerifiedUpdate) *ReleaseUpdate {
	if v == nil {
		return nil
	}
	return &ReleaseUpdate{verified: v, Version: v.version, Notes: v.Manifest.Notes, target: v.target,
		asset: releaseAsset{URL: v.archive.URL, Size: v.archive.Size, Digest: "sha256:" + v.archive.SHA256}}
}

// The same signed envelope is used on Pages and, for the existing optional
// private repository transport, as a latest.json release attachment.
type privateSignedMetadata struct {
	Release githubRelease
	Signed  []byte
}

func checkSignedPortableUpdate(ctx context.Context, cache *UpdateMetadataCache, source UpdateSource, target, current, build, key, token string) (*ReleaseUpdate, error) {
	if releaseAssetName(target) == "" {
		return nil, fmt.Errorf("当前系统或架构不支持自更新")
	}
	if token == "" {
		v, err := CheckSignedUpdate(ctx, cache, source, target, "dev.oshell.desktop", current, build, key)
		if err != nil {
			return nil, err
		}
		return portableSignedCandidate(v), nil
	}
	if _, err := ParseUpdatePublicKey(key); err != nil {
		return nil, err
	}
	decode := func(data []byte) (*ReleaseUpdate, error) {
		var m privateSignedMetadata
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("私有更新元数据无效")
		}
		v, err := DecodeSignedUpdate(m.Signed, source, target, "dev.oshell.desktop", current, build, key)
		if err != nil || v == nil {
			return nil, err
		}
		if m.Release.Draft || m.Release.Prerelease || m.Release.Tag != "v"+v.version {
			return nil, fmt.Errorf("Release 与签名版本不匹配")
		}
		var asset *releaseAsset
		for _, a := range m.Release.Assets {
			if a.Name != releaseAssetName(target) {
				continue
			}
			if asset != nil || a.ID <= 0 || a.State != "uploaded" || a.URL != v.archive.URL || a.Size != v.archive.Size || a.Digest != "sha256:"+v.archive.SHA256 {
				return nil, fmt.Errorf("Release 附件与签名信息不匹配")
			}
			copy := a
			asset = &copy
		}
		if asset == nil {
			return nil, fmt.Errorf("Release 缺少签名指定的安装包")
		}
		u := portableSignedCandidate(v)
		u.token, u.assetAPIURL = token, "https://api.github.com/repos/"+source.Repository+"/releases/assets/"+strconv.FormatInt(asset.ID, 10)
		return u, nil
	}
	data, err := cache.Value(ctx, func(ctx context.Context) ([]byte, error) {
		data, err := fetchPrivateUpdateMetadata(ctx, source, token)
		if err == nil {
			_, err = decode(data)
		}
		return data, err
	})
	if err != nil {
		return nil, err
	}
	return decode(data)
}

func fetchPrivateUpdateMetadata(ctx context.Context, source UpdateSource, token string) ([]byte, error) {
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	get := func(url, accept string) ([]byte, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", accept)
		req.Header.Set("User-Agent", "OShell Desktop")
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("无法连接私有更新源")
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return nil, &UpdateHTTPError{Status: resp.StatusCode}
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, UpdateMetadataLimit+1))
		if err != nil {
			return nil, fmt.Errorf("读取私有更新信息失败")
		}
		if len(data) > UpdateMetadataLimit {
			return nil, fmt.Errorf("更新信息超过大小限制")
		}
		return data, nil
	}
	base := "https://api.github.com/repos/" + source.Repository + "/releases/"
	data, err := get(base+"latest", "application/vnd.github+json")
	if err != nil {
		return nil, err
	}
	var release githubRelease
	if json.Unmarshal(data, &release) != nil || release.Draft || release.Prerelease {
		return nil, fmt.Errorf("私有 Release 信息无效")
	}
	var id int64
	for _, a := range release.Assets {
		if a.Name != "latest.json" {
			continue
		}
		if id != 0 || a.ID <= 0 || a.State != "uploaded" || a.Size <= 0 || a.Size > UpdateMetadataLimit || !strings.EqualFold(a.URL, "https://github.com/"+source.Repository+"/releases/download/"+release.Tag+"/latest.json") {
			return nil, fmt.Errorf("签名元数据附件无效或重复")
		}
		id = a.ID
	}
	if id == 0 {
		return nil, fmt.Errorf("私有 Release 缺少已签名的 latest.json，拒绝未签名更新")
	}
	signed, err := get(base+"assets/"+strconv.FormatInt(id, 10), "application/octet-stream")
	if err != nil {
		return nil, err
	}
	return json.Marshal(privateSignedMetadata{Release: release, Signed: signed})
}

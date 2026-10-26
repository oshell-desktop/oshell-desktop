// SPDX-License-Identifier: GPL-3.0-only
package core

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

type UpdateSource struct{ Repository string }

var updateOwner = regexp.MustCompile(`^[A-Za-z0-9]+(?:-[A-Za-z0-9]+)*$`)
var updateRepo = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

func ParseUpdateSource(input string) (UpdateSource, error) {
	value := strings.TrimSpace(input)
	if strings.HasPrefix(value, "https://") {
		u, e := url.Parse(value)
		if e != nil || u.Scheme != "https" || !strings.EqualFold(u.Host, "github.com") || u.User != nil || u.RawQuery != "" || u.ForceQuery || strings.Contains(value, "#") {
			return UpdateSource{}, fmt.Errorf("请输入公开 GitHub 仓库 HTTPS 地址")
		}
		value = strings.Trim(u.Path, "/")
	}
	value = strings.TrimSuffix(value, ".git")
	parts := strings.Split(value, "/")
	if len(parts) != 2 || len(parts[0]) > 39 || len(parts[1]) > 100 || !updateOwner.MatchString(parts[0]) || !updateRepo.MatchString(parts[1]) || parts[1] == "." || parts[1] == ".." {
		return UpdateSource{}, fmt.Errorf("仓库格式应为 owner/repo，或对应 GitHub HTTPS 地址")
	}
	return UpdateSource{Repository: value}, nil
}
func (s UpdateSource) MetadataURL() string {
	owner, name, _ := strings.Cut(s.Repository, "/")
	owner = strings.ToLower(owner)
	prefix := "/" + name
	if strings.EqualFold(name, owner+".github.io") {
		prefix = ""
	}
	return "https://" + owner + ".github.io" + prefix + "/updates/latest.json"
}
func (s UpdateSource) acceptsArchive(raw, version, target string) bool {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || !strings.EqualFold(u.Host, "github.com") || u.User != nil || u.RawQuery != "" || u.ForceQuery || strings.Contains(raw, "#") {
		return false
	}
	// Desktop updates have a separate product namespace; an OShell DMG cannot
	// pass this gate, even if both apps happen to use the same release repository.
	filename := "OShell-Desktop-" + version + "-" + target + ".tar.gz"
	if target == "windows-amd64" || target == "linux-amd64" {
		filename = releaseAssetName(target)
	}
	path := "/" + s.Repository + "/releases/download/v" + version + "/" + filename
	return strings.HasSuffix(u.EscapedPath(), "/"+filename) && strings.EqualFold(strings.TrimSuffix(u.EscapedPath(), filename), strings.TrimSuffix(path, filename))
}

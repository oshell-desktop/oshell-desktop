package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/font/opentype/tables"
)

const MaxFontFile = 20 << 20
const MaxFontLibrary = 64 << 20
const MaxFontFiles = 64
const MaxFontScan = 10000

type FontAsset struct {
	SHA256    string `json:"sha256"`
	Extension string `json:"extension"`
	Size      int    `json:"size"`
}

func (a FontAsset) Name() string { return a.SHA256 + "." + a.Extension }

type FontManifest struct {
	Format  string      `json:"format"`
	Version int         `json:"version"`
	Fonts   []FontAsset `json:"fonts"`
}
type LibraryFont struct {
	Asset FontAsset
	Faces []FontEntry
	Data  []byte
}
type FontLibrary struct{ Directory string }
type FontLibraryDraft struct {
	Before FontManifest
	Fonts  []LibraryFont
}

var fontLibraryWrite sync.Mutex

func NewFontLibrary(dataDirectory string) FontLibrary {
	return FontLibrary{filepath.Join(dataDirectory, "fonts")}
}
func fontExtension(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ttf", ".otf", ".ttc", ".otc":
		return true
	}
	return false
}
func (l FontLibrary) CheckDirectory() error {
	info, err := os.Lstat(l.Directory)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("字体库必须是本机普通目录")
	}
	return nil
}
func readFontFile(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, fmt.Errorf("字体文件必须是普通文件，且不能超过大小限制")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("字体文件超过大小限制")
	}
	return data, nil
}
func (m FontManifest) validate() error {
	if m.Format != "OShell.font-library" || m.Version != 1 || m.Fonts == nil || len(m.Fonts) > MaxFontFiles {
		return fmt.Errorf("字体清单格式、版本或数量无效")
	}
	total := 0
	seen := map[string]bool{}
	for _, a := range m.Fonts {
		_, err := hex.DecodeString(a.SHA256)
		if err != nil || len(a.SHA256) != 64 || a.SHA256 != strings.ToLower(a.SHA256) || !fontExtension("x."+a.Extension) || strings.ContainsAny(a.Extension, "./\\") || a.Extension != strings.ToLower(a.Extension) || a.Size < 12 || a.Size > MaxFontFile || seen[a.Name()] {
			return fmt.Errorf("字体清单含无效或重复项目")
		}
		seen[a.Name()] = true
		total += a.Size
	}
	if total > MaxFontLibrary {
		return fmt.Errorf("活动字体库不能超过 64 MiB")
	}
	return nil
}
func (l FontLibrary) Manifest() (FontManifest, error) {
	m := FontManifest{Format: "OShell.font-library", Version: 1, Fonts: []FontAsset{}}
	if err := l.CheckDirectory(); err != nil {
		return m, err
	}
	data, err := readFontFile(filepath.Join(l.Directory, "library.json"), 65536)
	if os.IsNotExist(err) {
		return m, nil
	}
	if err != nil {
		return m, err
	}
	return DecodeFontManifest(data)
}

// DecodeFontManifest is the boundary for local and remote font manifests.
func DecodeFontManifest(data []byte) (FontManifest, error) {
	var m FontManifest
	var root map[string]json.RawMessage
	if len(data) > 65536 || json.Unmarshal(data, &root) != nil || len(root) != 3 || root["format"] == nil || root["version"] == nil || root["fonts"] == nil {
		return m, fmt.Errorf("字体清单含未知字段或格式错误")
	}
	var assets []map[string]json.RawMessage
	if json.Unmarshal(root["fonts"], &assets) != nil || assets == nil {
		return m, fmt.Errorf("字体清单格式错误")
	}
	for _, a := range assets {
		if len(a) != 3 || a["sha256"] == nil || a["extension"] == nil || a["size"] == nil {
			return m, fmt.Errorf("字体清单含未知字段或格式错误")
		}
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	// Decode into a zero value so absent required fields cannot become defaults.
	var decoded FontManifest
	if err := d.Decode(&decoded); err != nil {
		return m, fmt.Errorf("字体清单字段类型无效")
	}
	if d.Decode(new(any)) != io.EOF {
		return m, fmt.Errorf("字体清单包含多余数据")
	}
	if err := decoded.validate(); err != nil {
		return m, err
	}
	sort.Slice(decoded.Fonts, func(i, j int) bool { return decoded.Fonts[i].Name() < decoded.Fonts[j].Name() })
	return decoded, nil
}
func FontFaces(data []byte) ([]FontEntry, error) {
	if len(data) < 12 || len(data) > MaxFontFile {
		return nil, fmt.Errorf("字体大小无效（单文件最多 20 MiB）")
	}
	magic := string(data[:4])
	if magic != "\x00\x01\x00\x00" && magic != "OTTO" && magic != "ttcf" && magic != "true" {
		return nil, fmt.Errorf("请选择 TTF / OTF / TTC / OTC 字体")
	}
	if magic == "ttcf" {
		n := binary.BigEndian.Uint32(data[8:12])
		if n == 0 || n > 128 {
			return nil, fmt.Errorf("字体集合须包含 1–128 个字面")
		}
	}
	faces, err := font.ParseTTC(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("字体无法解析：%w", err)
	}
	if len(faces) == 0 || len(faces) > 128 {
		return nil, fmt.Errorf("字体集合须包含 1–128 个字面")
	}
	entries := []FontEntry{}
	seen := map[string]bool{}
	loaders, err := opentype.NewLoaders(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	for i, face := range faces {
		raw, err := loaders[i].RawTable(opentype.MustNewTag("name"))
		if err != nil {
			return nil, err
		}
		names, _, err := tables.ParseName(raw)
		if err != nil {
			return nil, err
		}
		name := strings.TrimSpace(names.Name(6))
		if name == "" || len(name) > 512 || strings.ContainsAny(name, ",") || strings.ContainsFunc(name, unicode.IsControl) {
			return nil, fmt.Errorf("字体 PostScript 名称无效")
		}
		if !seen[name] {
			entries = append(entries, FontEntry{Name: name, Monospaced: face.IsMonospace()})
			seen[name] = true
		}
	}
	return entries, nil
}
func (l FontLibrary) Load() (FontLibraryDraft, error) {
	m, err := l.Manifest()
	if err != nil {
		return FontLibraryDraft{}, err
	}
	draft := FontLibraryDraft{Before: m}
	for _, asset := range m.Fonts {
		data, err := readFontFile(filepath.Join(l.Directory, asset.Name()), MaxFontFile)
		if err != nil {
			return draft, err
		}
		sum := sha256.Sum256(data)
		if len(data) != asset.Size || hex.EncodeToString(sum[:]) != asset.SHA256 {
			return draft, fmt.Errorf("字体文件大小或摘要不匹配：%s", asset.Name())
		}
		faces, err := FontFaces(data)
		if err != nil {
			return draft, err
		}
		draft.Fonts = append(draft.Fonts, LibraryFont{asset, faces, data})
	}
	return draft, nil
}

// Read directory batches so the 10,000-entry limit also bounds enumeration memory.
func SelectedFontFiles(ctx context.Context, selections []string) ([]string, error) {
	var files []string
	seen := map[string]bool{}
	var inspect func(string, bool) error
	inspect = func(path string, explicit bool) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		path, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		if seen[path] {
			return nil
		}
		if len(seen) >= MaxFontScan {
			return fmt.Errorf("目录扫描超过 10000 项，请缩小范围")
		}
		seen[path] = true
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		packageDir := info.IsDir() && (strings.HasSuffix(strings.ToLower(path), ".app") || strings.HasSuffix(strings.ToLower(path), ".bundle") || strings.HasSuffix(strings.ToLower(path), ".framework") || strings.HasSuffix(strings.ToLower(path), ".plugin"))
		if info.Mode()&os.ModeSymlink != 0 || packageDir {
			if explicit {
				return fmt.Errorf("请选择普通字体文件或目录，不支持符号链接或应用包")
			}
			return nil
		}
		if !explicit && (strings.HasPrefix(info.Name(), ".") || fontFileHidden(info)) {
			return nil
		}
		if info.IsDir() {
			dir, err := os.Open(path)
			if err != nil {
				return err
			}
			defer dir.Close()
			for {
				entries, err := dir.ReadDir(128)
				if err != nil && err != io.EOF {
					return err
				}
				for _, entry := range entries {
					if err := inspect(filepath.Join(path, entry.Name()), false); err != nil {
						return err
					}
				}
				if err == io.EOF {
					return nil
				}
			}
		}
		if info.Mode().IsRegular() && fontExtension(path) {
			if len(files) >= MaxFontFiles {
				return fmt.Errorf("每次最多导入 64 个字体文件")
			}
			files = append(files, path)
		} else if explicit {
			return fmt.Errorf("请选择字体目录或 TTF / OTF / TTC / OTC 文件")
		}
		return nil
	}
	for _, path := range selections {
		if err := inspect(path, true); err != nil {
			return nil, err
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("所选目录中没有可导入字体")
	}
	sort.Strings(files)
	return files, nil
}
func (l FontLibrary) PrepareImport(ctx context.Context, selections []string) (FontLibraryDraft, error) {
	draft, err := l.Load()
	if err != nil {
		return draft, err
	}
	files, err := SelectedFontFiles(ctx, selections)
	if err != nil {
		return draft, err
	}
	seen := map[string]bool{}
	total := 0
	for _, f := range draft.Fonts {
		seen[f.Asset.Name()] = true
		total += f.Asset.Size
	}
	for _, path := range files {
		if err := ctx.Err(); err != nil {
			return draft, err
		}
		data, err := readFontFile(path, MaxFontFile)
		if err != nil {
			return draft, err
		}
		sum := sha256.Sum256(data)
		asset := FontAsset{hex.EncodeToString(sum[:]), strings.ToLower(filepath.Ext(path)[1:]), len(data)}
		if seen[asset.Name()] {
			continue
		}
		if len(draft.Fonts) >= MaxFontFiles || total+len(data) > MaxFontLibrary {
			return draft, fmt.Errorf("活动字体库最多 64 个文件、合计 64 MiB")
		}
		faces, err := FontFaces(data)
		if err != nil {
			return draft, err
		}
		draft.Fonts = append(draft.Fonts, LibraryFont{asset, faces, data})
		seen[asset.Name()] = true
		total += len(data)
	}
	sort.Slice(draft.Fonts, func(i, j int) bool { return draft.Fonts[i].Asset.Name() < draft.Fonts[j].Asset.Name() })
	return draft, nil
}
func (l FontLibrary) Commit(draft FontLibraryDraft) error {
	fontLibraryWrite.Lock()
	defer fontLibraryWrite.Unlock()
	before, err := l.Manifest()
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(before, draft.Before) {
		return fmt.Errorf("字体库已变化，请刷新后重试")
	}
	next := FontManifest{Format: "OShell.font-library", Version: 1, Fonts: []FontAsset{}}
	for _, f := range draft.Fonts {
		next.Fonts = append(next.Fonts, f.Asset)
	}
	if err := next.validate(); err != nil {
		return err
	}
	sort.Slice(next.Fonts, func(i, j int) bool { return next.Fonts[i].Name() < next.Fonts[j].Name() })
	if reflect.DeepEqual(before, next) {
		return nil
	}
	if err := os.MkdirAll(l.Directory, 0700); err != nil {
		return err
	}
	for _, f := range draft.Fonts {
		path := filepath.Join(l.Directory, f.Asset.Name())
		data, err := readFontFile(path, MaxFontFile)
		if os.IsNotExist(err) {
			if err := AtomicWrite(path, f.Data); err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if !bytes.Equal(data, f.Data) {
			return fmt.Errorf("已有字体文件已改变，请恢复字体库后重试")
		}
	}
	old, _ := json.Marshal(before)
	sum := sha256.Sum256(old)
	backups := FontLibrary{filepath.Join(l.Directory, "backups")}
	if err := backups.CheckDirectory(); err != nil {
		return err
	}
	if err := os.MkdirAll(backups.Directory, 0700); err != nil {
		return err
	}
	if err := AtomicWrite(filepath.Join(backups.Directory, hex.EncodeToString(sum[:])+".json"), old); err != nil {
		return err
	}
	data, _ := json.Marshal(next)
	return AtomicWrite(filepath.Join(l.Directory, "library.json"), data)
}

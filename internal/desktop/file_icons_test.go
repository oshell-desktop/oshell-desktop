package desktop

import (
	"github.com/egoist/mygo/ui"
	"os"
	"oshell-desktop/internal/core"
	"strings"
	"testing"
	"time"
)

type iconFileInfo struct {
	name string
	mode os.FileMode
}

func (f iconFileInfo) Name() string       { return f.name }
func (f iconFileInfo) Size() int64        { return 2048 }
func (f iconFileInfo) Mode() os.FileMode  { return f.mode }
func (f iconFileInfo) ModTime() time.Time { return time.Date(2026, 10, 13, 10, 0, 0, 0, time.Local) }
func (f iconFileInfo) IsDir() bool        { return f.mode.IsDir() }
func (f iconFileInfo) Sys() any           { return nil }
func TestFileIconsRespectMetadataBeforeExtension(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode os.FileMode
		kind string
	}{
		{"assets.png", os.ModeDir | 0755, "目录"}, {"shortcut.zip", os.ModeSymlink | os.ModeDir | 0777, "链接"},
		{"ARCHIVE.TAR.GZ", 0644, "压缩包"}, {"photo.PNG", 0644, "图片"}, {"script.sh", 0755, "脚本"},
		{".env", 0600, "配置"}, {"report.pdf", 0644, "PDF"}, {"binary", 0755, "可执行"}, {"unknown", 0644, "文件"}, {"socket", os.ModeSocket, "特殊文件"},
		{"CMakeLists.txt", 0644, "代码"}, {".env.production", 0600, "配置"}, {"KNOWN_HOSTS", 0600, "配置"},
		{"README", 0644, "文本"}, {"report.csv", 0644, "表格"}, {"slides.pptx", 0644, "演示文稿"},
		{"app.rpm", 0644, "安装包"}, {"font.woff2", 0644, "字体"}, {"key.pem", 0600, "证书 / 密钥"}, {"state.sqlite3", 0600, "数据库"},
	} {
		style := presentFile(iconFileInfo{tc.name, tc.mode})
		if style.kind != tc.kind || style.icon == nil {
			t.Fatalf("%s: %s", tc.name, style.kind)
		}
	}
	hint := fileAttributeHint(iconFileInfo{".script.sh", 0755}, "代码")
	for _, part := range []string{"隐藏项目", "具有执行权限", "-rwxr-xr-x", "2048 字节"} {
		if !strings.Contains(hint, part) {
			t.Fatal("attribute missing", part, hint)
		}
	}
}
func TestFileIconsRenderAndKeepSelection(t *testing.T) {
	a := newApp(&core.Store{Dir: t.TempDir()}, core.DefaultConfig())
	f := &fileState{dir: "/home/develop", input: "/home/develop", entries: []os.FileInfo{
		iconFileInfo{"assets", os.ModeDir | 0755}, iconFileInfo{"latest", os.ModeSymlink | 0777}, iconFileInfo{"backup.tar.gz", 0644}, iconFileInfo{"photo.png", 0644}, iconFileInfo{"main.go", 0644}, iconFileInfo{".env", 0600}, iconFileInfo{"notes.md", 0644}, iconFileInfo{"deploy", 0755}, iconFileInfo{"recording.mp4", 0644}, iconFileInfo{"music.flac", 0644},
	}}
	p := &pane{profile: core.Profile{Kind: "ftp"}, ftp: &core.FTPSession{}, files: f}
	tt := ui.NewTester(func(c *ui.Context) { a.theme(c); a.fileBrowser(c, p, true) }, 900, 660)
	if err := tt.Click("assets"); err != nil {
		t.Fatal(err)
	}
	if !f.isSelected("assets") {
		t.Fatal("icon changed row selection")
	}
	snapshot(t, tt, "file-type-icons.png")
	for _, kind := range []string{"目录", "链接", "压缩包", "图片", "代码", "配置", "可执行"} {
		if !tt.HasText(kind) {
			t.Fatal("missing file type", kind)
		}
	}
}

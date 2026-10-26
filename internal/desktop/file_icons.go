package desktop

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"os"
	"oshell-desktop/internal/core"
	"path"
	"strings"
)

// Parsed once; classification uses directory metadata, never extra remote reads.
var (
	documentFileIcon = glyph(`<path d="M4 2.5h8l4 4v11H4zM12 2.5v4h4"/><path d="M7 10h6M7 13h6"/>`)
	archiveFileIcon  = glyph(`<rect x="3" y="3" width="14" height="14" rx="2"/><path d="M9 3h2v3H9v3h2v3H9v3h2"/>`)
	imageFileIcon    = glyph(`<rect x="2.5" y="3" width="15" height="14" rx="2"/><circle cx="7" cy="7" r="1.5"/><path d="m3 15 4-4 3 2 3-5 4 7"/>`)
	codeFileIcon     = glyph(`<path d="M4 2.5h8l4 4v11H4zM12 2.5v4h4M8 10l-2 2 2 2m4-4 2 2-2 2"/>`)
	mediaFileIcon    = glyph(`<rect x="3" y="3" width="14" height="14" rx="2"/><path d="m8 6 6 4-6 4z"/>`)
	audioFileIcon    = glyph(`<path d="M8 14V5l8-2v9M8 7l8-2"/><ellipse cx="5.5" cy="14.5" rx="2.5" ry="2"/><ellipse cx="13.5" cy="12.5" rx="2.5" ry="2"/>`)
)

type filePresentation struct {
	kind  string
	icon  *ui.SVG
	color ui.Color
}

func presentFile(entry os.FileInfo) filePresentation {
	mode := entry.Mode()
	makeStyle := func(kind string, icon *ui.SVG, color string) filePresentation {
		return filePresentation{kind, icon, ui.Hex(color)}
	}
	// A link to a directory must remain visibly a link.
	if mode&os.ModeSymlink != 0 {
		return makeStyle("链接", linkIcon, "#8B78C6")
	}
	if !core.RemoteFileTypeKnown(entry) {
		return makeStyle("未知", documentFileIcon, "#7B91A6")
	}
	if entry.IsDir() {
		return makeStyle("目录", folderIcon, "#428BC1")
	}
	if !mode.IsRegular() {
		return makeStyle("特殊文件", settingsIcon, "#8A8196")
	}
	name := strings.ToLower(entry.Name())
	kind := ""
	switch name {
	case "dockerfile", "makefile", "gnumakefile", "cmakelists.txt", "jenkinsfile", "vagrantfile":
		kind = "代码"
	case ".bashrc", ".zshrc", ".profile", ".bash_profile", ".zprofile":
		kind = "脚本"
	case ".env", ".gitignore", ".gitattributes", ".editorconfig", ".npmrc", ".vimrc", "config", "known_hosts", "authorized_keys":
		kind = "配置"
	case "readme", "license", "licence", "copying", "changelog", "authors", "notice":
		kind = "文本"
	}
	if strings.HasPrefix(name, ".env.") {
		kind = "配置"
	}
	if kind == "" {
		kind = fileExtensionKinds[strings.ToLower(path.Ext(name))]
	}
	if style, ok := fileKindStyles[kind]; ok {
		return style
	}
	if mode.Perm()&0111 != 0 {
		return makeStyle("可执行", terminalIcon, "#509975")
	}
	return makeStyle("文件", documentFileIcon, "#7B91A6")
}
func fileAttributeHint(entry os.FileInfo, kind string) string {
	attributes := core.RemoteFileAttributes(entry)
	permission := attributes.Permissions
	if attributes.Octal != "—" {
		permission += "（" + attributes.Octal + "）"
	}
	text := entry.Name() + "\n类型：" + kind + "\n权限：" + permission + "\n所属用户 UID：" + attributes.User + "\n所属组 GID：" + attributes.Group
	if strings.HasPrefix(entry.Name(), ".") {
		text += "\n隐藏项目"
	}
	if entry.Mode().IsRegular() && entry.Mode().Perm()&0111 != 0 {
		text += "\n具有执行权限"
	}
	if !core.RemoteFileTypeKnown(entry) {
		text += "\n服务器未提供类型和大小；可尝试进入目录"
	} else if !entry.IsDir() {
		text += fmt.Sprintf("\n大小：%s（%d 字节）", bytesLabel(entry.Size()), entry.Size())
	}
	return text + "\n修改时间：" + fileModifiedDetail(entry)
}

var fileKindStyles = map[string]filePresentation{
	"文本": {"文本", documentFileIcon, ui.Hex("#7B91A6")}, "代码": {"代码", codeFileIcon, ui.Hex("#428BC1")},
	"脚本": {"脚本", terminalIcon, ui.Hex("#509975")}, "配置": {"配置", settingsIcon, ui.Hex("#8B78C6")},
	"压缩包": {"压缩包", archiveFileIcon, ui.Hex("#B58132")}, "图片": {"图片", imageFileIcon, ui.Hex("#8B78C6")},
	"音频": {"音频", audioFileIcon, ui.Hex("#C06A60")}, "视频": {"视频", mediaFileIcon, ui.Hex("#8B78C6")},
	"PDF": {"PDF", documentFileIcon, ui.Hex("#C06A60")}, "文档": {"文档", documentFileIcon, ui.Hex("#428BC1")},
	"表格": {"表格", documentFileIcon, ui.Hex("#509975")}, "演示文稿": {"演示文稿", documentFileIcon, ui.Hex("#B58132")},
	"数据库": {"数据库", settingsIcon, ui.Hex("#509975")}, "安装包": {"安装包", archiveFileIcon, ui.Hex("#B58132")},
	"字体": {"字体", documentFileIcon, ui.Hex("#428BC1")}, "证书 / 密钥": {"证书 / 密钥", settingsIcon, ui.Hex("#B58132")},
}
var fileExtensionKinds = func() map[string]string {
	out := map[string]string{}
	for kind, extensions := range map[string]string{
		"文本":  "txt log md markdown rst rtf",
		"代码":  "swift c h cc cpp hpp m mm rs go java kt scala js jsx ts tsx vue html htm css scss sql",
		"脚本":  "sh bash zsh fish py rb pl lua php ps1 bat cmd",
		"配置":  "json jsonc yaml yml toml ini conf config cfg xml plist properties service env",
		"压缩包": "zip tar gz tgz bz2 tbz2 xz txz zst 7z rar lz lz4 jar war",
		"图片":  "png jpg jpeg gif webp svg ico icns heic tif tiff bmp avif",
		"音频":  "mp3 wav flac aac m4a ogg opus aiff", "视频": "mp4 mov mkv avi webm m4v mpeg",
		"PDF": "pdf", "文档": "doc docx odt pages", "表格": "xls xlsx csv tsv ods numbers", "演示文稿": "ppt pptx odp",
		"数据库": "db sqlite sqlite3 mdb", "安装包": "dmg pkg deb rpm msi exe apk iso", "字体": "ttf otf woff woff2", "证书 / 密钥": "pem crt cer key p12 pfx pub",
	} {
		for _, ext := range strings.Fields(extensions) {
			out["."+ext] = kind
		}
	}
	return out
}()

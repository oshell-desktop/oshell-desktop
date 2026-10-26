package desktop

import (
	"path"
	"strings"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

// Presentation only: shared session types and configuration stay unchanged.
var (
	sessionLocalIcon  = glyph(`<rect x="4" y="3" width="12" height="10" rx="1.5"/><path d="m4 13-2 4h16l-2-4"/>`)
	sessionFolderIcon = glyph(`<path d="M2 17V3h6l2 2h8v12z"/>`)
	sessionParentIcon = glyph(`<path d="M16 15V9H4m4-4L4 9l4 4"/>`)
)

type sessionIconStyle struct {
	image              *ui.SVG
	label, light, dark string
	badge              string
	shortcut           bool
}

func sessionIconFor(kind string, shortcut, folder, parent bool) sessionIconStyle {
	if parent {
		return sessionIconStyle{image: sessionParentIcon, label: "上一级目录", light: "#666B75", dark: "#B3B8C2"}
	}
	if folder {
		return sessionIconStyle{shortcut: shortcut, image: sessionFolderIcon, label: "目录", light: "#9C6E1F", dark: "#E8BA5C"}
	}
	style := sessionIconStyle{badge: "SSH", label: "SSH · 远程终端", light: "#1F66BF", dark: "#80B5FF"}
	switch kind {
	case "local":
		style = sessionIconStyle{image: sessionLocalIcon, label: "本地终端", light: "#8252B8", dark: "#C29CF7"}
	case "sftp":
		style = sessionIconStyle{badge: "SFTP", label: "SFTP · 安全文件传输", light: "#1B7C50", dark: "#70D6A5"}
	case "ftp":
		style = sessionIconStyle{badge: "FTP", label: "FTP · 文件传输", light: "#A95312", dark: "#FFB573"}
	}
	if shortcut {
		style.shortcut = true
		style.label = "Link · 快捷引用 · " + style.label
	}
	return style
}

func (s sessionIconStyle) draw(c *ui.Context, selected bool) {
	color := ui.Hex(s.light)
	if c.Theme().Dark {
		color = ui.Hex(s.dark)
	}
	corner := func() {
		if s.shortcut {
			ui.Box(c).Absolute().Right(-3).Bottom(-3).Size(12, 12).Radius(3).Background(c.Theme().Background).Children(func() { ui.Icon(c, linkIcon).Size(12, 12).FontSize(12).TextColor(ui.Hex("#168B9E")) })
		}
	}
	if s.badge != "" {
		// An opaque theme base keeps the badge legible on selected rows too.
		ui.Box(c).Size(36, 20).Shrink(0).Radius(5).Background(c.Theme().Background).Label(s.label).Children(func() {
			ui.Row(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Radius(5).
				Background(color.Alpha(0.10)).Border(1, color.Alpha(0.30)).Children(func() {
				ui.Text(c, s.badge).FontSize(10).FontWeight(600).SingleLine().TextColor(color)
			})
			corner()
		})
		return
	}
	if selected {
		color = c.Theme().AccentText
	}
	ui.Row(c).Size(36, 20).Shrink(0).AlignItems(ui.Center).Justify(ui.Center).Label(s.label).Children(func() {
		ui.Icon(c, s.image).FontSize(20).Size(20, 20).Shrink(0).TextColor(color)
		corner()
	})
}

func sessionListTooltip(p core.Profile, shortcut bool) string {
	text := sessionIconFor(p.Kind, shortcut, false, false).label
	if shortcut {
		text += "\n来源：" + path.Join("/", p.Group, p.Name)
	}
	return text + "\n" + core.SessionSearchMetadata(p)
}

func sessionTreeItem(c *ui.Context, name string, open *bool, children func(), selected bool, style sessionIconStyle) ui.Element {
	// Keep the toolkit's arrow, keyboard navigation and tree accessibility;
	// append the shared icon/name presentation after its empty label.
	return ui.TreeItem(c, "", open, children).Label(name).Selected(selected).MinWidth(0).Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).Gap(7).AlignItems(ui.Center).Children(func() {
			style.draw(c, selected)
			ui.Text(c, name).SingleLine().Grow(1).MinWidth(0)
		})
	})
}

func sidebarSessionStyle(e sidebarEntry) sessionIconStyle {
	return sessionIconFor(e.profile.Kind, strings.HasPrefix(e.id, "link:"), false, false)
}

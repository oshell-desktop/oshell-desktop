package desktop

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"strings"
)

type quickConnectionNode struct {
	label, id, kind, linkID string
	linksFolder             bool
	children                []*quickConnectionNode
	order                   sessionSortKey
}

func quickConnectionTree(profiles []core.Profile, pins map[string]bool, links ...core.SessionLink) []*quickConnectionNode {
	root := &quickConnectionNode{}
	type entry struct {
		profile core.Profile
		linkID  string
	}
	var entries []entry
	byID := map[string]core.Profile{}
	for _, p := range profiles {
		byID[p.ID] = p
		if p.QuickConnect {
			entries = append(entries, entry{profile: p})
		}
	}
	for _, link := range links {
		if p, ok := byID[link.ProfileID]; ok {
			p.Name = link.Name
			p.Group = "Links"
			if link.Folder != "" {
				p.Group += "/" + link.Folder
			}
			entries = append(entries, entry{profile: p, linkID: link.ID})
		}
	}
	for _, e := range entries {
		p := e.profile
		node := root
		for _, part := range strings.Split(strings.Trim(p.Group, "/"), "/") {
			if part == "" {
				continue
			}
			var child *quickConnectionNode
			for _, n := range node.children {
				if n.id == "" && n.label == part {
					child = n
					break
				}
			}
			if child == nil {
				pinned := node == root && pins["d:"+part]
				child = &quickConnectionNode{label: part, linksFolder: node == root && part == "Links", order: sessionSortKey{name: part, folder: true, pinned: pinned}}
				node.children = append(node.children, child)
			}
			node = child
		}
		pinned := p.Group == "" && pins[pinKey(sessionRow{profile: p})]
		node.children = append(node.children, &quickConnectionNode{
			label: p.Name, id: p.ID, kind: p.Kind, linkID: e.linkID,
			order: sessionSortKey{name: p.Name, pinned: pinned},
		})
	}
	var arrange func(*quickConnectionNode)
	arrange = func(node *quickConnectionNode) {
		sortSessionList(node.children, func(n *quickConnectionNode) sessionSortKey { return n.order })
		for _, n := range node.children {
			arrange(n)
		}
	}
	arrange(root)
	return root.children
}
func (a *App) connectQuickID(id string, reference ...string) {
	p, exists := a.profileByID(id)
	allowed := p.QuickConnect
	if len(reference) > 0 && reference[0] != "" {
		allowed = false
		for _, link := range a.config.Links {
			if link.ID == reference[0] && link.ProfileID == id {
				allowed = true
				break
			}
		}
	}
	if !exists || !allowed {
		a.status = "快捷会话已被删除或停用，请重新选择"
		return
	}
	a.request(p, 0)
}
func (n *quickConnectionNode) icon(dark bool) *ui.MenuIcon {
	return sessionIconFor(n.kind, n.linkID != "" || n.linksFolder, n.id == "", false).menuIcon(dark)
}

func (a *App) populateQuickConnections(menu *ui.Menu, nodes []*quickConnectionNode, dark bool) {
	for _, n := range nodes {
		if n.id == "" {
			menu.Submenu(n.label, func(child *ui.Menu) { a.populateQuickConnections(child, n.children, dark) }).Icon(n.icon(dark))
		} else if menu.ItemWithID(n.id+":"+n.linkID, n.label).Icon(n.icon(dark)).Chosen() {
			a.connectQuickID(n.id, n.linkID)
		}
	}
}
func (h *applicationHub) nativeQuickConnectionMenu() *mygo.MenuItem {
	dark := interfaceDark(h.config.Settings)
	var items func([]*quickConnectionNode) []*mygo.MenuItem
	items = func(nodes []*quickConnectionNode) []*mygo.MenuItem {
		var out []*mygo.MenuItem
		for _, n := range nodes {
			if n.id == "" {
				out = append(out, &mygo.MenuItem{Label: n.label, Icon: n.icon(dark), Submenu: items(n.children)})
			} else {
				id := n.id
				item := h.workspaceMenuItem(n.label, func(a *App) { a.connectQuickID(id, n.linkID) })
				item.Icon = n.icon(dark)
				out = append(out, item)
			}
		}
		return out
	}
	var pins map[string]bool
	if len(h.windows) > 0 {
		pins = h.windows[0].localUI.Pins
	}
	children := items(quickConnectionTree(h.config.Profiles, pins, h.config.Links...))
	if len(children) == 0 {
		children = append(children, &mygo.MenuItem{Label: "暂无快捷会话", Disabled: true})
	}
	children = append(children, mygo.Separator(), h.workspaceMenuItem("管理会话…", func(a *App) { a.openManager() }))
	return &mygo.MenuItem{Label: "快捷连接", Submenu: children}
}

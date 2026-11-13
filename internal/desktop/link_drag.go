package desktop

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"path"
	"reflect"
	"slices"
	"strings"
	"time"
)

func (a *App) linkDrag(key string) linkRootDrag {
	snapshot := a.config.Clone()
	var generation uint64
	if a.store != nil {
		generation = a.store.ConfigurationGeneration()
	}
	return linkRootDrag{Key: key, Owner: a, Snapshot: &snapshot, Generation: generation}
}
func (a *App) acceptsLinkDrag(d linkRootDrag) bool {
	return d.Owner == a && d.Snapshot != nil && !a.passwordLocked() && !a.configurationBusy() && reflect.DeepEqual(*d.Snapshot, a.config) && (a.store == nil || d.Generation == a.store.ConfigurationGeneration())
}
func (a *App) applyLinkDrop(d linkRootDrag, before, folder string) {
	if !a.acceptsLinkDrag(d) {
		a.status = "链接栏配置已变化，请重新拖动"
		return
	}
	if a.changeConfig(func(c *core.Config) error {
		// A menu entry can start below the root. Keep its previous folder, even
		// when moving its last child, then use the existing root ordering rules.
		found := false
		switch {
		case strings.HasPrefix(d.Key, "link:"):
			id := strings.TrimPrefix(d.Key, "link:")
			for i := range c.Links {
				if strings.EqualFold(c.Links[i].ID, id) {
					old := c.Links[i].Folder
					if old != "" && !slices.Contains(c.Directories, "Links/"+old) {
						c.Directories = append(c.Directories, "Links/"+old)
					}
					c.Links[i].Folder = ""
					found = true
					break
				}
			}
		case strings.HasPrefix(d.Key, "session:"):
			id := strings.TrimPrefix(d.Key, "session:")
			for i := range c.Profiles {
				p := &c.Profiles[i]
				if strings.EqualFold(p.ID, id) && (p.Group == "Links" || strings.HasPrefix(p.Group, "Links/")) {
					if p.Group != "Links" && !slices.Contains(c.Directories, p.Group) {
						c.Directories = append(c.Directories, p.Group)
					}
					p.Group = "Links"
					found = true
					break
				}
			}
		case strings.HasPrefix(d.Key, "folder:"):
			found = folder == ""
		}
		if !found {
			return fmt.Errorf("拖动来源不属于当前链接栏")
		}
		if folder != "" {
			_, err := c.MoveLinkRootIntoFolder(d.Key, folder)
			return err
		}
		_, err := c.ReorderLinkRoot(d.Key, before)
		return err
	}) {
		a.linkMenuOpen = false
	}
}
func (a *App) openLinkMenu(anchor, folder string) {
	a.linkMenuAnchor = anchor
	a.linkMenuFolder = folder
	a.linkMenuExpanded = make(map[string]*bool)
	a.linkMenuOpen = true
}

const linkHoverDelay = 250 * time.Millisecond

type linkHoverState struct {
	since time.Time
	fired bool
}

// Open once per pointer visit. A click, manual collapse or Escape must not
// immediately be undone by the pointer still resting on the same directory.
func (a *App) expandLinkOnHover(c *ui.Context, item ui.Element, expanded bool, expand func()) {
	state := ui.Local(item, "link-directory-hover", func() linkHoverState { return linkHoverState{} })
	if !item.Hovered() {
		*state = linkHoverState{}
		return
	}
	if expanded || item.Pressed() || item.Dragging() || !c.WindowFocused() || a.modalActive() || a.settingsOpen || a.passwordLocked() {
		state.fired = true
		return
	}
	if state.fired {
		return
	}
	if state.since.IsZero() {
		state.since = c.Now()
	}
	if remaining := linkHoverDelay - c.Now().Sub(state.since); remaining > 0 {
		c.After(remaining)
		return
	}
	state.fired = true
	expand()
	c.After(0)
}

func (a *App) linkFolderPopover(c *ui.Context, anchor ui.Element, key string) {
	if a.linkMenuAnchor != key {
		return
	}
	ui.Popover(c, anchor, &a.linkMenuOpen, func() {
		width, height := c.Size()
		ui.Column(c).Width(min(float32(320), max(1, width-16))).Gap(2).Children(func() {
			folder := a.linkMenuFolder
			ui.Scroll(c).MaxHeight(min(float32(340), max(24, height-anchor.Bounds().Y-anchor.Bounds().H-50))).FillWidth().Label("快捷目录树").Children(func() {
				a.linkFolderTree(c, folder)
			})
			ui.Divider(c)
			flatButton(c, "管理此快捷目录…", folderIcon, "管理此快捷目录…").FillWidth().Justify(ui.Start).OnClick(func() {
				a.linkMenuOpen = false
				a.managerGroup = "Links"
				if folder != "" {
					a.managerGroup += "/" + folder
				}
				a.managerAll = false
				a.query = ""
				a.openManager()
			})
		})
	})
}

func (a *App) linkFolderTree(c *ui.Context, folder string) {
	children := make(map[string][]core.LinkRootItem)
	for _, dir := range a.config.AllDirectories() {
		relative, ok := strings.CutPrefix(dir, "Links/")
		if !ok || relative == "" {
			continue
		}
		parent := path.Dir(relative)
		if parent == "." {
			parent = ""
		}
		children[parent] = append(children[parent], core.LinkRootItem{Kind: "folder", Folder: relative, Name: path.Base(relative)})
	}
	for _, p := range a.config.Profiles {
		if relative, ok := strings.CutPrefix(p.Group, "Links/"); ok {
			children[relative] = append(children[relative], core.LinkRootItem{Kind: "session", ID: p.ID, Name: p.Name, ProfileID: p.ID, Folder: relative})
		}
	}
	// The manager lists sessions before links for equal display names.
	for _, link := range a.config.Links {
		children[link.Folder] = append(children[link.Folder], core.LinkRootItem{Kind: "link", ID: link.ID, Name: link.Name, ProfileID: link.ProfileID, Folder: link.Folder})
	}
	children[""] = a.config.OrderedLinkRootItems()
	if a.linkMenuExpanded == nil {
		a.linkMenuExpanded = make(map[string]*bool)
	}
	if len(children[folder]) == 0 {
		ui.Text(c, "此目录暂无快捷项目").TextColor(c.Theme().TextMuted).Padding(8)
		return
	}
	ui.Tree(c, func() {
		var draw func(string)
		draw = func(parent string) {
			items := children[parent]
			if parent != "" {
				sortSessionList(items, func(item core.LinkRootItem) sessionSortKey {
					return sessionSortKey{name: item.Name, folder: item.Kind == "folder"}
				})
			}
			for _, item := range items {
				contextMenu := func(m *ui.Menu) { a.linkContextMenu(m, &item) }
				if item.Kind == "folder" {
					open := a.linkMenuExpanded[item.Folder]
					if open == nil {
						open = new(bool)
						a.linkMenuExpanded[item.Folder] = open
					}
					row := sessionTreeItem(c.Key(item.Key()), item.Name, open, func() { draw(item.Folder) }, false, sessionIconFor("", false, true, false)).
						Label("快捷子目录 " + item.Name).Tooltip("/Links/" + item.Folder).OnClick(func() { *open = !*open }).ContextMenu(contextMenu)
					a.expandLinkOnHover(c, row, *open, func() { *open = true })
					continue
				}
				p, ok := a.profileByID(item.ProfileID)
				if !ok {
					continue
				}
				shortcut := item.Kind == "link"
				row := sessionTreeItem(c.Key(item.Key()), item.Name, nil, nil, false, sessionIconFor(p.Kind, shortcut, false, false)).
					Label("拖动快捷项目 " + item.Name).Tooltip(sessionListTooltip(p, shortcut)).OnClick(func() {
					if latest, exists := a.profileByID(item.ProfileID); exists {
						a.linkMenuOpen = false
						a.request(latest, 0)
					}
				})
				row.Drag(a.linkDrag(item.Key())).ContextMenu(contextMenu)
			}
		}
		draw(folder)
	}).FillWidth()
}
func (a *App) scrollLinkDrag(c *ui.Context, scroll ui.Element, childOver bool) {
	if d, over := ui.DragOver[linkRootDrag](scroll); childOver || over && a.acceptsLinkDrag(d) {
		x, _, _ := scroll.PointerPosition()
		width := scroll.Bounds().W
		step := float32(0)
		if x < 24 {
			step = -12
		} else if x > width-24 {
			step = 12
		}
		if step != 0 {
			a.linkScroll.X = max(0, min(a.linkScroll.MaxX, a.linkScroll.X+step))
			c.After(16 * time.Millisecond)
		}
	}
}

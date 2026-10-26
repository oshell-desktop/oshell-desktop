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
	a.linkMenuOpen = true
}
func (a *App) linkFolderPopover(c *ui.Context, anchor ui.Element, key string) {
	if a.linkMenuAnchor != key {
		return
	}
	ui.Popover(c, anchor, &a.linkMenuOpen, func() {
		ui.Column(c).Width(300).Gap(4).Children(func() {
			folder := a.linkMenuFolder
			if folder != "" {
				ui.Button(c, "返回上级").OnClick(func() {
					parent := path.Dir(folder)
					if parent == "." {
						parent = ""
					}
					a.linkMenuFolder = parent
				})
			}
			ui.Scroll(c).MaxHeight(340).FillWidth().Children(func() {
				full := "Links"
				if folder != "" {
					full += "/" + folder
				}
				entry := func(key, name, id string) {
					row := ui.Button(c.Key(key), name).FillWidth().Label("拖动快捷项目 " + name).OnClick(func() {
						if p, ok := a.profileByID(id); ok {
							a.linkMenuOpen = false
							a.request(p, 0)
						}
					})
					row.Drag(a.linkDrag(key))
				}
				if folder == "" {
					for _, item := range a.config.OrderedLinkRootItems() {
						if item.Kind == "folder" {
							next := item.Folder
							ui.Button(c, "▸ "+item.Name).FillWidth().OnClick(func() { a.linkMenuFolder = next })
						} else {
							entry(item.Key(), item.Name, item.ProfileID)
						}
					}
					return
				}
				for _, dir := range a.config.AllDirectories() {
					if rest, ok := strings.CutPrefix(dir, full+"/"); ok && rest != "" && !strings.Contains(rest, "/") {
						next := strings.TrimPrefix(dir, "Links/")
						ui.Button(c, "▸ "+rest).FillWidth().OnClick(func() { a.linkMenuFolder = next })
					}
				}
				for _, l := range a.config.Links {
					if l.Folder == folder {
						entry("link:"+l.ID, l.Name, l.ProfileID)
					}
				}
				for _, p := range a.config.Profiles {
					if p.Group == full {
						entry("session:"+p.ID, p.Name, p.ID)
					}
				}
			})
			ui.Button(c, "管理此快捷目录…").OnClick(func() {
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

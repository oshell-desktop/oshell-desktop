package desktop

import (
	"fmt"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"strings"
)

type linkEditorState struct {
	expanded                      map[string]*bool
	searchExpanded                map[string]*bool
	query                         string
	index                         int
	list                          ui.ListState
	open                          bool
	mode, id, name, folder, error string
}

func (a *App) editLink(id string) {
	for _, link := range a.config.Links {
		if link.ID == id {
			a.features.linkEditor = linkEditorState{open: true, mode: "link", id: id, name: link.Name, folder: link.Folder}
			return
		}
	}
	a.status = "错误：快捷引用已不存在"
}
func (a *App) editLinkFolder(folder string, create bool) {
	state := linkEditorState{open: true, mode: "renameFolder", folder: folder, name: folder}
	if create {
		state.mode = "newFolder"
		state.name = ""
	} else {
		parts := strings.Split(folder, "/")
		state.name = parts[len(parts)-1]
	}
	a.features.linkEditor = state
}
func (a *App) applyLinkEditor() {
	state := &a.features.linkEditor
	if !state.open {
		return
	}
	var operationError error
	if a.changeConfig(func(config *core.Config) error {
		switch state.mode {
		case "link":

			operationError = config.UpdateLink(state.id, state.name, state.folder)
		case "newFolder":
			var child string
			child, operationError = core.LinkFolderChild(state.folder, state.name)
			if operationError == nil {
				for _, directory := range config.AllDirectories() {
					if directory == "Links/"+child {
						return fmt.Errorf("文件夹已存在")
					}
				}
				operationError = config.AddDirectory("Links/" + child)
			}
		case "renameFolder":
			parent := ""
			if index := strings.LastIndexByte(state.folder, '/'); index >= 0 {
				parent = state.folder[:index]
			}
			var child string
			child, operationError = core.LinkFolderChild(parent, state.name)
			if operationError == nil {
				operationError = config.MoveDirectory("Links/"+state.folder, "Links/"+child)
			}
		}
		return operationError
	}) {
		state.open = false
	} else {
		state.error = a.status
	}
}
func (a *App) linkEditorDialog(c *ui.Context) {
	state := &a.features.linkEditor
	wasOpen := state.open
	ui.Modal(c, &state.open, func() {
		ui.Column(c).Width(480).Gap(12).Children(func() {
			if state.mode == "addExisting" {
				a.linkPickerView(c)
				return
			}
			title := "快捷引用属性"
			if state.mode == "newFolder" {
				title = "新增文件夹"
			}
			if state.mode == "renameFolder" {
				title = "重命名文件夹"
			}
			ui.Text(c, title).FontSize(18).Bold()
			if state.mode == "link" {
				if profile, ok := a.currentLinkProfile(state.id); ok {
					source := "/"
					if profile.Group != "" {
						source += strings.Trim(profile.Group, "/") + "/"
					}
					ui.Text(c, "原会话："+source+profile.Name+" · "+profile.Host).Wrap()
				}
				ui.Text(c, "原会话的位置、密码和连接配置保持不变；连接时读取最新配置。").Wrap()
			}
			nameInput := ui.TextInput(c, &state.name).Label("名称").AutoFocus()
			if state.mode == "link" {
				nameInput.Placeholder("留空使用原会话名称")
			}
			if state.mode == "link" {
				options := [][2]string{{"", "快捷链接栏（平铺）"}}
				for _, directory := range a.config.AllDirectories() {
					if folder, ok := strings.CutPrefix(directory, "Links/"); ok && folder != "" {
						options = append(options, [2]string{folder, folder})
					}
				}
				selectValue(c, "保存位置", &state.folder, options)
			}
			if state.error != "" {
				ui.Text(c, state.error).TextColor(c.Theme().Danger)
			}
			ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
				ui.Button(c, "取消").OnClick(func() { state.open = false })
				ui.PrimaryButton(c, "保存").OnClick(a.applyLinkEditor)
			})
		})
	})
	if wasOpen && !state.open {
		a.restoreManagerReturn(nil)
	}
}
func (a *App) removeLink(id string) {
	a.changeConfig(func(config *core.Config) error { config.RemoveLink(id); return nil })
}
func (a *App) removeLinkFolder(folder string) {
	candidate := a.config.Clone()
	if e := candidate.RemoveLinkFolder(folder); e != nil {
		a.status = "错误：" + e.Error()
		if a.win != nil && mygo.App.IsReady() {
			go mygo.Dialog.Message(mygo.MessageOptions{Parent: a.win, Type: mygo.MessageWarning, Message: "无法删除快捷链接文件夹", Detail: e.Error(), Buttons: []string{"好"}})
		}
		return
	}
	if a.win == nil {
		return
	}
	go func() {
		result, e := mygo.Dialog.Message(mygo.MessageOptions{Parent: a.win, Type: mygo.MessageQuestion, Message: "删除快捷链接文件夹“" + folder + "”？", Detail: "该目录和其中的快捷引用会被移除；原会话及配置保留。", Buttons: []string{"取消", "删除"}, DefaultButton: 0, CancelButton: 0})
		if e == nil && result.Button == 1 {
			a.post(func() { a.changeConfig(func(config *core.Config) error { return config.RemoveLinkFolder(folder) }) })
		}
	}()
}
func (a *App) showLinksDirectory(folder string) {
	a.managerGroup = "Links"
	if folder != "" {
		a.managerGroup += "/" + folder
	}
	a.managerAll = false
	a.query = ""
	a.openManager()
}
func (a *App) linkContextMenu(menu *ui.Menu, item *core.LinkRootItem) {
	folder := ""
	if item != nil {
		switch item.Kind {
		case "session":
			if menu.Item("新建连接").Chosen() {
				if profile, ok := a.profileByID(item.ProfileID); ok {
					a.request(profile, 0)
				}
			}
			if menu.Item("在会话管理中查看…").Chosen() {
				if profile, ok := a.profileByID(item.ProfileID); ok {
					a.showLinkedSessionInManager(profile)
				}
			}
		case "link":
			if menu.Item("源会话属性…").Chosen() {
				if profile, ok := a.currentLinkProfile(item.ID); ok {
					a.edit(profile)
				}
			}
			if menu.Item("新建连接").Chosen() {
				if profile, ok := a.currentLinkProfile(item.ID); ok {
					a.request(profile, 0)
				}
			}
			if menu.Item("重命名链接…").Chosen() {
				a.editLink(item.ID)
			}
			a.linkMoveMenu(menu, item.ID)
			if menu.Item("从快捷链接移除").Chosen() {
				a.removeLink(item.ID)
			}
		case "folder":
			folder = item.Folder
			if menu.Item("重命名文件夹…").Chosen() {
				a.editLinkFolder(folder, false)
			}
			if menu.Item("删除快捷链接文件夹…").Chosen() {
				a.removeLinkFolder(folder)
			}
		}
		menu.Separator()
	}
	pane := a.activePane()
	canAdd := pane != nil && !pane.closed && pane.profile.Kind != "blank"
	if menu.Item("添加链接").Disabled(!canAdd).Chosen() {
		a.addCurrentLink()
	}
	if menu.Item("添加已有会话链接…").Chosen() {
		a.addExistingLink(folder)
	}
	if menu.Item("新增文件夹").Chosen() {
		a.editLinkFolder(folder, true)
	}
	if item != nil {
		menu.Submenu("管理链接", func(management *ui.Menu) {
			for _, directory := range a.config.AllDirectories() {
				child, ok := strings.CutPrefix(directory, "Links/")
				if !ok || child == "" || folder != "" && (child == folder || !core.Within(child, folder)) {
					continue
				}
				management.Submenu(child, func(actions *ui.Menu) {
					if actions.Item("重命名文件夹…").Chosen() {
						a.editLinkFolder(child, false)
					}
					if actions.Item("新建子文件夹…").Chosen() {
						a.editLinkFolder(child, true)
					}
					if actions.Item("删除快捷链接文件夹…").Chosen() {
						a.removeLinkFolder(child)
					}
				})
			}
			for _, link := range a.config.Links {
				if folder != "" && !core.Within(link.Folder, folder) {
					continue
				}
				title := link.Name
				if link.Folder != "" {
					title = link.Folder + "/" + title
				}
				management.Submenu(title, func(actions *ui.Menu) {
					if actions.Item("重命名链接…").Chosen() {
						a.editLink(link.ID)
					}
					if actions.Item("从快捷链接移除").Chosen() {
						a.removeLink(link.ID)
					}
					a.linkMoveMenu(actions, link.ID)
				})
			}
		})
		menu.Separator()
	}
	if item == nil {
		menu.Separator()
	}
	if menu.Item("打开 Links 文件夹").Chosen() {
		a.showLinksDirectory("")
	}
	if menu.Item("隐藏链接栏").Chosen() {
		a.changeConfig(func(config *core.Config) error { config.Settings.LinksVisible = false; return nil })
	}
}
func (a *App) linkMoveMenu(menu *ui.Menu, id string) {
	menu.Submenu("移动到", func(targets *ui.Menu) {
		folders := []string{""}
		for _, directory := range a.config.AllDirectories() {
			if folder, ok := strings.CutPrefix(directory, "Links/"); ok && folder != "" {
				folders = append(folders, folder)
			}
		}
		for _, folder := range folders {
			title := folder
			if title == "" {
				title = "快捷链接栏（平铺）"
			}
			if targets.Item(title).Chosen() {
				a.changeConfig(func(config *core.Config) error {
					for _, link := range config.Links {
						if link.ID == id {
							return config.UpdateLink(id, link.Name, folder)
						}
					}
					return nil
				})
			}
		}
	})
}

func (a *App) showLinkedSessionInManager(profile core.Profile) {
	a.managerGroup = profile.Group
	a.managerAll = false
	a.query = ""
	a.openManager()
	a.managerSelection.Clear()
	a.managerRow = -1
	for index, row := range a.managerRows() {
		if row.linkID == "" && !row.folder && row.profile.ID == profile.ID {
			a.managerSelection.Add(row.id)
			a.managerRow = index
			a.selected = profile.ID
			a.managerList.ScrollTo(index, ui.Center)
			break
		}
	}
	if a.manager != nil {
		a.manager.Invalidate()
	}
}

package desktop

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
	"oshell-desktop/internal/terminal"
)

type imagePasteRule struct{ Binding, Directory, Name string }
type imagePasteState struct {
	contextToken                 string
	hostEpoch                    int
	pane                         *pane
	term                         *terminal.Terminal
	conn                         *core.Connection
	identity                     core.HostIdentity
	data                         []byte
	width, height                int
	directory, error             string
	open, busy, remote, remember bool
	cancel                       context.CancelFunc
	sent, total                  atomic.Int64
}

func imagePasteBinding(p core.Profile) string {
	// Include routing as well as the credential identity. No credentials are stored.
	proxy := func(p core.ProxySettings) []string {
		return []string{p.Kind, p.Host, fmt.Sprint(p.Port), p.Username, p.IdentityFile}
	}
	fields := []string{p.Kind, p.Host, fmt.Sprint(p.Port), p.Username, p.JumpHost, p.IdentityFile, strings.ToLower(p.ProxyID)}
	fields = append(fields, proxy(p.Proxy)...)
	for _, route := range p.RuntimeProxyRoute {
		fields = append(fields, proxy(route)...)
	}
	b, _ := json.Marshal(fields)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func (a *App) requestClipboardImage(p *pane) bool {
	if !mygo.App.IsReady() {
		return false
	}
	formats := mygo.Clipboard.Formats()
	var data []byte
	var representations [][]byte
	var imagePath string
	var err error
	if slices.Contains(formats, transfer.FileList) || slices.Contains(formats, transfer.URIList) {
		files, e := mygo.Clipboard.ReadFiles()
		if e != nil {
			a.status = "读取剪贴板文件失败"
			return true
		}
		imagePath, err = clipboardImageFile(files)
		if err == nil && imagePath == "" {
			return false // Never paste a file manager thumbnail.
		}
	} else {
		representations, err = readClipboardImageRepresentations(formats, mygo.Clipboard.ReadFormat)
		if len(representations) == 0 && err == nil {
			return false
		}
	}
	if err != nil {
		a.status = "读取剪贴板图片失败：" + err.Error()
		return true
	}
	if p == nil || p.closed || p.term == nil || a.workspaceLocked() || a.modalActive() || !p.term.Available() {
		a.status = "当前终端无法粘贴图片"
		return true
	}
	remote := p.conn != nil && !p.term.InLocalToolsMode()
	if p.term.ToolConnectionActive() || !remote && !p.term.LocalImagePasteAllowed() {
		a.status = "请在已连接的 SSH 会话或正在运行的本地终端程序中粘贴图片"
		return true
	}
	s := &imagePasteState{pane: p, term: p.term, conn: p.conn, identity: p.hostObservation.Identity, remote: remote, directory: "/tmp", open: true, busy: true}
	s.contextToken = p.term.PasteContextToken()
	s.hostEpoch = p.hostProbe.epoch
	a.imagePaste = s
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	go func() {
		var encoded []byte
		var width, height int
		if imagePath != "" {
			data, err = core.ReadClipboardImageFile(imagePath)
		}
		if err == nil && ctx.Err() == nil {
			if imagePath != "" {
				representations = [][]byte{data}
			}
			encoded, err = normalizeClipboardRepresentations(representations)
			if err == nil {
				dimensions, readErr := png.DecodeConfig(bytes.NewReader(encoded))
				err = readErr
				width, height = dimensions.Width, dimensions.Height
			}
		}
		a.post(func() {
			if a.imagePaste != s {
				return
			}
			s.busy = false
			if ctx.Err() != nil {
				a.imagePaste = nil
				return
			}
			if err != nil {
				s.error = err.Error()
				return
			}
			s.data, s.width, s.height = encoded, width, height
			if remote {
				if rule, ok := a.localUI.ImagePasteSSH[strings.ToLower(p.profile.ID)]; ok && rule.Binding == imagePasteBinding(p.profile) {
					s.directory = rule.Directory
					a.startImagePaste(s)
				}
			} else if a.localUI.ImagePasteLocal {
				a.startImagePaste(s)
			}
		})
	}()
	return true
}

func (a *App) imagePasteValid(s *imagePasteState) bool {
	p := s.pane
	return !a.workspaceLocked() && !a.stopped.Load() && p != nil && !p.closed && p.term != nil && p.owner.Load() == a && p.term == s.term && p.conn == s.conn && p.term.Available() && p.term.PasteContextToken() == s.contextToken && p.hostProbe.epoch == s.hostEpoch && !p.term.ToolConnectionActive() && (s.remote || p.term.LocalImagePasteAllowed()) && s.remote == (p.conn != nil && !p.term.InLocalToolsMode()) && reflect.DeepEqual(p.hostObservation.Identity, s.identity)
}

func (a *App) startImagePaste(s *imagePasteState) {
	if s.busy || len(s.data) == 0 {
		return
	}
	if !a.imagePasteValid(s) {
		s.error = "原终端连接已改变，请重新粘贴"
		return
	}
	if s.cancel != nil {
		s.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.busy = true
	s.error = ""
	data := s.data
	dir := s.directory
	dataDir := a.store.Dir
	go func() {
		var target string
		var err error
		if s.remote {
			fs, release, e := s.conn.OpenFiles(ctx)
			err = e
			if err == nil {
				defer release()
				defer fs.Close()
				stop := context.AfterFunc(ctx, func() { _ = fs.Close() })
				defer stop()
				if client, ok := fs.(*core.SFTPFileSystem); ok {
					target, err = core.UploadClipboardImage(ctx, client.Client, dir, data, func(sent, total int64) { s.sent.Store(sent); s.total.Store(total) })
				} else {
					err = fmt.Errorf("图片上传需要 SFTP")
				}
			}
		} else if ctx.Err() == nil {
			target, err = core.SaveClipboardImage(dataDir, data, time.Now())
		}
		a.post(func() {
			if a.imagePaste != s {
				if !s.remote && target != "" {
					_ = os.Remove(target)
				}
				return
			}
			s.busy = false
			if ctx.Err() != nil {
				if !s.remote && target != "" {
					_ = os.Remove(target)
				}
				a.imagePaste = nil
				return
			}
			if err != nil {
				s.error = err.Error()
				return
			}
			if !a.imagePasteValid(s) {
				s.error = "原终端连接已改变，未粘贴路径。图片位置：" + target
				return
			}
			// Paste directly into the captured terminal, without synchronized input or a newline.
			escaped, pathErr := core.ClipboardImageShellPath(target, !s.remote && runtime.GOOS == "windows")
			if pathErr != nil {
				s.error = pathErr.Error()
				return
			}
			s.term.Paste(escaped)
			if s.remember {
				p := a.localUI.clone()
				if s.remote {
					if p.ImagePasteSSH == nil {
						p.ImagePasteSSH = map[string]imagePasteRule{}
					}
					p.ImagePasteSSH[strings.ToLower(s.pane.profile.ID)] = imagePasteRule{imagePasteBinding(s.pane.profile), dir, s.pane.profile.Name}
				} else {
					p.ImagePasteLocal = true
				}
				if !a.saveLocalPreferences(p) {
					a.status = "路径已粘贴，但无法保存记忆设置"
					a.imagePaste = nil // Do not offer a retry that pastes the path twice.
					return
				}
			}
			a.status = "图片已保存并粘贴路径"
			a.imagePaste = nil
		})
	}()
}

func clipboardImageFile(files []string) (string, error) {
	for _, file := range files {
		if slices.Contains([]string{".png", ".jpg", ".jpeg", ".gif", ".bmp", ".tif", ".tiff", ".webp", ".heic"}, strings.ToLower(filepath.Ext(file))) {
			if len(files) != 1 {
				return "", fmt.Errorf("图片粘贴一次只能处理一张图片")
			}
			return file, nil
		}
	}
	return "", nil
}

func (a *App) imagePasteDialog(c *ui.Context) {
	s := a.imagePaste
	if s == nil {
		return
	}
	ui.Modal(c, &s.open, func() {
		ui.Column(c).Width(460).Gap(12).Children(func() {
			ui.Text(c, "粘贴图片").FontSize(18).Bold()
			if s.remote {
				ui.Text(c, "上传到当前 SSH 连接后插入文件路径").Wrap()
				ui.TextInput(c, &s.directory).Label("远端图片目录").FillWidth().Disabled(s.busy)
			} else {
				ui.Text(c, "保存到本机图片缓存后插入路径；不会自动执行命令。").Wrap()
			}
			if len(s.data) > 0 {
				ui.Text(c, fmt.Sprintf("PNG · %d × %d · %.1f KiB", s.width, s.height, float64(len(s.data))/1024)).TextColor(c.Theme().TextMuted)
			}
			ui.Checkbox(c, &s.remember, "记住此选择，下次直接处理（仅本机）").Disabled(s.busy)
			if s.busy {
				if total := s.total.Load(); total > 0 {
					ui.Progress(c, float64(s.sent.Load())/float64(total)).FillWidth()
					ui.Text(c, fmt.Sprintf("正在上传图片：%d / %d 字节", s.sent.Load(), total))
				} else {
					ui.Spinner(c).Size(20, 20)
					ui.Text(c, "正在处理图片…")
				}
				c.After(50 * time.Millisecond)
			}
			if s.error != "" {
				ui.Text(c, s.error).Wrap().TextColor(c.Theme().Danger)
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Button(c, "取消").OnClick(func() { s.open = false })
				label := "保存并粘贴路径"
				if s.remote {
					label = "上传并粘贴路径"
				}
				ui.PrimaryButton(c, label).Disabled(s.busy || len(s.data) == 0).OnClick(func() { a.startImagePaste(s) })
			})
		})
	})
	if !s.open {
		if s.cancel != nil {
			s.cancel()
		}
		a.imagePaste = nil
	}
}

func (a *App) imagePasteSettings(c *ui.Context) {
	ui.Divider(c)
	ui.Text(c, "图片粘贴（仅本机）").Bold()
	p := &a.localUIDraft
	if p.ImagePasteLocal || len(p.ImagePasteSSH) > 0 {
		ui.Button(c, "恢复全部图片粘贴确认").OnClick(func() { p.ImagePasteLocal = false; p.ImagePasteSSH = nil })
	}
	if p.ImagePasteLocal {
		ui.Button(c, "恢复本机图片粘贴确认").OnClick(func() { p.ImagePasteLocal = false })
	}
	keys := make([]string, 0, len(p.ImagePasteSSH))
	for k := range p.ImagePasteSSH {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		rule := p.ImagePasteSSH[k]
		ui.Row(c.Key(k)).Gap(8).Children(func() {
			ui.Text(c, rule.Name+" · "+rule.Directory).Grow(1).Wrap()
			ui.Button(c, "恢复确认").OnClick(func() { delete(p.ImagePasteSSH, k) })
		})
	}
	if len(keys) == 0 && !p.ImagePasteLocal {
		ui.Text(c, "尚未记住自动图片粘贴选择").TextColor(c.Theme().TextMuted)
	}
}

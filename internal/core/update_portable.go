package core

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"debug/elf"
	"debug/pe"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const portableJournal = ".oshell-update.json"
const portableResult = ".oshell-update-result.txt"
const portableReceiptEnv = "OSHELL_UPDATE_RECEIPT"

type portableFile struct {
	Name, SHA256 string
	Existed      bool
}
type portablePlan struct {
	Executable, Stage, Token, Version string
	ParentPID                         int
	Files                             []portableFile
}
type PortableUpdate struct {
	plan      portablePlan
	handedOff bool
}

func portableResource(name string) bool {
	if name == "LICENSE" || name == "THIRD_PARTY_NOTICES.md" || name == "icon.png" || name == "oshell-desktop.png" || name == "oshell-desktop.desktop" || name == "libghostty-vt.so" {
		return true
	}
	return strings.HasPrefix(name, "licenses/") || strings.HasPrefix(name, "shell-integration/")
}
func cleanUpdateName(name string) bool {
	for _, part := range strings.Split(name, "/") {
		if strings.TrimRight(part, " .") != part {
			return false
		}
		base, _, _ := strings.Cut(strings.ToUpper(part), ".")
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '0' && base[3] <= '9' {
			return false
		}
	}
	return name != "." && name != "" && path.Clean(name) == name && !strings.HasPrefix(name, "/") && !strings.HasPrefix(name, "../") && !strings.ContainsAny(name, "\\:\x00")
}
func digestFile(name string) (string, error) {
	f, e := os.Open(name)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return "", e
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func copyUpdateFile(source, destination string, mode os.FileMode) error {
	in, e := os.Open(source)
	if e != nil {
		return e
	}
	defer in.Close()
	if e = os.MkdirAll(filepath.Dir(destination), 0700); e != nil {
		return e
	}
	out, e := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if e != nil {
		return e
	}
	_, e = io.Copy(out, in)
	if e == nil {
		e = out.Sync()
	}
	closeErr := out.Close()
	if e == nil {
		e = closeErr
	}
	return e
}
func writeUpdateJSON(name string, value any) error {
	data, e := json.Marshal(value)
	if e != nil {
		return e
	}
	f, e := os.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	_, e = f.Write(data)
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e == nil {
		e = closeErr
	}
	return e
}
func checkUpdateBinary(name, target string) error {
	switch target {
	case "windows-amd64":
		f, e := pe.Open(name)
		if e != nil {
			return fmt.Errorf("更新 EXE 无效：%w", e)
		}
		defer f.Close()
		if f.Machine != pe.IMAGE_FILE_MACHINE_AMD64 || f.Characteristics&pe.IMAGE_FILE_DLL != 0 {
			return fmt.Errorf("更新 EXE 架构错误")
		}
	case "linux-amd64":
		f, e := elf.Open(name)
		if e != nil {
			return fmt.Errorf("更新 ELF 无效：%w", e)
		}
		defer f.Close()
		if f.Machine != elf.EM_X86_64 || (f.Type != elf.ET_EXEC && f.Type != elf.ET_DYN) {
			return fmt.Errorf("更新 ELF 架构错误")
		}
	default:
		return fmt.Errorf("当前平台不支持自更新")
	}
	return nil
}
func (u *ReleaseUpdate) Prepare(ctx context.Context, executable string, progress func(int64, int64)) (result *PortableUpdate, err error) {
	if err = u.verifyAuthority(); err != nil {
		return nil, err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return nil, err
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(executable)
	if _, e := os.Lstat(filepath.Join(dir, portableJournal)); !os.IsNotExist(e) {
		return nil, fmt.Errorf("有未完成的更新，请先重新启动应用以恢复")
	}
	stage, err := os.MkdirTemp(dir, ".oshell-update-")
	if err != nil {
		return nil, fmt.Errorf("程序目录不可写，请将便携版移动到可写目录后更新：%w", err)
	}
	defer func() {
		if result == nil {
			os.RemoveAll(stage)
		}
	}()
	archive, err := u.download(ctx, stage, progress, updateDownloadClient())
	if err != nil {
		return nil, err
	}
	defer os.Remove(archive)
	p := &PortableUpdate{plan: portablePlan{Executable: executable, Stage: filepath.Base(stage), Version: u.Version, ParentPID: os.Getpid()}}
	var token [32]byte
	if _, err = rand.Read(token[:]); err != nil {
		return nil, err
	}
	p.plan.Token = hex.EncodeToString(token[:])
	if err = p.extract(ctx, archive, u.target); err != nil {
		return nil, err
	}
	return p, nil
}
func (p *PortableUpdate) stage() string {
	return filepath.Join(filepath.Dir(p.plan.Executable), p.plan.Stage)
}
func (p *PortableUpdate) Discard() {
	if p != nil && !p.handedOff {
		os.RemoveAll(p.stage())
	}
}
func (p *PortableUpdate) extract(ctx context.Context, archive, target string) error {
	seen := map[string]bool{}
	var total int64
	exeName := "oshell-desktop"
	if target == "windows-amd64" {
		exeName = "OShell Desktop.exe"
	}
	consume := func(name string, size int64, mode os.FileMode, r io.Reader) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if target == "windows-amd64" {
			name = strings.TrimPrefix(name, "windows-amd64-single-file/")
		}
		if !cleanUpdateName(name) || !mode.IsRegular() {
			return fmt.Errorf("更新包含不安全路径或链接：%s", name)
		}
		if name != exeName && (target != "linux-amd64" || !portableResource(name)) {
			return fmt.Errorf("更新包包含非程序文件：%s", name)
		}
		if name == exeName {
			name = filepath.Base(p.plan.Executable)
		}
		key := strings.ToLower(name)
		if seen[key] {
			return fmt.Errorf("更新包包含重复文件")
		}
		seen[key] = true
		total += size
		if size < 0 || total > UpdateArchiveLimit || len(seen) > 4096 {
			return fmt.Errorf("更新包解压超过限制")
		}
		destination := filepath.Join(p.stage(), "payload", filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
			return err
		}
		f, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0755)
		if err != nil {
			return err
		}
		n, err := io.Copy(f, io.LimitReader(r, size+1))
		if err == nil && n != size {
			err = fmt.Errorf("解压文件大小不匹配")
		}
		if err == nil {
			err = f.Sync()
		}
		ce := f.Close()
		if err == nil {
			err = ce
		}
		if err != nil {
			return err
		}
		digest, err := digestFile(destination)
		if err != nil {
			return err
		}
		p.plan.Files = append(p.plan.Files, portableFile{Name: name, SHA256: digest})
		return nil
	}
	if target == "windows-amd64" {
		z, e := zip.OpenReader(archive)
		if e != nil {
			return e
		}
		defer z.Close()
		for _, f := range z.File {
			if f.FileInfo().IsDir() {
				if strings.TrimSuffix(f.Name, "/") != "windows-amd64-single-file" {
					return fmt.Errorf("更新 ZIP 含意外目录")
				}
				continue
			}
			if f.UncompressedSize64 > UpdateArchiveLimit {
				return fmt.Errorf("更新包解压超过限制")
			}
			r, e := f.Open()
			if e != nil {
				return e
			}
			e = consume(f.Name, int64(f.UncompressedSize64), f.Mode(), r)
			r.Close()
			if e != nil {
				return e
			}
		}
	} else {
		f, e := os.Open(archive)
		if e != nil {
			return e
		}
		defer f.Close()
		gz, e := gzip.NewReader(f)
		if e != nil {
			return e
		}
		defer gz.Close()
		tr := tar.NewReader(gz)
		for {
			h, e := tr.Next()
			if e == io.EOF {
				break
			}
			if e != nil {
				return e
			}
			if h.Typeflag == tar.TypeDir {
				name := strings.TrimSuffix(h.Name, "/")
				if !cleanUpdateName(name) || !(name == "licenses" || name == "shell-integration" || portableResource(name)) {
					return fmt.Errorf("更新 TAR 目录无效")
				}
				continue
			}
			if h.Typeflag != tar.TypeReg {
				return fmt.Errorf("更新 TAR 不允许链接或特殊文件")
			}
			if e = consume(h.Name, h.Size, h.FileInfo().Mode(), tr); e != nil {
				return e
			}
		}
	}
	if !seen[strings.ToLower(filepath.Base(p.plan.Executable))] || target == "linux-amd64" && !seen["libghostty-vt.so"] {
		return fmt.Errorf("更新包缺少程序或运行库")
	}
	return checkUpdateBinary(filepath.Join(p.stage(), "payload", filepath.Base(p.plan.Executable)), target)
}
func safeUpdateDestination(root, name string) error {
	if !cleanUpdateName(filepath.ToSlash(name)) {
		return fmt.Errorf("更新目标路径无效")
	}
	parts := strings.Split(filepath.ToSlash(name), "/")
	for i := range parts {
		info, e := os.Lstat(filepath.Join(append([]string{root}, parts[:i+1]...)...))
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return e
		}
		if info.Mode()&os.ModeSymlink != 0 || i < len(parts)-1 && !info.IsDir() || i == len(parts)-1 && !info.Mode().IsRegular() {
			return fmt.Errorf("更新目标含链接或非普通文件：%s", name)
		}
	}
	return nil
}

// Start makes a copy of the CURRENT executable, not the downloaded binary, to
// perform replacement after exit. It runs before any GUI/DLL initialization.
func (p *PortableUpdate) Start() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	dir := filepath.Dir(p.plan.Executable)
	unlock, e := lockNamedDirectory(ctx, dir, ".oshell-update.lock")
	if e != nil {
		return fmt.Errorf("另一个更新正在运行")
	}
	defer func() { unlock() }()
	if _, e = os.Lstat(filepath.Join(dir, portableJournal)); !os.IsNotExist(e) {
		return fmt.Errorf("存在未完成更新，请重新启动后重试")
	}
	for i := range p.plan.Files {
		f := &p.plan.Files[i]
		if e = safeUpdateDestination(dir, f.Name); e != nil {
			return e
		}
		_, e = os.Stat(filepath.Join(dir, filepath.FromSlash(f.Name)))
		f.Existed = e == nil
		if e != nil && !os.IsNotExist(e) {
			return e
		}
	}
	helper := filepath.Join(p.stage(), "updater"+executableSuffix())
	if e = copyUpdateFile(p.plan.Executable, helper, 0700); e != nil {
		return e
	}
	if e = writeUpdateJSON(filepath.Join(dir, portableJournal), p.plan); e != nil {
		return e
	}
	cmd := exec.Command(helper, "--oshell-apply-update", filepath.Join(dir, portableJournal))
	prepareUpdateCommand(cmd)
	cmd.Dir = dir
	if e = cmd.Start(); e != nil {
		os.Remove(filepath.Join(dir, portableJournal))
		return e
	}
	p.handedOff = true
	// Release the lock before the helper acquires it. Wait for readiness before
	// closing the user's sessions, so antivirus/launch failures leave the app open.
	unlock()
	unlock = func() {}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case e := <-done:
			return fmt.Errorf("无法启动更新进程：%v", e)
		case <-deadline.C:
			cmd.Process.Kill()
			<-done
			return fmt.Errorf("更新进程未就绪，请重新启动后重试")
		case <-time.After(50 * time.Millisecond):
			if b, e := os.ReadFile(filepath.Join(p.stage(), "helper-ready")); e == nil && string(b) == p.plan.Token {
				return nil
			}
		}
	}
}
func executableSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

func readPortablePlan(journal string) (portablePlan, error) {
	var p portablePlan
	f, e := os.Open(journal)
	if e != nil {
		return p, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, 1<<20))
	if e != nil {
		return p, e
	}
	if e = json.Unmarshal(b, &p); e != nil {
		return p, e
	}
	dir := filepath.Dir(journal)
	if !filepath.IsAbs(p.Executable) || filepath.Clean(p.Executable) != p.Executable || filepath.Dir(p.Executable) != dir || filepath.Base(p.Stage) != p.Stage || !strings.HasPrefix(p.Stage, ".oshell-update-") || len(p.Token) != 64 || len(p.Files) == 0 || len(p.Files) > 4096 {
		return p, fmt.Errorf("更新事务路径无效")
	}
	if e = safeUpdateDestination(dir, p.Stage+"/payload/"+filepath.Base(p.Executable)); e != nil {
		return p, e
	}
	seen := map[string]bool{}
	for _, f := range p.Files {
		if !cleanUpdateName(f.Name) || f.Name != filepath.Base(p.Executable) && (runtime.GOOS != "linux" || !portableResource(f.Name)) || seen[strings.ToLower(f.Name)] {
			return p, fmt.Errorf("更新事务文件无效")
		}
		seen[strings.ToLower(f.Name)] = true
		for _, name := range []string{f.Name, p.Stage + "/payload/" + f.Name, p.Stage + "/backup/" + f.Name} {
			if e = safeUpdateDestination(dir, name); e != nil {
				return p, e
			}
		}
	}
	return p, nil
}

// Replacement writes a sibling then renames it, so the executable is never
// missing between backup and installation. Backups survive interrupted recovery.
func replaceUpdateFile(source, destination string) error {
	temp := destination + ".oshell-new"
	if e := os.Remove(temp); e != nil && !os.IsNotExist(e) {
		return e
	}
	if e := copyUpdateFile(source, temp, 0755); e != nil {
		return e
	}
	defer os.Remove(temp)
	return renameUpdateFile(temp, destination)
}
func applyPortableFiles(p portablePlan) error {
	dir := filepath.Dir(p.Executable)
	stage := filepath.Join(dir, p.Stage)
	for _, f := range p.Files {
		incoming := filepath.Join(stage, "payload", filepath.FromSlash(f.Name))
		digest, e := digestFile(incoming)
		if e != nil || digest != f.SHA256 {
			return fmt.Errorf("待安装文件校验失败：%s", f.Name)
		}
	}
	// Finish ALL backups before replacing ANY runtime file.
	for _, f := range p.Files {
		if f.Existed {
			if e := copyUpdateFile(filepath.Join(dir, filepath.FromSlash(f.Name)), filepath.Join(stage, "backup", filepath.FromSlash(f.Name)), 0755); e != nil {
				return e
			}
		}
	}
	if e := writeUpdateJSON(filepath.Join(stage, "backups-ready"), p.Token); e != nil {
		return e
	}
	for _, f := range p.Files {
		if e := replaceUpdateFile(filepath.Join(stage, "payload", filepath.FromSlash(f.Name)), filepath.Join(dir, filepath.FromSlash(f.Name))); e != nil {
			return e
		}
	}
	return nil
}
func rollbackPortableFiles(p portablePlan) error {
	dir := filepath.Dir(p.Executable)
	stage := filepath.Join(dir, p.Stage)
	b, e := os.ReadFile(filepath.Join(stage, "backups-ready"))
	if os.IsNotExist(e) {
		return nil
	} // installation never started
	var token string
	if e != nil || json.Unmarshal(b, &token) != nil || token != p.Token {
		return fmt.Errorf("更新备份标记无效，保留事务供恢复")
	}
	var failures []error
	for _, f := range p.Files {
		var e error
		target := filepath.Join(dir, filepath.FromSlash(f.Name))
		if f.Existed {
			e = replaceUpdateFile(filepath.Join(stage, "backup", filepath.FromSlash(f.Name)), target)
		} else {
			e = os.Remove(target)
			if os.IsNotExist(e) {
				e = nil
			}
		}
		if e != nil {
			failures = append(failures, e)
		}
	}
	return errors.Join(failures...)
}
func finishPortable(p portablePlan, message string) error {
	dir := filepath.Dir(p.Executable)
	if e := os.WriteFile(filepath.Join(dir, portableResult), []byte(message), 0600); e != nil {
		return e
	}
	if e := os.Remove(filepath.Join(dir, portableJournal)); e != nil && !os.IsNotExist(e) {
		return e
	}
	os.Remove(filepath.Join(dir, ".oshell-recover.json"))
	// Windows cannot remove the running helper. Keep its private staging folder;
	// the next startup removes it after the helper exits.
	os.WriteFile(filepath.Join(dir, p.Stage, "finished"), []byte(p.Token), 0600)
	return nil
}
func launchUpdated(p portablePlan, receipt bool) (*exec.Cmd, error) {
	cmd := exec.Command(p.Executable)
	cmd.Dir = filepath.Dir(p.Executable)
	cmd.Env = append([]string(nil), os.Environ()...)
	for i := len(cmd.Env) - 1; i >= 0; i-- {
		if strings.HasPrefix(cmd.Env[i], portableReceiptEnv+"=") {
			cmd.Env = append(cmd.Env[:i], cmd.Env[i+1:]...)
		}
	}
	if receipt {
		cmd.Env = append(cmd.Env, portableReceiptEnv+"="+p.Token)
	}
	prepareUpdateCommand(cmd)
	return cmd, cmd.Start()
}
func runPortableHelper(journal string, recoverOnly bool) error {
	p, e := readPortablePlan(journal)
	if e != nil {
		return e
	}
	dir := filepath.Dir(p.Executable)
	stage := filepath.Join(dir, p.Stage)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	unlock, e := lockNamedDirectory(ctx, dir, ".oshell-update.lock")
	if e != nil {
		return e
	}
	defer unlock()
	if e = os.WriteFile(filepath.Join(stage, "helper-ready"), []byte(p.Token), 0600); e != nil {
		return e
	}
	if e = waitUpdateParent(p.ParentPID, 90*time.Second); e != nil {
		return e
	}
	if !recoverOnly {
		e = applyPortableFiles(p)
	} else {
		e = fmt.Errorf("恢复上次中断的更新")
	}
	if e == nil {
		var cmd *exec.Cmd
		cmd, e = launchUpdated(p, true)
		if e == nil {
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			deadline := time.NewTimer(90 * time.Second)
			confirmed := false
		waitReady:
			for {
				select {
				case <-done:
					e = fmt.Errorf("新版程序在启动确认前退出")
					break waitReady
				case <-deadline.C:
					e = fmt.Errorf("新版程序启动超时")
					cmd.Process.Kill()
					<-done
					break waitReady
				case <-time.After(100 * time.Millisecond):
					b, _ := os.ReadFile(filepath.Join(stage, "receipt"))
					if string(b) == p.Token+":"+strconv.Itoa(cmd.Process.Pid) {
						confirmed = true
						break waitReady
					}
				}
			}
			deadline.Stop()
			if confirmed {
				// A receipt immediately followed by a startup crash is not success.
				select {
				case <-done:
					e = fmt.Errorf("新版程序在启动确认后立即退出")
				case <-time.After(2 * time.Second):
					return finishPortable(p, "已更新至 "+p.Version)
				}
			}
		}
	}
	if rollbackErr := rollbackPortableFiles(p); rollbackErr != nil {
		os.WriteFile(filepath.Join(dir, portableResult), []byte(fmt.Sprintf("更新失败，自动回滚未完成；备份保留于 %s：%v", stage, rollbackErr)), 0600)
		return rollbackErr
	}
	if err := finishPortable(p, fmt.Sprintf("更新失败，已恢复旧版：%v", e)); err != nil {
		return err
	}
	_, err := launchUpdated(p, false)
	return err
}

// HandlePortableUpdate runs before loading native libraries. Normal startups
// cannot race the helper. An interrupted transaction is recovered by a copy of
// the old executable after this process exits (Windows locks running images).
func HandlePortableUpdate() (handled bool, err error) {
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		return false, nil
	}
	if len(os.Args) == 3 && (os.Args[1] == "--oshell-apply-update" || os.Args[1] == "--oshell-recover-update") {
		return true, runPortableHelper(os.Args[2], os.Args[1] == "--oshell-recover-update")
	}
	executable, e := os.Executable()
	if e != nil {
		return false, e
	}
	executable, e = filepath.EvalSymlinks(executable)
	if e != nil {
		return false, e
	}
	dir := filepath.Dir(executable)
	journal := filepath.Join(dir, portableJournal)
	if _, e = os.Lstat(journal); os.IsNotExist(e) {
		return false, nil
	} else if e != nil {
		return false, e
	}
	p, e := readPortablePlan(journal)
	if e != nil {
		return false, e
	}
	if os.Getenv(portableReceiptEnv) == p.Token && executable == p.Executable {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	unlock, e := lockNamedDirectory(ctx, dir, ".oshell-update.lock")
	if e != nil {
		return false, fmt.Errorf("更新正在安装，请等待程序自动重启")
	}
	defer unlock()
	p.ParentPID = os.Getpid()
	// Recovery uses a separate plan file, leaving the original journal intact
	// until restoration succeeds.
	recovery := filepath.Join(dir, ".oshell-recover.json")
	os.Remove(recovery)
	if e = writeUpdateJSON(recovery, p); e != nil {
		return false, e
	}
	helper := filepath.Join(dir, p.Stage, "updater"+executableSuffix())
	cmd := exec.Command(helper, "--oshell-recover-update", recovery)
	cmd.Dir = dir
	prepareUpdateCommand(cmd)
	if e = cmd.Start(); e != nil {
		return false, e
	}
	return true, nil
}
func ConfirmPortableUpdate(version string) error {
	executable, e := os.Executable()
	if e != nil {
		return e
	}
	executable, e = filepath.EvalSymlinks(executable)
	if e != nil {
		return e
	}
	dir := filepath.Dir(executable)
	token := os.Getenv(portableReceiptEnv)
	if token == "" {
		return nil
	}
	// This nonce belongs to this startup only, not future relaunches or local
	// terminals spawned by the updated application.
	os.Unsetenv(portableReceiptEnv)
	p, e := readPortablePlan(filepath.Join(dir, portableJournal))
	if e != nil {
		return e
	}
	if p.Token != token || p.Executable != executable || p.Version != version {
		return fmt.Errorf("更新启动确认不匹配")
	}
	return os.WriteFile(filepath.Join(dir, p.Stage, "receipt"), []byte(token+":"+strconv.Itoa(os.Getpid())), 0600)
}

// Only folders with an explicit completed marker are eligible for cleanup.
// An unfinished backup is never removed, including after a failed rollback.
func CleanupPortableUpdates() {
	executable, e := os.Executable()
	if e != nil {
		return
	}
	executable, e = filepath.EvalSymlinks(executable)
	if e != nil {
		return
	}
	dir := filepath.Dir(executable)
	entries, e := os.ReadDir(dir)
	if e != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), ".oshell-update-") {
			continue
		}
		folder := filepath.Join(dir, entry.Name())
		if b, e := os.ReadFile(filepath.Join(folder, "finished")); e == nil && len(b) == 64 {
			if e = os.RemoveAll(folder); e != nil {
				// Windows may still hold the helper image open. RemoveAll may have
				// already removed the marker, so retain it for the next startup.
				os.WriteFile(filepath.Join(folder, "finished"), b, 0600)
			}
		}
	}
}
func PortableUpdateResult() string {
	executable, e := os.Executable()
	if e != nil {
		return ""
	}
	executable, e = filepath.EvalSymlinks(executable)
	if e != nil {
		return ""
	}
	b, _ := os.ReadFile(filepath.Join(filepath.Dir(executable), portableResult))
	return string(b)
}

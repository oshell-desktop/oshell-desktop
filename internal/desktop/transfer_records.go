package desktop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	"oshell-desktop/internal/core"
)

type transferRecord struct {
	ID, Source, Target, Protocol, Status, Error string
	Upload                                      bool
	Started                                     time.Time
	Bytes                                       int64
	Resume                                      bool
	checkpoint                                  *core.SFTPCheckpoint
}
type transferLog struct {
	mu       sync.Mutex
	records  []*transferRecord
	pending  string
	identity core.SSHIdentity
	bound    bool
}

// An SSH config alias can resolve to a different host without changing the
// saved profile. Never carry checkpoints across that actual identity change.
func bindTransferIdentity(f *fileState, identity core.SSHIdentity) {
	if f.records == nil {
		f.records = &transferLog{}
	}
	l := f.records
	l.mu.Lock()
	changed := l.bound && l.identity != identity
	if !changed {
		l.identity = identity
		l.bound = true
	}
	l.mu.Unlock()
	if changed {
		f.records = &transferLog{identity: identity, bound: true}
		f.recordDetail = ""
		f.notice = nil
	}
}

func (l *transferLog) snapshot() []transferRecord {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]transferRecord, len(l.records))
	for i, r := range l.records {
		out[i] = *r
	}
	return out
}
func recordEnded(s string) bool {
	return s == "成功" || s == "失败" || s == "取消" || s == "未执行"
}
func (l *transferLog) pruneLocked(clear bool) {
	count := 0
	for _, r := range l.records {
		if recordEnded(r.Status) {
			count++
		}
	}
	kept := l.records[:0]
	for _, r := range l.records {
		if recordEnded(r.Status) && (clear || count > 500) {
			count--
			continue
		}
		kept = append(kept, r)
	}
	l.records = kept
}
func (l *transferLog) update(r *transferRecord, fn func()) {
	l.mu.Lock()
	fn()
	l.pruneLocked(false)
	l.mu.Unlock()
}

type transferPlan struct {
	source, target string
	upload         bool
}
type recordedFileSystem struct {
	core.RemoteFileSystem
	log      *transferLog
	task     *fileTransfer
	protocol string
	batch    []*transferRecord
	retry    *transferRecord
}

func (f *recordedFileSystem) prepare(plans []transferPlan) {
	f.log.mu.Lock()
	defer f.log.mu.Unlock()
	for _, p := range plans {
		r := &transferRecord{ID: core.NewID(), Source: p.source, Target: p.target, Upload: p.upload, Protocol: f.protocol, Status: "等待", Started: time.Now()}
		f.batch = append(f.batch, r)
		f.log.records = append(f.log.records, r)
	}
}
func (f *recordedFileSystem) finish() {
	f.log.mu.Lock()
	defer f.log.mu.Unlock()
	for _, r := range f.batch {
		if r.Status == "等待" {
			r.Status = "未执行"
		}
	}
	f.log.pruneLocked(false)
}
func (f *recordedFileSystem) run(ctx context.Context, source, target string, upload, replace bool, progress func(core.TransferProgress)) error {
	r := f.retry
	if r == nil {
		for _, candidate := range f.batch {
			if candidate.Source == source && candidate.Target == target && candidate.Upload == upload {
				r = candidate
				break
			}
		}
	}
	if r == nil {
		f.prepare([]transferPlan{{source, target, upload}})
		r = f.batch[len(f.batch)-1]
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	sftp, canResume := f.RemoteFileSystem.(*core.SFTPFileSystem)
	canResume = canResume && !replace
	var info os.FileInfo
	var statErr error
	if upload {
		info, statErr = os.Lstat(source)
	} else if _, standalone := f.RemoteFileSystem.(*standaloneSCPFileSystem); !standalone {
		info, statErr = f.RemoteFileSystem.Lstat(source)
	}
	total := int64(-1)
	if statErr == nil && core.RemoteFileTypeKnown(info) && info.Mode().IsRegular() {
		total = info.Size()
		if canResume && r.checkpoint == nil {
			f.log.update(r, func() { r.checkpoint = &core.SFTPCheckpoint{Source: source, Target: target, Upload: upload} })
		}
	}
	// Directories and standalone SCP downloads have no trustworthy total;
	// display activity and transferred bytes without inventing a percentage.
	f.task.beginNotice(source, f.protocol, r.ID, upload, total)
	f.log.update(r, func() { r.Status = "传输中"; r.Error = ""; r.Resume = r.checkpoint != nil })
	report := func(p core.TransferProgress) {
		f.log.update(r, func() { r.Bytes = p.Bytes })
		if progress != nil {
			progress(p)
		}
	}
	var err error
	if canResume && r.checkpoint != nil {
		err = r.checkpoint.Run(ctx, sftp.Client, report)
	} else if upload {
		err = f.RemoteFileSystem.Upload(ctx, source, target, report)
	} else {
		err = f.RemoteFileSystem.Download(ctx, source, target, replace, report)
	}
	f.log.update(r, func() {
		r.Status = "成功"
		r.Resume = false
		if err != nil {
			r.Error = err.Error()
			r.Status = "失败"
			r.Resume = r.checkpoint != nil
			if errors.Is(err, context.Canceled) || ctx.Err() != nil {
				r.Status = "取消"
				if f.task.pause.Load() && r.Resume {
					r.Status = "已暂停"
				}
			}
		}
	})
	return err
}
func reconnectFileState(old *fileState) *fileState {
	if old == nil {
		return nil
	}
	next := &fileState{dir: old.dir, input: old.dir, filter: old.filter, protocol: old.protocol, records: old.records, recordsOpen: old.recordsOpen, recordDetail: old.recordDetail, rateChoice: old.rateChoice, rateCustom: old.rateCustom, rateUnit: old.rateUnit}
	next.limiter.Set(old.limiter.Rate())
	return next
}
func (f *recordedFileSystem) Upload(ctx context.Context, source, target string, p func(core.TransferProgress)) error {
	return f.run(ctx, source, target, true, false, p)
}
func (f *recordedFileSystem) Download(ctx context.Context, source, target string, replace bool, p func(core.TransferProgress)) error {
	return f.run(ctx, source, target, false, replace, p)
}

func (a *App) resumeTransfer(p *pane, id string) {
	if p == nil || p.closed || p.files == nil || p.files.busy || p.files.records == nil {
		return
	}
	f := p.files
	l := f.records
	l.mu.Lock()
	var record *transferRecord
	for _, r := range l.records {
		if r.ID == id && r.Resume && r.Status != "传输中" {
			record = r
			break
		}
	}
	l.mu.Unlock()
	if record == nil {
		return
	}
	if p.conn == nil || !p.conn.Alive() {
		l.mu.Lock()
		l.pending = id
		l.mu.Unlock()
		a.features.reconnectTarget = p
		profile := p.profile
		profile.InitialDirectory = f.dir
		a.request(profile, -1)
		return
	}
	a.withTransfer(p, func(ctx context.Context, client core.RemoteFileSystem) error {
		wrapped, ok := client.(*recordedFileSystem)
		if !ok {
			return fmt.Errorf("传输记录不可用")
		}
		// A later protocol change must not turn a SFTP retry into SCP overwrite.
		if _, ok := wrapped.RemoteFileSystem.(*core.SFTPFileSystem); !ok {
			return fmt.Errorf("请切换至 SFTP 后继续")
		}
		wrapped.retry = record
		return wrapped.run(ctx, record.Source, record.Target, record.Upload, false, a.transferProgress(p, ctx))
	})
}
func (a *App) resumePendingTransfer(p *pane) {
	if p.files == nil || p.files.records == nil {
		return
	}
	l := p.files.records
	l.mu.Lock()
	id := l.pending
	l.pending = ""
	l.mu.Unlock()
	if id != "" {
		a.resumeTransfer(p, id)
	}
}
func (a *App) transferRecordsView(c *ui.Context, p *pane) {
	f := p.files
	if f.records == nil {
		return
	}
	records := f.records.snapshot()
	ui.Row(c).Gap(6).Children(func() {
		ui.Button(c, fmt.Sprintf("传输记录 (%d)", len(records))).OnClick(func() { f.recordsOpen = !f.recordsOpen })
		if f.recordsOpen {
			ui.Button(c, "清除已结束记录").Disabled(f.busy).OnClick(func() { f.records.mu.Lock(); f.records.pruneLocked(true); f.records.mu.Unlock(); f.recordDetail = "" })
		}
	})
	if !f.recordsOpen {
		return
	}
	f.recordList.Key = func(i int) any { return records[len(records)-1-i].ID }
	ui.List(c, &f.recordList, len(records), func(index int) {
		r := records[len(records)-1-index]
		direction := "下载"
		if r.Upload {
			direction = "上传"
		}
		ui.Row(c.Key(r.ID)).Gap(6).Children(func() {
			label := fmt.Sprintf("%s · %s · %s · %s · %s · %s", r.Started.Format("15:04:05"), direction, r.Protocol, r.Status, bytesLabel(r.Bytes), r.Target)
			item := ui.Button(c, label).Grow(1).MinWidth(0).Justify(ui.Start).Background(ui.Transparent).BorderWidth(0).Tooltip(label + "\n双击查看详情")
			if item.DoubleClicked() {
				f.recordDetail = r.ID
			}
			if r.Resume && r.Status != "传输中" {
				title := "重试"
				if r.Status == "已暂停" {
					title = "继续"
				}
				ui.Button(c, title).Disabled(f.busy).OnClick(func() { a.resumeTransfer(p, r.ID) })
			}
		})
	}).Height(170).FillWidth()
	for _, r := range records {
		if f.recordDetail == r.ID {
			for _, line := range []string{"时间：" + r.Started.Format(time.DateTime), "来源：" + r.Source, "目标：" + r.Target, "结果：" + r.Status + " " + r.Error} {
				ui.Text(c, line).SingleLine().Tooltip(line).FontSize(11)
			}
			break
		}
	}
	ui.Text(c, "续传仅支持 SFTP 普通文件；暂停后未开始的项目需重新选择。关闭标签或清除失败记录后不能恢复，临时片段保留。").FontSize(11).TextColor(c.Theme().TextMuted)
}

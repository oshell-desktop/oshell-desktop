// Package diagnostics records local lifecycle evidence, never session content.
package diagnostics

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const maxLogBytes = 2 << 20

var current atomic.Pointer[Recorder]

type Recorder struct {
	mu             sync.Mutex
	file           *os.File
	path           string
	size           int64
	started        time.Time
	stop           chan struct{}
	done           chan struct{}
	closeOnce      sync.Once
	stopMonitor    func()
	previousWriter io.Writer
	libraryCount   atomic.Uint64
}

// Start must run before loading native libraries. Failure to open diagnostics
// does not prevent the application from starting.
func Start(dir, version string) (*Recorder, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	prune(dir)
	path := filepath.Join(dir, fmt.Sprintf("run-%s-%d.jsonl", time.Now().UTC().Format("20060102T150405.000000000"), os.Getpid()))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	r := &Recorder{file: f, path: path, started: time.Now(), stop: make(chan struct{}), done: make(chan struct{}), previousWriter: log.Writer()}
	current.Store(r)
	log.SetOutput(libraryWriter{})
	info, _ := debug.ReadBuildInfo()
	build := map[string]any{"version": version, "go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH, "pid": os.Getpid()}
	if info != nil {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" || s.Key == "vcs.modified" {
				build[s.Key] = s.Value
			}
		}
	}
	r.Record("process_start", build)
	r.stopMonitor, err = startMonitor(path)
	if err != nil {
		Failure("crash_monitor_start", err)
	}
	go func() {
		exe, e := os.Executable()
		if e != nil {
			return
		}
		f, e := os.Open(exe)
		if e != nil {
			return
		}
		defer f.Close()
		h := sha256.New()
		if _, e = io.Copy(h, f); e == nil {
			r.Record("executable_identity", map[string]any{"sha256": fmt.Sprintf("%x", h.Sum(nil))})
		}
	}()
	go func() {
		defer close(r.done)
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-r.stop:
				return
			case <-ticker.C:
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				fields := map[string]any{"goroutines": runtime.NumGoroutine(), "heap_bytes": m.HeapAlloc, "heap_sys_bytes": m.HeapSys, "sys_bytes": m.Sys, "gc_count": m.NumGC}
				for k, v := range processStats() {
					fields[k] = v
				}
				r.Record("process_heartbeat", fields)
			}
		}
	}()
	return r, nil
}

func Event(event string, fields map[string]any) {
	if r := current.Load(); r != nil {
		r.Record(event, fields)
	}
}
func Exit(reason string, code int) {
	if r := current.Load(); r != nil {
		r.Close(reason, code)
	}
}
func Failure(stage string, err error) {
	fields := map[string]any{"stage": stage, "error_type": fmt.Sprintf("%T", err), "stack": Stack()}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		fields["os_error_code"] = uint64(errno)
	}
	Event("operation_error", fields)
}
func Stack() []string {
	pcs := make([]uintptr, 24)
	n := runtime.Callers(2, pcs)
	frames := runtime.CallersFrames(pcs[:n])
	var out []string
	for {
		f, more := frames.Next()
		out = append(out, fmt.Sprintf("%s %s:%d", f.Function, filepath.Base(f.File), f.Line))
		if !more {
			break
		}
	}
	return out
}

// Fields are deliberately supplied by call sites: never pass raw errors,
// command lines, profile names, addresses, paths or terminal data here.
func (r *Recorder) Record(event string, fields map[string]any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return
	}
	data, err := json.Marshal(map[string]any{"time": time.Now().UTC().Format(time.RFC3339Nano), "uptime_ms": time.Since(r.started).Milliseconds(), "event": event, "fields": fields})
	if err != nil {
		return
	}
	if len(data) > 64<<10 {
		return
	}
	data = append(data, '\n')
	if r.size+int64(len(data)) > maxLogBytes {
		r.file.Close()
		os.Remove(r.path + ".2")
		os.Rename(r.path+".1", r.path+".2")
		if err := os.Rename(r.path, r.path+".1"); err != nil {
			r.file, _ = os.OpenFile(r.path, os.O_WRONLY|os.O_APPEND, 0600)
			return
		}
		r.file, err = os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
		if err != nil {
			return
		}
		r.size = 0
	}
	n, _ := r.file.Write(data)
	r.size += int64(n)
	// Lifecycle/heartbeat events are low frequency. Flush each one so a crash
	// does not erase the final close/quit decision.
	r.file.Sync()
}

func (r *Recorder) Close(reason string, code int) {
	if r == nil {
		return
	}
	r.closeOnce.Do(func() {
		close(r.stop)
		<-r.done
		r.Record("process_exit", map[string]any{"reason": reason, "code": code})
		marker, _ := json.Marshal(map[string]any{"reason": reason, "code": code, "time": time.Now().UTC().Format(time.RFC3339Nano)})
		if err := os.WriteFile(r.path+".exit.json", marker, 0600); err != nil {
			Failure("exit_marker", err)
		}
		if r.stopMonitor != nil {
			r.stopMonitor()
		}
		current.CompareAndSwap(r, nil)
		log.SetOutput(r.previousWriter)
		r.mu.Lock()
		if r.file != nil {
			r.file.Close()
			r.file = nil
		}
		r.mu.Unlock()
	})
}

// Framework logs may embed user paths/errors. Keep their emitting call stack
// and only explicitly known native-loop numeric diagnostics, not raw messages.
type libraryWriter struct{}

func (libraryWriter) Write(p []byte) (int, error) {
	r := current.Load()
	if r == nil {
		return len(p), nil
	}
	count := r.libraryCount.Add(1)
	text := strings.TrimSpace(string(p))
	// Keep the first ten reports and periodic samples under repeated errors.
	if count > 10 && count%1000 != 0 && !nativeLoopPattern.MatchString(text) && !nativeEventPattern.MatchString(text) {
		return len(p), nil
	}
	fields := map[string]any{"stack": Stack()}
	fields["report_count"] = count
	if match := nativeLoopPattern.FindStringSubmatch(text); match != nil {
		fields["native_loop"] = match[0]
	}
	if match := nativeEventPattern.FindStringSubmatch(text); match != nil {
		fields["native_event"] = match[0]
	}
	Event("framework_log", fields)
	return len(p), nil
}

func prune(dir string) {
	entries, _ := os.ReadDir(dir)
	var runs []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "run-") && strings.HasSuffix(e.Name(), ".jsonl") {
			runs = append(runs, e.Name())
		}
	}
	sort.Strings(runs)
	// Only completed runs are removed; a concurrently running process keeps
	// its open files. Abnormal termination is sealed by its monitor.
	for _, name := range runs[:max(0, len(runs)-9)] {
		base := filepath.Join(dir, name)
		_, clean := os.Stat(base + ".exit.json")
		_, ended := os.Stat(base + ".termination.json")
		if clean != nil && ended != nil {
			continue
		}
		for _, suffix := range []string{"", ".1", ".2", ".exit.json", ".termination.json", ".crash.log"} {
			os.Remove(base + suffix)
		}
	}
}

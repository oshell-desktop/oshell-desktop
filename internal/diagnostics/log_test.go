package diagnostics

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if HandleMonitor() {
		return
	}
	os.Exit(m.Run())
}

func TestDiagnosticHelper(t *testing.T) {
	mode := os.Getenv("OSHELL_DIAG_FIXTURE")
	if mode == "" {
		return
	}
	r, err := Start(os.Getenv("OSHELL_DIAG_FIXTURE_DIR"), "fixture")
	if err != nil {
		os.Exit(88)
	}
	Event("fixture_ready", nil)
	log.Print("synthetic-password-must-not-persist https://user:secret@example.invalid/private")
	if err = os.WriteFile(filepath.Join(os.Getenv("OSHELL_DIAG_FIXTURE_DIR"), "ready"), []byte("ready"), 0600); err != nil {
		os.Exit(89)
	}
	switch mode {
	case "normal":
		r.Close("fixture_normal", 0)
	case "panic":
		panic("synthetic-password-must-not-persist")
	case "goroutine":
		go func() { panic("synthetic-password-must-not-persist") }()
		select {}
	case "exit":
		os.Exit(17)
	case "kill":
		for {
			time.Sleep(time.Second)
		}
	}
}

func TestDiagnosticProcessOutcomes(t *testing.T) {
	for _, mode := range []string{"normal", "panic", "goroutine", "exit", "kill"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDiagnosticHelper$")
			cmd.Env = append(os.Environ(), "OSHELL_DIAG_FIXTURE="+mode, "OSHELL_DIAG_FIXTURE_DIR="+dir)
			hideMonitor(cmd)
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			if mode == "kill" {
				waitForFile(t, filepath.Join(dir, "ready"))
				if err := cmd.Process.Kill(); err != nil {
					t.Fatal(err)
				}
			}
			err := cmd.Wait()
			if (err == nil) != (mode == "normal") {
				t.Fatalf("unexpected outcome %v", err)
			}
			runs, _ := filepath.Glob(filepath.Join(dir, "run-*.jsonl"))
			if len(runs) != 1 {
				t.Fatal("missing run log", runs)
			}
			path := runs[0]
			if mode == "normal" {
				waitForFile(t, path+".exit.json")
				if _, err := os.Stat(path + ".termination.json"); !os.IsNotExist(err) {
					t.Fatal("normal exit marked abnormal")
				}
			} else {
				waitForFile(t, path+".termination.json")
				data, _ := os.ReadFile(path + ".termination.json")
				var status map[string]any
				if err := json.Unmarshal(data, &status); err != nil {
					t.Fatal(err)
				}
				if runtime.GOOS == "windows" && mode == "exit" && status["exit_code"] != float64(17) {
					t.Fatal("exit code not captured", status)
				}
				if mode == "panic" || mode == "goroutine" {
					data, err := os.ReadFile(path + ".crash.log")
					if err != nil || !strings.Contains(string(data), "TestDiagnosticHelper") {
						t.Fatal("missing panic stack", err)
					}
				}
			}
			files, _ := os.ReadDir(dir)
			for _, f := range files {
				data, _ := os.ReadFile(filepath.Join(dir, f.Name()))
				if strings.Contains(string(data), "synthetic-password") || strings.Contains(string(data), "example.invalid") || strings.Contains(string(data), "user:secret") {
					t.Fatal("sensitive fixture persisted", f.Name())
				}
			}
			data, _ := os.ReadFile(path)
			if !strings.Contains(string(data), "crash_monitor_ready") {
				t.Fatal("monitor unavailable")
			}
		})
	}
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("missing evidence", filepath.Base(path))
}

func TestDiagnosticRotationRetentionAndPrivacy(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 13; i++ {
		p := filepath.Join(dir, fmt.Sprintf("run-20000101-%02d.jsonl", i))
		os.WriteFile(p, []byte("old"), 0600)
		os.WriteFile(p+".exit.json", []byte("{}"), 0600)
	}
	r, err := Start(dir, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close("test_done", 0)
	for i := 0; i < 140; i++ {
		r.Record("fixture", map[string]any{"sequence": i, "padding": strings.Repeat("x", 16384)})
	}
	r.Record("last_event", nil)
	if _, err := os.Stat(r.path + ".1"); err != nil {
		t.Fatal("no rotation", err)
	}
	for _, suffix := range []string{"", ".1"} {
		info, err := os.Stat(r.path + suffix)
		if err != nil || info.Size() > maxLogBytes {
			t.Fatal("oversized log", err)
		}
	}
	runs, _ := filepath.Glob(filepath.Join(dir, "run-*.jsonl"))
	if len(runs) != 10 {
		t.Fatal("retention", len(runs))
	}
	for i := 0; i < 25; i++ {
		log.Print("synthetic-library-secret")
	}
	log.Print("OSHELL_NATIVE_LOOP result=0 code=0 error=0")
	log.Print("OSHELL_NATIVE_EVENT kind=session_end value=1 flags=0")
	data, _ := os.ReadFile(r.path)
	if !strings.Contains(string(data), "native_loop") {
		t.Fatal("native loop result lost")
	}
	if !strings.Contains(string(data), "native_event") || strings.Contains(string(data), "synthetic-library-secret") {
		t.Fatal("critical native event suppressed or raw log leaked")
	}
	if runtime.GOOS == "windows" {
		fields := processStats()
		if fields["working_set_bytes"] == nil || fields["private_bytes"] == nil || fields["handle_count"] == nil {
			t.Fatal("native resource stats missing", fields)
		}
	}
}

func TestCrashSanitizerDropsValuesAndPaths(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run-fixture.jsonl")
	runMonitor(strings.NewReader("panic: password-123\nsecretFunction(secret-value)\ngoroutine 1 [running]:\nmain.failure(0x123, password-123)\n\tC:\\Users\\private-user\\src\\failure.go:27 +0x42\n"), path, func() any { return 2 })
	data, err := os.ReadFile(path + ".crash.log")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "password") || strings.Contains(text, "private-user") || strings.Contains(text, "secretFunction") || strings.Contains(text, "0x123") {
		t.Fatal("value/path leaked", text)
	}
	if !strings.Contains(text, "main.failure(...)") || !strings.Contains(text, "failure.go:27") {
		t.Fatal("stack location lost", text)
	}
}

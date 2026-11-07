package diagnostics

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

const monitorFlag = "--oshell-diagnostic-monitor"

var nativeLoopPattern = regexp.MustCompile(`OSHELL_NATIVE_LOOP result=-?[0-9]+ code=[0-9]+ error=[0-9]+$`)
var nativeEventPattern = regexp.MustCompile(`OSHELL_NATIVE_EVENT kind=(session_query|session_end|device_removed) value=[0-9]+ flags=[0-9]+$`)
var framePattern = regexp.MustCompile(`^([A-Za-z0-9_./*()+\[\]·-]+)\(.*\)$`)
var sourcePattern = regexp.MustCompile(`^\s+(.+\.go):([0-9]+)(?:\s|$)`)
var goroutinePattern = regexp.MustCompile(`^goroutine [0-9]+ \[[a-zA-Z0-9 ,()]+\]:$`)

// HandleMonitor is called before normal startup, so this small helper never
// loads the native runtime or opens configuration. EOF ends it with its parent.
func HandleMonitor() bool {
	if len(os.Args) != 4 || os.Args[1] != monitorFlag {
		return false
	}
	pid, err := strconv.Atoi(os.Args[3])
	if err != nil {
		return true
	}
	observe, closeObserver := observeProcess(pid)
	defer closeObserver()
	io.WriteString(os.Stdout, "1")
	runMonitor(os.Stdin, os.Args[2], observe)
	return true
}

func startMonitor(path string) (func(), error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(exe, monitorFlag, path, strconv.Itoa(os.Getpid()))
	cmd.Stdin = reader
	ack, err := cmd.StdoutPipe()
	if err != nil {
		reader.Close()
		writer.Close()
		return nil, err
	}
	hideMonitor(cmd)
	if err = cmd.Start(); err != nil {
		reader.Close()
		writer.Close()
		return nil, err
	}
	reader.Close()
	ready := make(chan error, 1)
	go func() {
		var b [1]byte
		_, e := io.ReadFull(ack, b[:])
		if e == nil && b[0] != '1' {
			e = fmt.Errorf("invalid monitor acknowledgement")
		}
		ready <- e
	}()
	select {
	case err = <-ready:
	case <-time.After(3 * time.Second):
		err = fmt.Errorf("monitor startup timed out")
	}
	ack.Close()
	if err != nil {
		writer.Close()
		cmd.Process.Kill()
		cmd.Wait()
		return nil, err
	}
	if err = debug.SetCrashOutput(writer, debug.CrashOptions{}); err != nil {
		writer.Close()
		cmd.Wait()
		return nil, err
	}
	debug.SetTraceback("all")
	Event("crash_monitor_ready", map[string]any{"pid": cmd.Process.Pid})
	done := make(chan struct{})
	go func() { cmd.Wait(); close(done) }()
	return func() {
		debug.SetCrashOutput(nil, debug.CrashOptions{})
		writer.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			cmd.Process.Kill()
			<-done
		}
	}, nil
}

// Go panic values and argument dumps can contain credentials. Preserve only
// failure class, goroutine state, function names and source basename/line.
func sanitizeCrashLine(line string) string {
	if strings.HasPrefix(line, "panic:") {
		return "panic: [value omitted]"
	}
	if strings.HasPrefix(line, "fatal error:") {
		return "fatal error: [value omitted]"
	}
	if strings.HasPrefix(line, "Exception ") {
		return "native exception"
	}
	if strings.HasPrefix(line, "[signal ") {
		return "runtime signal"
	}
	if goroutinePattern.MatchString(line) {
		return line
	}
	if m := sourcePattern.FindStringSubmatch(line); m != nil {
		return "  " + filepath.Base(strings.ReplaceAll(m[1], "\\", "/")) + ":" + m[2]
	}
	if m := framePattern.FindStringSubmatch(line); m != nil {
		return m[1] + "(...)"
	}
	if strings.HasPrefix(line, "created by ") {
		s := strings.Fields(line)
		if len(s) >= 3 && regexp.MustCompile(`^[A-Za-z0-9_./*()·-]+$`).MatchString(s[2]) {
			return "created by " + s[2]
		}
	}
	return ""
}

func runMonitor(input io.Reader, path string, observe func() any) {
	var output *os.File
	var written int
	stackStarted := false
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), 64<<10)
	for scanner.Scan() {
		raw := scanner.Text()
		if goroutinePattern.MatchString(raw) {
			stackStarted = true
		}
		if strings.HasPrefix(raw, "panic:") || strings.HasPrefix(raw, "fatal error:") {
			stackStarted = false
		}
		line := sanitizeCrashLine(raw)
		if !stackStarted && !strings.HasPrefix(line, "panic:") && !strings.HasPrefix(line, "fatal error:") && line != "native exception" && line != "runtime signal" {
			continue
		}
		if line == "" || written >= maxLogBytes {
			continue
		}
		if output == nil {
			output, _ = os.OpenFile(path+".crash.log", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		}
		if output != nil {
			n, _ := io.WriteString(output, line+"\n")
			written += n
			output.Sync()
		}
	}
	// Drain after an overlong line too: do not leave the crashing runtime
	// blocked writing a pipe whose monitor abandoned its reader.
	io.Copy(io.Discard, input)
	if output != nil {
		output.Close()
	}
	_, clean := os.Stat(path + ".exit.json")
	if clean != nil {
		data, _ := json.Marshal(map[string]any{"event": "process_ended_without_clean_exit", "time": time.Now().UTC().Format(time.RFC3339Nano), "crash_stack_saved": written > 0, "exit_code": observe()})
		os.WriteFile(path+".termination.json", data, 0600)
	}
}

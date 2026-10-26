package terminal

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"oshell-desktop/internal/terminal/internal/vt"
	"oshell-desktop/internal/testsupport/winshm"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"oshell-desktop/internal/bundled"
)

// Run by the single-file build while its real embedded payload is available.
// Copy only the binary to an empty directory and start it with no adjacent
// library, no marker and a deliberately invalid external library override.
func TestSingleFileResourcesLoad(t *testing.T) {
	if !bundled.Enabled() {
		t.Skip("requires the single-file build's embedded payload")
	}
	if os.Getenv("OSHELL_EMBEDDED_CHILD") != "1" {
		dir := t.TempDir()
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(executable)
		if err != nil {
			t.Fatal(err)
		}
		copy := filepath.Join(dir, "独立终端 测试.exe")
		if err := os.WriteFile(copy, data, 0700); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(copy, "-test.run=^TestSingleFileResourcesLoad$", "-test.timeout=30s")
		cmd.Dir = dir
		// The parent removes this cache only after the child has exited and
		// Windows has released its loaded DLL.
		cmd.Env = append(os.Environ(), "OSHELL_EMBEDDED_CHILD=1", "LOCALAPPDATA="+t.TempDir())
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("standalone process failed: %v\n%s", err, out)
		}
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) != 1 {
			t.Fatal("resources escaped cache into executable directory", entries, err)
		}
		return
	}
	t.Setenv("MYGO_GHOSTTY_VT", "invalid-external-library.dll")
	lib, err := LibraryPath()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(lib, os.Getenv("LOCALAPPDATA")+string(os.PathSeparator)) {
		t.Fatal("library did not use cache", lib)
	}
	for _, name := range []string{"LICENSE", "THIRD_PARTY_NOTICES.md", "licenses/ghostty-LICENSE", "licenses/SwiftTerm-LICENSE", "licenses/uax29-LICENSE"} {
		if _, err := bundled.Path(name); err != nil {
			t.Fatal(err)
		}
	}
	term, err := New(Options{Conn: newPipe(), NoBlink: true})
	if err != nil {
		t.Fatal(err)
	}
	defer term.Close()
	if !term.term.SetDerivedPalette(fillLabPalette) {
		t.Fatal("embedded engine lacks OShell dynamic palette extension")
	}
	term.write([]byte("embedded-terminal-ready"))
	if !strings.Contains(term.Text(), "embedded-terminal-ready") {
		t.Fatal("embedded engine failed to parse terminal output")
	}
	term.write([]byte("\x1b[?1h"))
	if !term.term.Mode(1) {
		t.Fatal("embedded engine failed to enable application cursor mode")
	}
	term.write([]byte("\x1b[!p"))
	if term.term.Mode(1) || !strings.Contains(term.Text(), "embedded-terminal-ready") {
		t.Fatal("embedded engine lacks non-destructive OShell soft reset")
	}
	term.mu.Lock()
	term.term.SelectCells(0, 0, 7, 0)
	term.mu.Unlock()
	if err := term.SetHistoryLimit(500); err != nil {
		t.Fatal(err)
	}
	if got := term.SearchSelection(); got != "embedded" {
		t.Fatal("embedded capacity change cleared selection", got)
	}
	if err := term.SetHistoryLimit(10000); err != nil {
		t.Fatal(err)
	}
	var imageBytes bytes.Buffer
	if err := png.Encode(&imageBytes, image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	term.mu.Lock()
	defer term.mu.Unlock()
	term.term.Resize(4, 5, 8, 16)
	term.term.Write([]byte("\x1b[2J\x1b[HABCDEFG"))
	term.term.Resize(6, 5, 8, 16)
	if text, _ := term.term.RowText(0); strings.TrimRight(string(text), " ") != "ABCD" {
		t.Fatal("embedded engine reflowed the cursor paragraph", string(text))
	}
	term.term.Write([]byte("\x1b[2J\x1b[H"))
	term.term.Resize(12, 5, 8, 16)
	term.term.Write([]byte("\x1b[2J\x1b[Habcdefghijkl\x1b7\x1b[2;1H\x1b8X"))
	if text, _ := term.term.RowText(0); string(text) != "abcdefghijkX" {
		t.Fatal("embedded engine restored pending wrap", string(text))
	}
	term.term.Write([]byte("\x1b[2J\x1b[H"))
	if err := term.term.SetHistoryLines(1); err != nil {
		t.Fatal(err)
	}
	term.term.Write([]byte(strings.Repeat("h\r\n", 8)))
	if bar := term.term.Scrollbar(); bar.Total-bar.Len != 1 {
		t.Fatal("embedded engine exceeded history capacity", bar)
	}
	term.term.ScrollToTop()
	if !term.term.SelectCells(0, 0, 0, 0) {
		t.Fatal("embedded selection fixture failed")
	}
	term.term.Write([]byte("h\r\n"))
	if _, ok := term.term.SelectionText(); ok {
		t.Fatal("embedded engine retained discarded selection")
	}
	term.term.ScrollToBottom()
	if err := term.term.SetHistoryLines(10000); err != nil {
		t.Fatal(err)
	}
	term.term.Write([]byte("\x1b[3J\x1b[2J\x1b[H"))
	term.term.Write([]byte("000\r\n111\r\n222\r\n333\r\n444\x1b[2;4r"))
	term.term.SelectCells(0, 2, 2, 2)
	term.term.Write([]byte("\x1b[S"))
	if text, ok := term.term.SelectionText(); !ok || text != "222" {
		t.Fatal("embedded engine did not move region selection", text, ok)
	}
	term.term.Write([]byte("\x1b[S"))
	if _, ok := term.term.SelectionText(); ok {
		t.Fatal("embedded engine retained region selection after discard")
	}
	term.term.Write([]byte("\x1b[r\x1b[2J\x1b[H"))
	term.term.Write([]byte("000\r\n111\r\n222\r\n333\r\n444\r\n555\r\n666"))
	if !term.term.SelectSearchRange(2, 0, 2, 3) {
		t.Fatal("embedded logical selection fixture failed")
	}
	term.term.Write([]byte("\x1b[3J"))
	if text, ok := term.term.SelectionText(); !ok || text != "444" {
		t.Fatal("embedded engine lost explicit-trim logical coordinates", text, ok)
	}
	term.term.Write([]byte("\x1b[3J\x1b[2J\x1b[H"))
	term.term.Write([]byte("ABCDEFGHIJKLMNOPQRSTUVWX"))
	endpointGesture, err := vt.NewGesture(500000000, 5)
	if err != nil {
		t.Fatal(err)
	}
	defer endpointGesture.Free(term.term)
	endpointRef, _ := term.term.CellAt(2, 1)
	endpointSelection, selected := endpointGesture.PressCount(term.term, endpointRef, 18, 21, 1, 3)
	if !selected {
		t.Fatal("embedded row gesture fixture")
	}
	term.term.SetSelection(&endpointSelection)
	if text, ok := term.term.SelectionText(); !ok || text != "MNOPQRSTUVW" {
		t.Fatal("embedded engine did not retain the physical row and exclusive endpoint", text, ok)
	}
	term.term.SetBidiEnabled(false)
	term.term.SetSelection(nil)
	endpointGesture.PressCount(term.term, endpointRef, 18, 21, 100, 1)
	endpointSelection, selected = endpointGesture.Drag(term.term, endpointRef, 18, 21, vt.Geometry{Columns: 12, CellWidth: 8, Height: 80}, false)
	if !selected {
		t.Fatal("embedded engine did not return an empty logical selection with BiDi disabled")
	}
	term.term.SetSelection(&endpointSelection)
	if text, ok := term.term.SelectionText(); !ok || text != "" || !term.term.SelectionActive() {
		t.Fatal("embedded engine lost the active empty selection", text, ok)
	}
	term.term.SetBidiEnabled(true)
	term.term.SelectSearchRange(0, 2, 0, 5)
	shiftRef, _ := term.term.CellAt(0, 0)
	shiftSelection, selected := endpointGesture.PressCountWithShift(term.term, shiftRef, 2, 5, 200, 1, true)
	if !selected {
		t.Fatal("embedded engine lacks Shift selection extension")
	}
	term.term.SetSelection(&shiftSelection)
	if text, ok := term.term.SelectionText(); !ok || text != "ABCDE" {
		t.Fatal("embedded reverse Shift lost the former endpoint", text, ok)
	}
	shiftRef, _ = term.term.CellAt(6, 0)
	shiftSelection, selected = endpointGesture.Drag(term.term, shiftRef, 50, 5, vt.Geometry{Columns: 12, CellWidth: 8, Height: 80}, false)
	if !selected {
		t.Fatal("embedded Shift drag fixture")
	}
	term.term.SetSelection(&shiftSelection)
	if text, ok := term.term.SelectionText(); !ok || text != "F" {
		t.Fatal("embedded Shift drag used the pointer press as the fixed endpoint", text, ok)
	}
	term.term.SetSelection(nil)
	identityBefore, identityOK := term.term.BufferIdentity()
	if !identityOK {
		t.Fatal("embedded engine lacks word selection buffer identity")
	}
	term.term.Reset()
	if identityAfter, ok := term.term.BufferIdentity(); !ok || identityAfter[0] != 0 || identityAfter[1] != identityBefore[1]+1 {
		t.Fatal("embedded reset reused the old buffer identity", identityBefore, identityAfter, ok)
	}
	term.term.Write([]byte("x\x1b]133;A;cl=invalid\ay\x1b]133;L;unknown=value\az"))
	if text := term.term.FormattedText(false); text != "xy\nz" {
		t.Fatal("embedded OSC 133 option parsing differs", text)
	}
	term.term.Write([]byte("\x1b]133;N;k=r\aq"))
	if text := term.term.FormattedText(false); text != "xy\nzq" {
		t.Fatal("embedded right prompt moved the cursor", text)
	}
	term.term.Write([]byte("\x1b[?1049h\x1b[2J\x1b[H a\x1b]133;A\ab"))
	if text := term.term.FormattedText(false); text != " ab" {
		t.Fatal("embedded alternate screen applied OSC 133", text)
	}
	term.term.Write([]byte("\x1b[?1049l"))
	term.term.Write([]byte("\x1b]133;B\a"))
	term.term.RegisterUserInput([]byte("\x1b[200~line\nnext\x1b[201~"))
	if state, ok := term.term.SemanticInputState(); !ok || state != vt.SemanticInputArmed {
		t.Fatal("embedded semantic paste scanner differs", state, ok)
	}
	term.term.RegisterUserInput([]byte("\r"))
	if state, ok := term.term.SemanticInputState(); !ok || state != vt.SemanticInputSubmitted {
		t.Fatal("embedded semantic submission scanner differs", state, ok)
	}
	term.term.Reset()
	term.term.Write([]byte("\x1b]133;A\a\x1b]133;B\ax\r\ny"))
	if kind, ok := term.term.SemanticRow(1); !ok || kind != vt.SemanticRowContinuation {
		t.Fatal("embedded engine lacks derived semantic row classification", kind, ok)
	}
	term.term.Write([]byte("\x1b[H\x1b]133;N\a"))
	if kind, ok := term.term.SemanticRow(1); !ok || kind != vt.SemanticRowNone {
		t.Fatal("embedded engine joined a stranded old-group row", kind, ok)
	}
	term.term.Reset()
	term.term.Write([]byte("\x1b]133;A;cl=m\a\x1b]133;B\aabc"))
	if data := string(term.term.SemanticClickData(0, 0)); data != "\x1b[D\x1b[D\x1b[D" {
		t.Fatal("embedded engine lacks semantic click geometry", data)
	}
	if role, ok := term.term.SemanticContentAt(0, 0); !ok || role != vt.SemanticInput {
		t.Fatal("embedded engine lacks per-cell semantic content", role, ok)
	}
	target, ok := term.term.CaptureSemanticClickTarget(0)
	if !ok {
		t.Fatal("embedded engine lacks semantic line capture")
	}
	term.term.Write([]byte("\x1b[H\x1b[L"))
	if row, ok := term.term.ResolveSemanticClickTarget(target); !ok || row != 1 {
		t.Fatal("embedded engine lost moved click target", row, ok)
	}
	term.term.Reset()
	if _, ok := term.term.ResolveSemanticClickTarget(target); ok {
		t.Fatal("embedded reset retained an old click target")
	}
	term.term.Write([]byte("\x1b[3J\x1b[2J\x1b[H"))
	if err := term.term.Resize(80, 24, 8, 16); err != nil {
		t.Fatal(err)
	}
	linkStart := "\x1b]8;id=same;https://example.test/\x1b\\"
	term.term.Write([]byte("\x1b[2J\x1b[H" + linkStart + "ab" + linkStart + "cd\x1b]8;;\x1b\\"))
	for _, x := range []int{0, 2} {
		if a, b, found := term.term.HyperlinkRange(x, 0); !found || a != x || b != x+2 {
			t.Fatalf("embedded engine lacks OSC 8 write identity: %d..%d/%v", a, b, found)
		}
	}
	term.term.Write([]byte("\x1b[2J\x1b[H\x1b]8;id=empty;\x1b\\x\x1b]8;id=例;https://ignored.test/\x1b\\y\x1b]8;;\x1b\\"))
	ref, _ := term.term.CellAt(0, 0)
	if _, _, found := term.term.HyperlinkRange(0, 0); !found || term.term.Hyperlink(ref) != "" {
		t.Fatal("embedded engine lost explicit empty URL")
	}
	if _, _, found := term.term.HyperlinkRange(1, 0); found {
		t.Fatal("embedded engine accepted non-ASCII payload")
	}
	term.term.Write([]byte("\x1b[2J\x1b[H"))
	term.term.Write([]byte("\x1b]1337;File=inline=1:" + base64.StdEncoding.EncodeToString(imageBytes.Bytes()) + "\x07"))
	if len(term.term.Graphics(8, 16)) != 1 {
		t.Fatal("embedded engine lacks OShell inline-image support")
	}
	term.term.Write([]byte("\x1b[2J\x1bPq#1;2;100;0;0~~\x1b\\"))
	if g := term.term.Graphics(8, 16); len(g) != 1 || g[0].Image.Pix[0] != 255 {
		t.Fatal("embedded engine lacks OShell Sixel support")
	}
	term.term.Write([]byte(strings.Repeat("\r\n", 26)))
	term.term.ScrollToTop()
	if len(term.term.Graphics(8, 16)) != 1 {
		t.Fatal("embedded engine lost the history image before clear")
	}
	term.term.Write([]byte("\x1b[2J"))
	term.term.ScrollToTop()
	if len(term.term.Graphics(8, 16)) != 0 {
		t.Fatal("embedded engine retained history images after ED 2")
	}
	imageBytes.Reset()
	if err := png.Encode(&imageBytes, image.NewNRGBA(image.Rect(0, 0, 1200, 1000))); err != nil {
		t.Fatal(err)
	}
	if err := term.term.Resize(80, 24, 16, 2048); err != nil {
		t.Fatal(err)
	}
	term.term.ScrollToBottom()
	term.term.Write([]byte("\x1b[H\x1b]1337;File=inline=1:" + base64.StdEncoding.EncodeToString(imageBytes.Bytes()) + "\x07"))
	if g := term.term.Graphics(16, 2048); len(g) != 1 || len(g[0].Image.Pix) <= 4<<20 {
		t.Fatal("embedded engine conflates decoded inline image size with Kitty cache quota")
	}
	term.term.Write([]byte("\x1b[2J\x1b[H\x1b_Ga=T,f=24,s=1,v=1,i=17,c=1,r=1,C=1;/wAA\x1b\\\x1b_Ga=t,f=24,s=1,v=1,i=17;AAD/\x1b\\"))
	if g := term.term.Graphics(16, 2048); len(g) != 1 || g[0].Image.Pix[0] != 255 {
		t.Fatal("embedded engine replaced displayed pixels during a transmit-only update")
	}
	raw := bytes.Repeat([]byte{20, 40, 60, 255}, 1024*1280)
	term.term.Write([]byte("\x1b[2J\x1b[H\x1b_Ga=T,f=32,s=1024,v=1280,i=18,c=1,r=1,C=1;" + base64.StdEncoding.EncodeToString(raw) + "\x1b\\"))
	if g := term.term.Graphics(16, 2048); len(g) != 1 || len(g[0].Image.Pix) != len(raw) {
		t.Fatal("embedded engine rejected a display larger than the Kitty cache quota")
	}
	privateID := term.term.Graphics(16, 2048)[0].ImageID
	term.term.Write([]byte(fmt.Sprintf("\x1b_Ga=T,f=24,s=1,v=1,i=%d,c=1,r=1,C=1;/wAA\x1b\\", privateID)))
	if len(term.term.Graphics(16, 2048)) != 2 {
		t.Fatal("embedded engine reserves internal image IDs from the client")
	}
	term.term.Write([]byte("\x1b[2J\x1b_Ga=T,f=24,s=1,v=1,i=19,p=1,c=1,r=1,C=1;/wAA\x1b\\\x1b[?47h\x1b_Ga=p,i=19,p=2,c=1,r=1,C=1;\x1b\\"))
	if g := term.term.Graphics(16, 2048); len(g) != 1 || g[0].Image.Pix[0] != 255 {
		t.Fatal("embedded engine cannot share Kitty cache across screens")
	}
	term.term.Write([]byte("\x1b_Ga=d,d=I,i=19;\x1b\\\x1b[?47l"))
	if len(term.term.Graphics(16, 2048)) != 0 {
		t.Fatal("embedded engine did not apply image ID deletion to the other screen")
	}
	term.term.Write([]byte("\x1b[?47h\x1b[HRETAINED"))
	term.term.Reset()
	term.term.Write([]byte("\x1b[?47h\x1b[?1049h"))
	if !strings.Contains(term.term.Text(), "RETAINED") {
		t.Fatal("embedded engine discarded alternate text on reset or repeated entry")
	}
	term.term.Write([]byte("\x1b[?1049l\x1b[?47h"))
	if term.term.Text() != "" {
		t.Fatal("embedded engine did not clear alternate text on 1049 exit")
	}
	term.term.Reset()
	term.term.Write([]byte("\x1b[3g\x1b[1;4H\x1bH\x1b[?47h\x1b[H\t"))
	if x, _ := term.term.Cursor(); x != 8 {
		t.Fatal("embedded engine shares tab stops between screens", x)
	}
	term.term.Write([]byte("\x1b[3g\x1b[1;7H\x1bH\x1b[?47l\x1b[H\t"))
	if x, _ := term.term.Cursor(); x != 3 {
		t.Fatal("embedded engine lost primary custom tab stop", x)
	}
	term.term.Write([]byte("\x1b[?69h\x1b[3;0s\x1b[?6h\x1b[99;99H\t"))
	if x, _ := term.term.Cursor(); x != 0 {
		t.Fatal("embedded engine conflates explicit zero with an omitted margin", x)
	}
	term.term.Write([]byte("\x1b[?6l\x1b[?69l\x1b[H\x1b[0I"))
	if x, _ := term.term.Cursor(); x != 3 {
		t.Fatal("embedded engine ignores zero-count tab instead of defaulting to one", x)
	}
	term.term.Write([]byte("\x1bc\x1b[H\xe2ABZ"))
	if text := strings.TrimSpace(term.term.Text()); text != "âZ" {
		t.Fatal("embedded engine lacks SwiftTerm UTF-8 fallback", text)
	}
	term.term.Write([]byte("\x1bXGa=T,f=24,s=1,v=1,i=99,c=1,r=1,C=1;/wAA\x1b\\"))
	if len(term.term.Graphics(8, 16)) != 0 {
		t.Fatal("embedded engine dispatches graphics from an ignored SOS string")
	}
	term.term.Write([]byte("\x1b_Ga=T,f=24,s=1,v=1,i=99,c=1,r=1,C=1;/wAA\a"))
	if len(term.term.Graphics(8, 16)) != 1 {
		t.Fatal("embedded engine does not terminate APC at BEL")
	}
	term.term.Write([]byte("\x1b[?69h\x1b[3;9s\x1b[2;4r\x1b[?6h\x1b[2;3H\x1b[2G"))
	if x, y := term.term.Cursor(); x != 3 || y != 2 {
		t.Fatal("embedded engine applies row origin twice for CHA", x, y)
	}
	term.term.Write([]byte("\x1b[2d"))
	if x, y := term.term.Cursor(); x != 3 || y != 1 {
		t.Fatal("embedded engine applies origin offsets to VPA", x, y)
	}
	term.term.Reset()
	term.term.Write([]byte("\x1b[Habcdef\x1b[1;3H\x1b[2;99P"))
	if got := term.term.FormattedText(false); got != "abef" {
		t.Fatal("embedded engine lacks OShell character editing", got)
	}
	term.term.Write([]byte("\x1b[S"))
	if strings.Contains(term.term.Text(), "abef") {
		t.Fatal("embedded engine sends explicitly scrolled text to history")
	}
	term.term.Reset()
	term.term.Write([]byte("\x1b[Habcdef\x1b[1;3H\x1b['}\x1b[1;4H\x1b[0;99K"))
	if got := term.term.FormattedText(false); got != "ab" {
		t.Fatal("embedded engine lacks OShell column editing/erase parameters", got)
	}
	term.term.Reset()
	term.term.Write([]byte("\x1b[H\x1b#6double"))
	if term.term.LineModeAt(0) != vt.LineDoubleWidth {
		t.Fatal("embedded engine lacks row display mode support")
	}

	term.term.Write([]byte("\x1b#8"))
	if term.term.LineModeAt(0) != vt.LineDoubleWidth {
		t.Fatal("embedded engine resets line display during alignment")
	}

	term.term.Write([]byte("\x1b[62;0\"p"))
	if eight, level := term.term.ControlEncoding(); !eight || level != 62 {
		t.Fatal("embedded engine lacks control response encoding", eight, level)
	}
	var status bytes.Buffer
	probe, err := vt.NewTerminal(12, 5, vt.Effects{WritePTY: func(b []byte) { status.Write(b) }})
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Free()
	probe.Write([]byte("ABC\x1b[1;1;1;1;1;3*y"))
	if status.String() != "\x1bP1!~00c6\x1b\\" {
		t.Fatalf("embedded engine lacks rectangular checksum queries: %q", status.String())
	}
	status.Reset()
	probe.Write([]byte("\x1b[90;1;1;1;3$x\x1b[1;1;1;1;1;3*y"))
	if status.String() != "\x1bP1!~010e\x1b\\" {
		t.Fatalf("embedded engine lacks rectangular fills: %q", status.String())
	}
	status.Reset()
	probe.Write([]byte("\x1b[1;1;1;3${\x1b[1;1;1;1;1;3*y"))
	if status.String() != "\x1bP1!~0060\x1b\\" {
		t.Fatalf("embedded engine lacks rectangular erasure: %q", status.String())
	}
	probe.Reset()
	status.Reset()
	probe.Write([]byte("ABC\x1b[1;1;1;3;1;2;1;1;0$v\x1b[1;1;2;1;2;3*y"))
	if status.String() != "\x1bP1!~00c6\x1b\\" {
		t.Fatalf("embedded engine lacks rectangular copying: %q", status.String())
	}
	probe.Reset()
	status.Reset()
	probe.Write([]byte("\x1b_Ga=t,t=s,i=71,f=99,o=X;=bad\x1b\\"))
	if status.String() != "\x1b_Gi=71;EINVAL: bad payload\x1b\\" {
		t.Fatalf("embedded engine lacks shared-memory payload validation: %q", status.String())
	}
	// Placements need cell pixel geometry, just like the file media below.
	if err := probe.Resize(12, 5, 8, 16); err != nil {
		t.Fatal(err)
	}
	shared := winshm.New(t, []byte{255, 0, 0}, 0, 0)
	status.Reset()
	probe.Write([]byte("\x1b_Ga=T,t=s,i=72,f=24,s=1,v=1,c=1,r=1,C=1;" + base64.StdEncoding.EncodeToString([]byte("/"+shared.Name)) + "\x1b\\"))
	graphics := probe.Graphics(8, 16)
	if !strings.Contains(status.String(), ";OK") || len(graphics) != 1 || !bytes.Equal(graphics[0].Image.Pix, []byte{255, 0, 0, 255}) {
		t.Fatalf("embedded engine failed actual Windows shared pixels: %q", status.String())
	}
	shared.Close()
	if winshm.Exists(shared.Name) {
		t.Fatal("embedded engine leaked mapping")
	}
	probe.Reset()
	if err := probe.Resize(12, 5, 8, 16); err != nil {
		t.Fatal(err)
	}
	for _, medium := range []string{"f", "t"} {
		file := filepath.Join(t.TempDir(), "tty-graphics-protocol-embedded")
		if err := os.WriteFile(file, []byte{255, 0, 0}, 0600); err != nil {
			t.Fatal(err)
		}
		status.Reset()
		probe.Write([]byte("\x1b_Ga=T,t=" + medium + ",f=24,s=1,v=1,i=71,p=1,c=1,r=1,C=1;" + base64.StdEncoding.EncodeToString([]byte(file)) + "\x1b\\"))
		images := probe.Graphics(8, 16)
		if !strings.Contains(status.String(), ";OK") || len(images) != 1 || !bytes.Equal(images[0].Image.Pix, []byte{255, 0, 0, 255}) {
			t.Fatal("embedded local image failed", medium, status.String())
		}
		if medium == "t" {
			if _, err := os.Stat(file); !os.IsNotExist(err) {
				t.Fatal("embedded temporary image retained", err)
			}
		}
	}
	status.Reset()
	probe.Write([]byte("\x1b[?63;42n"))
	if status.String() != "\x1bP42!~0000\x1b\\" {
		t.Fatalf("embedded engine lacks private status reports: %q", status.String())
	}
	status.Reset()
	probe.Write([]byte("\x1b[31m\x1bP$qm\x1b\\\x1b[21t\x1b[19t"))
	if status.String() != "\x1bP1$r0;31;m\x1b\\\x1b]l\x1b\\\x1b[9;5;12t" {
		t.Fatalf("embedded engine lacks attribute/window reports: %q", status.String())
	}
	probe.Write([]byte("\x1b]2;saved-title\a\x1b[22;2t\x1b]2;temporary-title\a\x1b[23;1t"))
	if probe.Title() != "saved-title" {
		t.Fatalf("embedded engine lacks title stacks: %q", probe.Title())
	}
	longTitle := strings.Repeat("t", 8192)
	probe.Write([]byte("\x1b]2;" + longTitle + "\a"))
	if probe.Title() != longTitle {
		t.Fatal("embedded engine still drops long OSC titles")
	}
	status.Reset()
	probe.Write([]byte("\x1b[?1003h\x1b[?9h\x1b[?1003$p\x1b[?9;1000$p"))
	if status.String() != "\x1b[?1003;2$y\x1b[?9;1$y" {
		t.Fatalf("embedded engine reports stale mouse modes: %q", status.String())
	}
	status.Reset()
	probe.SetFocus(false)
	probe.Write([]byte("\x1b[?1004h"))
	probe.SetFocus(true)
	if status.String() != "\x1b[O\x1b[I" {
		t.Fatalf("embedded engine lacks host focus reporting: %q", status.String())
	}
	status.Reset()
	probe.Write([]byte("\x1b[?12l\x1b[5 q\x1bc\x1b[?12$p"))
	if status.String() != "\x1b[?12;2$y" {
		t.Fatalf("embedded engine mixes cursor appearance and mode 12: %q", status.String())
	}
	cursorState, err := vt.NewRenderState()
	if err != nil {
		t.Fatal(err)
	}
	defer cursorState.Free()
	if err := cursorState.Update(probe); err != nil {
		t.Fatal(err)
	}
	if cursor := cursorState.Cursor(); cursor.Style != vt.CursorBar || !cursor.Blinking {
		t.Fatalf("embedded engine discarded cursor appearance on reset: %+v", cursor)
	}
	status.Reset()
	probe.Write([]byte("\x1b[8$p"))
	if status.String() != "\x1b[8;1$y" || !probe.BidiEnabled() {
		t.Fatal("embedded engine did not enable the production paragraph bridge")
	}
	probe.Write([]byte("\x1b[H\x1b[2 kRTL\r\nnext"))
	if !probe.BidiRowAt(0).RTL || !probe.BidiRowAt(1).RTL {
		t.Fatal("embedded engine lost paragraph direction on line feed")
	}
	probe.Reset()
	probe.Write([]byte("\x1b_Ga=T,U=1,f=24,s=1,v=1,i=7,p=1,c=2,r=1;/wAA\x1b\\"))
	probe.Write([]byte("\x1b_Ga=t,f=24,s=1,v=1,i=7;AP8A\x1b\\"))
	if g := probe.RenderGraphics(8, 16); len(g) != 1 || !g[0].Virtual || g[0].LogicalImageID() != 7 || g[0].Image.Pix[1] != 255 {
		t.Fatal("embedded engine lacks current-image lookup for Unicode placeholders")
	}
}

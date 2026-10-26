package terminal

import (
	"fmt"
	"io"
	"testing"
)

func TestEvidenceI11PromptHistoryFiniteMatrix(t *testing.T) {
	e := newPromptEditor(io.Discard)
	defer e.Close()
	for n := 0; n < 101; n++ {
		e.set(fmt.Sprintf("entry-%03d", n))
		e.key("\r")
	}
	if len(e.history) != 100 || e.history[0] != "entry-001" {
		t.Fatal("history eviction")
	}
	e.set("entry-100")
	e.key("\r")
	if len(e.history) != 100 || e.history[0] != "entry-001" {
		t.Fatal("adjacent duplicate added")
	}
	e.set(" ")
	e.key("\r")
	if e.history[99] != "entry-100" {
		t.Fatal("blank added")
	}
	e.set("entry-050")
	e.key("\r")
	if e.history[99] != "entry-050" || e.history[0] != "entry-002" {
		t.Fatal("nonadjacent duplicate suppressed")
	}
	e.set("草稿e\u0301")
	e.pos = 1
	e.key("\x12")
	if e.text() != "entry-050" {
		t.Fatal("empty search should match newest")
	}
	e.key("\x07")
	if e.text() != "草稿e\u0301" || e.pos != 1 {
		t.Fatal("cancel did not restore exact draft/cursor")
	}
	e.key("\x1b[A")
	historicalIndex := e.historyIndex
	e.pos = 3
	draft := e.text()
	e.key("\x12")
	e.key("not-found")
	if _, ready, _ := e.key("\r"); ready {
		t.Fatal("failed search executed stale result")
	}
	e.key("\x07")
	if e.historyIndex != historicalIndex || e.text() != draft || e.pos != 3 {
		t.Fatal("search cancel lost history position")
	}
	e.key("\x1b[B")
	if e.text() != "草稿e\u0301" {
		t.Fatal("history down did not recover draft")
	}
	empty := newPromptEditor(io.Discard)
	defer empty.Close()
	empty.set("untouched")
	empty.pos = 2
	empty.key("\x12")
	if _, ready, _ := empty.key("\r"); ready {
		t.Fatal("empty history accepted")
	}
	empty.key("\x1b")
	if empty.text() != "untouched" || empty.pos != 2 {
		t.Fatal("empty search did not restore draft")
	}
}

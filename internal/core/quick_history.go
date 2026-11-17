package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const DefaultQuickHistoryLimit = 500

type QuickHistoryEntry struct {
	ID           string `json:"-"`
	Text         string `json:"text"`
	AppendReturn bool   `json:"appendReturn"`
}

// Local, shared between windows, and excluded from sync/archives. Separate
// records keep page reads bounded without introducing a database dependency.
type QuickHistoryStore struct{ Directory string }

func (s QuickHistoryStore) names() ([]string, error) {
	entries, err := os.ReadDir(s.Directory)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.Type().IsRegular() && len(name) == 90 && name[20] == '-' && strings.HasSuffix(name, ".json") {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	slices.Reverse(names)
	return names, nil
}

func (s QuickHistoryStore) read(name string) (QuickHistoryEntry, error) {
	file, err := os.Open(filepath.Join(s.Directory, name))
	if err != nil {
		return QuickHistoryEntry{}, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (6<<16)+256))
	var entry QuickHistoryEntry
	if err == nil {
		err = json.Unmarshal(data, &entry)
	}
	if err == nil && (strings.TrimSpace(entry.Text) == "" || len(entry.Text) > 65536 || strings.ContainsRune(entry.Text, 0)) {
		err = fmt.Errorf("快速发送历史记录无效")
	}
	entry.ID = name
	return entry, err
}

func (s QuickHistoryStore) Page(before string) ([]QuickHistoryEntry, bool, error) {
	names, err := s.names()
	if err != nil {
		return nil, false, err
	}
	var page []QuickHistoryEntry
	size := 0
	for _, name := range names {
		if before != "" && name >= before {
			continue
		}
		entry, err := s.read(name)
		if err != nil {
			return nil, false, err
		}
		if len(page) == 50 || size+len(entry.Text) > 65536 {
			return page, true, nil
		}
		page = append(page, entry)
		size += len(entry.Text)
	}
	return page, false, nil
}

func (s QuickHistoryStore) Neighbor(current string, older bool) (QuickHistoryEntry, bool, error) {
	names, err := s.names()
	if err != nil {
		return QuickHistoryEntry{}, false, err
	}
	if !older {
		slices.Reverse(names)
	}
	for _, name := range names {
		if older && (current == "" || name < current) || !older && current != "" && name > current {
			entry, err := s.read(name)
			return entry, err == nil, err
		}
	}
	return QuickHistoryEntry{}, false, nil
}

func (s QuickHistoryStore) Remember(text string, appendReturn bool, limit int) error {
	if strings.TrimSpace(text) == "" || len(text) > 65536 || strings.ContainsRune(text, 0) {
		return nil
	}
	return s.update(&QuickHistoryEntry{Text: text, AppendReturn: appendReturn}, limit)
}

// Zero is the explicit Clear operation; preference limits are 1..100000.
func (s QuickHistoryStore) Trim(limit int) error { return s.update(nil, limit) }
func (s QuickHistoryStore) update(entry *QuickHistoryEntry, limit int) error {
	if limit < 0 || limit > 100000 {
		return fmt.Errorf("历史保留条数须为 1 至 100000")
	}
	if err := os.MkdirAll(s.Directory, 0700); err != nil {
		return err
	}
	release, err := lockStateDirectory(context.Background(), s.Directory, false)
	if err != nil {
		return err
	}
	defer release()
	names, err := s.names()
	if err != nil {
		return err
	}
	if entry != nil {
		data, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(data)
		suffix := "-" + hex.EncodeToString(digest[:]) + ".json"
		name := fmt.Sprintf("%020d%s", time.Now().UnixNano(), suffix)
		if err := AtomicWrite(filepath.Join(s.Directory, name), data); err != nil {
			return err
		}
		for i := len(names) - 1; i >= 0; i-- {
			if strings.HasSuffix(names[i], suffix) {
				if names[i] != name {
					if err := os.Remove(filepath.Join(s.Directory, names[i])); err != nil {
						return err
					}
				}
				names = slices.Delete(names, i, i+1)
			}
		}
		names = append([]string{name}, names...)
	}
	for _, name := range names[min(limit, len(names)):] {
		if err := os.Remove(filepath.Join(s.Directory, name)); err != nil {
			return err
		}
	}
	return nil
}

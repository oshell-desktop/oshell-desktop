package ftp

import (
	"testing"
	"time"
)

func TestOShellMLSDLinkKinds(t *testing.T) {
	for _, kind := range []string{"OS.unix=slink", "OS.unix=slink:/target", "OS.UNIX=SLINK"} {
		entry, err := parseRFC3659ListLine("type="+kind+";unix.mode=0777; link", time.Now(), time.UTC)
		if err != nil || entry.Type != EntryTypeLink {
			t.Fatal("link classified as ordinary file", kind, entry, err)
		}
	}
}

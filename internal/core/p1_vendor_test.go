package core

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func vendorZIPFixture(t *testing.T, method uint16, entries int, payload []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for i := 0; i < entries; i++ {
		f, err := w.CreateHeader(&zip.FileHeader{Name: fmt.Sprintf("sessions/%d.xsh", i), Method: method})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.Write(payload); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestP1VendorZIPFixedBoundaries(t *testing.T) {
	for _, method := range []uint16{zip.Store, zip.Deflate} {
		data := vendorZIPFixture(t, method, 1, []byte("[CONNECTION]\nProtocol=SSH\nHost=fixture.invalid\n"))
		r, err := readVendorZIP(data)
		if err != nil {
			t.Fatal("ordinary ZIP rejected", method, err)
		}
		if _, err = readVendorZIPEntry(r.File[0]); err != nil {
			t.Fatal(err)
		}
		central := bytes.Index(data, []byte{'P', 'K', 1, 2})
		end := bytes.LastIndex(data, []byte{'P', 'K', 5, 6})
		for _, tc := range []struct {
			name   string
			change func([]byte)
		}{
			{"different local name", func(b []byte) { b[30] ^= 1 }},
			{"different local method", func(b []byte) { b[8] = 99 }},
			{"different local size", func(b []byte) { binary.LittleEndian.PutUint32(b[18:], 17) }},
			{"zip64 size", func(b []byte) { binary.LittleEndian.PutUint32(b[central+20:], 0xffffffff) }},
			{"zip64 count", func(b []byte) { binary.LittleEndian.PutUint16(b[end+10:], 0xffff) }},
			{"split disk", func(b []byte) { binary.LittleEndian.PutUint16(b[end+4:], 1) }},
			{"encrypted", func(b []byte) { b[6] |= 1; b[central+8] |= 1 }},
			{"special Unix file", func(b []byte) { b[central+5] = 3; binary.LittleEndian.PutUint32(b[central+38:], uint32(0010000)<<16) }},
		} {
			t.Run(tc.name, func(t *testing.T) {
				copy := bytes.Clone(data)
				tc.change(copy)
				if _, err := readVendorZIP(copy); err == nil {
					t.Fatal("invalid ZIP accepted")
				}
			})
		}
	}
	for _, n := range []int{vendorZIPEntries, vendorZIPEntries + 1} {
		_, err := readVendorZIP(vendorZIPFixture(t, zip.Store, n, nil))
		if (err == nil) != (n == vendorZIPEntries) {
			t.Fatal("entry limit", n, err)
		}
	}
	for _, n := range []int{vendorEntryLimit, vendorEntryLimit + 1} {
		r, err := readVendorZIP(vendorZIPFixture(t, zip.Deflate, 1, bytes.Repeat([]byte{'x'}, n)))
		if (err == nil) != (n == vendorEntryLimit) {
			t.Fatal("size limit", n, err)
		}
		if err == nil {
			if _, err = readVendorZIPEntry(r.File[0]); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestP1VendorZIPDirectoryCRCAndEntryCRC(t *testing.T) {
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	w.Create("sessions/")
	w.Close()
	data := b.Bytes()
	if _, err := readVendorZIP(data); err != nil {
		t.Fatal(err)
	}
	central := bytes.Index(data, []byte{'P', 'K', 1, 2})
	binary.LittleEndian.PutUint32(data[14:], 1)
	binary.LittleEndian.PutUint32(data[central+16:], 1)
	if _, err := readVendorZIP(data); err == nil {
		t.Fatal("directory CRC accepted")
	}
	data = vendorZIPFixture(t, zip.Store, 1, []byte("fixture"))
	central = bytes.Index(data, []byte{'P', 'K', 1, 2})
	// Corrupt actual payload while keeping matching local/central descriptors.
	start := 30 + int(binary.LittleEndian.Uint16(data[26:])) + int(binary.LittleEndian.Uint16(data[28:]))
	data[start] ^= 1
	r, err := readVendorZIP(data)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = readVendorZIPEntry(r.File[0]); err == nil {
		t.Fatal("bad payload CRC accepted")
	}
}

func TestP1VendorDirectoryTraversalLimitIncludesDirectories(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < vendorDirectoryEntries; i++ {
		if err := os.Mkdir(filepath.Join(root, fmt.Sprint(i)), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ReadThirdParty(context.Background(), []string{root}, "Xshell"); err != nil {
		t.Fatal("8000 directory boundary", err)
	}
	if err := os.Mkdir(filepath.Join(root, "overflow"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadThirdParty(context.Background(), []string{root}, "Xshell"); err == nil {
		t.Fatal("8001 directories accepted")
	}
}

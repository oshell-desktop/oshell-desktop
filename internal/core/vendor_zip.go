package core

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"sort"
)

const vendorZIPEntries = 4000
const vendorDirectoryEntries = 8000
const vendorEntryLimit = 2 << 20

func readVendorZIPEntry(f *zip.File) ([]byte, error) {
	r, err := f.OpenRaw()
	if err != nil {
		return nil, err
	}
	if f.Method == zip.Deflate {
		decoder := flate.NewReader(r)
		defer decoder.Close()
		r = decoder
	}
	data, err := io.ReadAll(io.LimitReader(r, vendorEntryLimit+1))
	if err != nil {
		return nil, err
	}
	if len(data) > vendorEntryLimit || uint64(len(data)) != f.UncompressedSize64 || crc32.ChecksumIEEE(data) != f.CRC32 {
		return nil, fmt.Errorf("ZIP 条目展开长度或 CRC 无效")
	}
	return data, nil
}

// Validate the physical records before archive/zip interprets them. Go's ZIP
// reader intentionally accepts ZIP64 and does not compare local filenames or
// directory CRCs; those are outside the fixed SF-E06 container contract.
func readVendorZIP(data []byte) (*zip.Reader, error) {
	bad := func() (*zip.Reader, error) {
		return nil, fmt.Errorf("ZIP 记录不一致或格式不支持（仅普通 stored/deflate，禁止 ZIP64、分卷和加密）")
	}
	u16 := func(b []byte) int { return int(binary.LittleEndian.Uint16(b)) }
	u32 := func(b []byte) uint64 { return uint64(binary.LittleEndian.Uint32(b)) }
	end := -1
	for at := len(data) - 22; at >= max(0, len(data)-22-65535); at-- {
		if bytes.Equal(data[at:at+4], []byte{'P', 'K', 5, 6}) && at+22+u16(data[at+20:]) == len(data) {
			end = at
			break
		}
	}
	if end < 0 {
		return bad()
	}
	e := data[end:]
	count, offset, length := u16(e[10:]), u32(e[16:]), u32(e[12:])
	if u16(e[4:]) != 0 || u16(e[6:]) != 0 || u16(e[8:]) != count || count > vendorZIPEntries || offset+length != uint64(end) {
		return bad()
	}
	validExtra := func(extra []byte) bool {
		for len(extra) > 0 {
			if len(extra) < 4 || u16(extra) == 1 || u16(extra[2:]) > len(extra)-4 {
				return false
			}
			extra = extra[4+u16(extra[2:]):]
		}
		return true
	}
	type span struct{ begin, end uint64 }
	var spans []span
	at := offset
	for i := 0; i < count; i++ {
		if at+46 > uint64(end) || u32(data[at:]) != 0x02014b50 {
			return bad()
		}
		c := data[at:]
		n, x, comment := u16(c[28:]), u16(c[30:]), u16(c[32:])
		next := at + 46 + uint64(n+x+comment)
		compressed, expanded, local := u32(c[20:]), u32(c[24:]), u32(c[42:])
		flags, method, crc := u16(c[8:]), u16(c[10:]), u32(c[16:])
		if next > uint64(end) || u16(c[34:]) != 0 || flags&(1|0x40) != 0 || (method != int(zip.Store) && method != int(zip.Deflate)) || compressed == 0xffffffff || expanded > vendorEntryLimit || !validExtra(c[46+n:46+n+x]) || local+30 > offset {
			return bad()
		}
		l := data[local:]
		ln, lx := u16(l[26:]), u16(l[28:])
		start := local + 30 + uint64(ln+lx)
		finish := start + compressed
		if u32(l) != 0x04034b50 || start > offset || finish > offset || u16(l[6:]) != flags || u16(l[8:]) != method || !bytes.Equal(l[30:30+ln], c[46:46+n]) || !validExtra(l[30+ln:30+ln+lx]) {
			return bad()
		}
		lc, ls, lu := u32(l[14:]), u32(l[18:]), u32(l[22:])
		if flags&8 == 0 {
			if lc != crc || ls != compressed || lu != expanded {
				return bad()
			}
		} else {
			if (lc != 0 && lc != crc) || (ls != 0 && ls != compressed) || (lu != 0 && lu != expanded) {
				return bad()
			}
			if finish+12 > offset {
				return bad()
			}
			if u32(data[finish:]) == 0x08074b50 {
				finish += 4
			}
			if finish+12 > offset || u32(data[finish:]) != crc || u32(data[finish+4:]) != compressed || u32(data[finish+8:]) != expanded {
				return bad()
			}
			finish += 12
		}
		spans = append(spans, span{local, finish})
		at = next
	}
	if at != uint64(end) {
		return bad()
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].begin < spans[j].begin })
	for i, s := range spans {
		if (i == 0 && s.begin != 0) || (i > 0 && spans[i-1].end > s.begin) {
			return bad()
		}
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	if len(reader.File) != count {
		return bad()
	}
	for _, f := range reader.File {
		if f.Mode().Type() != 0 && !f.Mode().IsDir() {
			return bad()
		}
		if f.Mode().IsDir() && (f.UncompressedSize64 != 0 || f.CRC32 != 0) {
			return bad()
		}
	}
	return reader, nil
}

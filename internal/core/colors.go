// SPDX-License-Identifier: GPL-3.0-only
package core

import (
	_ "embed"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf16"
)

type ColorScheme struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Background    string   `json:"background"`
	Foreground    string   `json:"foreground"`
	Cursor        string   `json:"cursor"`
	CursorText    string   `json:"cursorText"`
	AutoCursor    bool     `json:"autoCursor"` // Preserved for shared configuration round trips.
	Selection     string   `json:"selection"`
	SelectionText string   `json:"selectionText"`
	ANSI          []string `json:"ansi"`
}

//go:embed colors-presets.json
var presetJSON []byte

func ColorPresets() []ColorScheme {
	var out []ColorScheme
	json.Unmarshal(presetJSON, &out)
	return out
}
func (s ColorScheme) Validate() error {
	if s.ID == "" || strings.TrimSpace(s.Name) == "" || len([]rune(s.Name)) > 80 || len(s.ANSI) != 16 {
		return fmt.Errorf("配色需要有效名称和 16 个 ANSI 颜色")
	}
	colors := append([]string{s.Background, s.Foreground, s.Cursor, s.CursorText, s.Selection, s.SelectionText}, s.ANSI...)
	for _, c := range colors {
		if len(c) != 7 || c[0] != '#' {
			return fmt.Errorf("颜色应为 #RRGGBB")
		}
		if _, e := hex.DecodeString(c[1:]); e != nil {
			return e
		}
	}
	return nil
}
func (s ColorScheme) Copy() ColorScheme {
	s.ID = "custom-" + NewID()
	s.Name += " · 自定义"
	s.ANSI = append([]string(nil), s.ANSI...)
	return s
}
func (s Settings) Schemes() []ColorScheme { return append(ColorPresets(), s.CustomSchemes...) }
func (s Settings) Scheme() ColorScheme {
	for _, c := range s.Schemes() {
		if c.ID == s.ColorSchemeID && c.Validate() == nil {
			return c
		}
	}
	return ColorPresets()[0]
}
func ImportColors(data []byte) ([]ColorScheme, error) {
	if len(data) > 65536 {
		return nil, fmt.Errorf("配色文件不能超过 64 KiB")
	}
	var one ColorScheme
	if json.Unmarshal(data, &one) == nil && one.ID != "" {
		if e := one.Validate(); e != nil {
			return nil, e
		}
		return []ColorScheme{one.Copy()}, nil
	}
	if len(data) > 2 && ((data[0] == 255 && data[1] == 254) || (data[0] == 254 && data[1] == 255)) {
		var words []uint16
		little := data[0] == 255
		for i := 2; i+1 < len(data); i += 2 {
			if little {
				words = append(words, binary.LittleEndian.Uint16(data[i:]))
			} else {
				words = append(words, binary.BigEndian.Uint16(data[i:]))
			}
		}
		data = []byte(string(utf16.Decode(words)))
	}
	var result []ColorScheme
	var fields map[string]string
	name := ""
	flush := func() error {
		if fields == nil || strings.EqualFold(name, "names") {
			return nil
		}
		scheme := ColorPresets()[0].Copy()
		scheme.Name = name
		pick := func(key string) (string, error) {
			v, ok := fields[key]
			if !ok {
				return "", fmt.Errorf("Xshell 配色缺少 %s", key)
			}
			return "#" + strings.ToUpper(strings.TrimPrefix(v, "#")), nil
		}
		var e error
		scheme.Background, e = pick("background")
		if e != nil {
			return e
		}
		scheme.Foreground, e = pick("text")
		if e != nil {
			return e
		}
		keys := []string{"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white"}
		for i := 0; i < 16; i++ {
			key := keys[i%8]
			if i >= 8 {
				key += "(bold)"
			}
			scheme.ANSI[i], e = pick(key)
			if e != nil {
				return e
			}
		}
		scheme.Cursor = scheme.Foreground
		scheme.CursorText = scheme.Background
		scheme.Selection = "#405A78"
		scheme.SelectionText = "#FFFFFF"
		if e = scheme.Validate(); e != nil {
			return e
		}
		rgb, _ := hex.DecodeString(scheme.Background[1:])
		if float64(rgb[0])*.2126+float64(rgb[1])*.7152+float64(rgb[2])*.0722 >= 128 {
			scheme.Selection = "#C7D8EC"
			scheme.SelectionText = "#20252A"
		}
		result = append(result, scheme)
		return nil
	}

	for _, raw := range strings.Split(strings.TrimPrefix(string(data), "\ufeff"), "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			if e := flush(); e != nil {
				return nil, e
			}
			name = strings.TrimSuffix(strings.TrimPrefix(line, "["), "]")
			fields = map[string]string{}
		} else if k, v, ok := strings.Cut(line, "="); ok && fields != nil {
			fields[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
		}
	}
	if e := flush(); e != nil {
		return nil, e
	}
	if len(result) == 0 || len(result) > 64 {
		return nil, fmt.Errorf("未找到有效配色方案")
	}
	return result, nil
}

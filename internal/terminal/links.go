package terminal

import (
	"net/url"
	"strings"
)

// OShellTerminal.requestOpenLink accepts only these external URL schemes.
func terminalLinkAllowed(link string) bool {
	u, err := url.Parse(link)
	if err != nil {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "mailto":
		return true
	}
	return false
}

// linkAt uses logical cell coordinates and requires the terminal lock.
func (v *view) linkAt(col, row int) string {
	return v.linkMatchAt(col, row).text
}

type linkRowRange struct{ row, start, end int }
type terminalLink struct {
	text     string
	explicit bool
	ranges   []linkRowRange // absolute buffer rows
}

func (m terminalLink) clickURL() string {
	if m.explicit {
		// AppleTerminalView.urlAndParamsFrom splits every semicolon and
		// takes the URL component immediately after the parameter component.
		url, _, _ := strings.Cut(m.text, ";")
		return url
	}
	return m.text
}

func (v *view) linkMatchAt(col, row int) terminalLink {
	if ref, ok := v.t.term.CellAt(col, row); ok {
		start, end, found := v.t.term.HyperlinkRange(col, row)
		if link := v.t.term.Hyperlink(ref); link != "" || found {
			m := terminalLink{text: link, explicit: true}
			if found {
				m.ranges = []linkRowRange{{row + int(v.t.term.Scrollbar().Offset), start, end}}
			}
			return m
		}
	}
	return v.implicitLinkMatchAt(col, row)
}

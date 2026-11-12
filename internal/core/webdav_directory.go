package core

import (
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"net/url"
	"path"
	"strings"
)

func webDAVChild(directory *url.URL, name string) *url.URL {
	child := *directory
	child.Path += name
	if child.RawPath != "" {
		child.RawPath += name
	}
	return &child
}

// SyncDirectory preserves old direct-file endpoints. New targets use an oshell
// collection. The configured address and its credential identity never change.
func (w *WebDAVTransport) SyncDirectory(ctx context.Context) (*url.URL, error) {
	w.resolveMu.Lock()
	defer w.resolveMu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if w.resolvedDirectory != nil {
		copy := *w.resolvedDirectory
		return &copy, nil
	}
	target := w.directory
	explicit := strings.EqualFold(path.Base(strings.TrimRight(target.Path, "/")), "oshell")
	if !explicit {
		legacy, err := w.fetchIn(ctx, target)
		if err != nil {
			return nil, err
		}
		if legacy != nil {
			w.resolvedDirectory = target
			copy := *target
			return &copy, nil
		}
		target = webDAVChild(target, "oshell/")
	}
	exists, err := w.collectionExists(ctx, target)
	if err != nil {
		return nil, err
	}
	if !exists {
		status, _, _, err := w.request(ctx, "MKCOL", target, nil, nil)
		if err != nil {
			return nil, err
		}
		if status != 201 && status != 405 {
			return nil, &SyncFailure{Kind: "status", Status: status}
		}
		// 405 can also mean a regular file exists. Revalidate even after 201,
		// and accept concurrent creation only when this exact collection exists.
		if exists, err = w.collectionExists(ctx, target); err != nil {
			return nil, err
		} else if !exists {
			return nil, safeSyncFailure("webdav-directory")
		}
	}
	w.resolvedDirectory = target
	copy := *target
	return &copy, nil
}

func (w *WebDAVTransport) collectionExists(ctx context.Context, target *url.URL) (bool, error) {
	body := []byte(`<?xml version="1.0"?><d:propfind xmlns:d="DAV:"><d:prop><d:resourcetype/></d:prop></d:propfind>`)
	status, _, data, err := w.request(ctx, "PROPFIND", target, body, map[string]string{"Depth": "0", "Content-Type": "application/xml; charset=utf-8"})
	if err != nil {
		return false, err
	}
	if status == 404 {
		return false, nil
	}
	if status != 207 {
		return false, &SyncFailure{Kind: "status", Status: status}
	}
	if !validDAVCollection(data, target) {
		return false, safeSyncFailure("webdav-directory")
	}
	return true, nil
}

func validDAVCollection(data []byte, target *url.URL) bool {
	collection, _, ok := davProperties(data, target)
	return ok && collection
}

func davProperties(data []byte, target *url.URL) (bool, string, bool) {
	if len(data) > 1<<20 {
		return false, "", false
	}
	var document struct {
		XMLName   xml.Name `xml:"DAV: multistatus"`
		Responses []struct {
			Hrefs      []string `xml:"DAV: href"`
			Properties []struct {
				Status string `xml:"DAV: status"`
				Prop   struct {
					ETags []string `xml:"DAV: getetag"`
					Type  struct {
						Collections []struct{} `xml:"DAV: collection"`
					} `xml:"DAV: resourcetype"`
				} `xml:"DAV: prop"`
			} `xml:"DAV: propstat"`
		} `xml:"DAV: response"`
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	if decoder.Decode(&document) != nil {
		return false, "", false
	}
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return false, "", false
		}
		switch value := token.(type) {
		case xml.Comment, xml.ProcInst:
		case xml.CharData:
			if strings.TrimSpace(string(value)) != "" {
				return false, "", false
			}
		default:
			return false, "", false
		}
	}
	matches, collection := 0, false
	var etags []string
	for _, response := range document.Responses {
		if len(response.Hrefs) != 1 {
			return false, "", false
		}
		href, err := url.Parse(strings.TrimSpace(response.Hrefs[0]))
		if err != nil || strings.TrimSpace(response.Hrefs[0]) == "" {
			return false, "", false
		}
		actual := target.ResolveReference(href)
		ah, ap, ae := webDAVEndpoint(actual)
		th, tp, te := webDAVEndpoint(target)
		if ae != nil || te != nil || actual.User != nil || actual.RawQuery != "" || actual.Fragment != "" ||
			!strings.EqualFold(actual.Scheme, target.Scheme) || ah != th || ap != tp ||
			strings.TrimRight(actual.Path, "/") != strings.TrimRight(target.Path, "/") {
			continue
		}
		matches++
		for _, property := range response.Properties {
			status := strings.Fields(property.Status)
			if len(status) >= 2 && strings.HasPrefix(status[0], "HTTP/") && status[1] == "200" {
				collection = collection || len(property.Prop.Type.Collections) > 0
				etags = append(etags, property.Prop.ETags...)
			}
		}
	}
	tag := ""
	if len(etags) == 1 {
		tag = strings.TrimSpace(etags[0])
	}
	return collection, tag, matches == 1
}

// ABOUTME: Resolves document and resource links from each logical source parent.
// ABOUTME: Rewrites only supported passive URLs and explains unresolved searches.
package preview

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/net/html"
)

type reference struct {
	url              *url.URL
	path, search, id string
	local            bool
}

func parseReference(value, parent string) (reference, error) {
	r := reference{}
	if strings.HasPrefix(value, "id:") {
		r.id = strings.TrimPrefix(value, "id:")
		return r, nil
	}
	path, search, found := strings.Cut(value, "::")
	if found && !strings.Contains(path, "://") {
		value = path
		r.search = search
	}
	u, err := url.Parse(value)
	if err != nil {
		return r, err
	}
	r.url = u
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "mailto":
		return r, nil
	case "", "file":
		if u.Host != "" && u.Host != "localhost" {
			return r, errors.New("unsupported source file authority")
		}
		if u.Path == "" {
			return r, nil
		}
		if u.Opaque != "" {
			return r, errors.New("opaque file URL is unsupported")
		}
		r.path = u.Path
		if !filepath.IsAbs(r.path) {
			r.path = filepath.Join(parent, r.path)
		}
		r.path = filepath.Clean(r.path)
		r.local = true
		return r, nil
	case "data":
		return r, nil
	default:
		return r, errors.New("unsupported URL scheme")
	}
}

func (s *session) resolve(ctx context.Context, p *page) error {
	var refs []*html.Node
	for n := range p.dom.Descendants() {
		if n.Type == html.ElementNode && (n.Data == "a" || n.Data == "img") {
			refs = append(refs, n)
		}
	}
	for _, n := range refs {
		if err := ctx.Err(); err != nil {
			return err
		}
		key := "href"
		if n.Data == "img" {
			key = "src"
		}
		value := attribute(n, key)
		if value == "" {
			continue
		}
		r, err := parseReference(value, filepath.Dir(p.source.logical))
		if err != nil {
			s.inactive(p, n, key, "unsupported or malformed reference")
			continue
		}
		if n.Data == "img" && !r.local {
			if allowedRaster(value) {
				continue
			}
			if r.url != nil && (r.url.Scheme == "http" || r.url.Scheme == "https") {
				n.Data = "a"
				n.DataAtom = 0
				removeAttribute(n, "src")
				setAttribute(n, "href", value)
				n.AppendChild(nodeText("[Remote image: " + attribute(n, "alt") + "]"))
				s.log.warn("%q: remote image retained as a link", p.source.logical)
			} else {
				s.inactive(p, n, key, "unsupported image reference")
			}
			continue
		}
		if !r.local && r.id == "" {
			if r.url != nil && r.url.Scheme == "data" {
				s.inactive(p, n, key, "data anchors are unsupported")
			}
			continue
		}
		target, fragment := s.target(p, r)
		if r.id != "" && target == nil {
			s.inactive(p, n, key, "unknown or ambiguous Org ID")
			continue
		}
		var destination string
		if target != nil {
			destination = target.url
		} else {
			destination, err = s.desktop.fileURL(ctx, r.path)
			if err != nil {
				s.inactive(p, n, key, "original path cannot be represented for the browser")
				continue
			}
			if _, err := os.Stat(r.path); err != nil {
				s.log.warn("%q: missing or unavailable target %q", p.source.logical, r.path)
			}
		}
		u, err := url.Parse(destination)
		if err != nil {
			return err
		}
		if r.url != nil {
			u.RawQuery = r.url.RawQuery
			u.Fragment = r.url.Fragment
		}
		if fragment != "" {
			u.Fragment = fragment
		}
		if r.search != "" && fragment == "" {
			u.Fragment = ""
			s.explain(p, n, "Org search not resolved: "+r.search)
		}
		setAttribute(n, key, u.String())
	}
	return nil
}

func (s *session) target(p *page, r reference) (*page, string) {
	if r.id != "" {
		var match *page
		fragment := ""
		count := 0
		for _, candidate := range s.pages {
			if candidate.ready {
				for _, id := range candidate.orgIDs[r.id] {
					match = candidate
					fragment = id
					count++
				}
			}
		}
		if count == 1 {
			return match, fragment
		}
		return nil, ""
	}
	src, err := identify(r.path, "")
	if err != nil {
		return nil, ""
	}
	target := s.byKey[src.key]
	if target == nil || !target.ready {
		return nil, ""
	}
	if r.search == "" {
		return target, ""
	}
	var ids []string
	if search, ok := strings.CutPrefix(r.search, "#"); ok {
		ids = target.ids[search]
	} else if search, ok := strings.CutPrefix(r.search, "*"); ok {
		ids = target.headings[strings.Join(strings.Fields(search), " ")]
	}
	if len(ids) == 1 {
		return target, ids[0]
	}
	return nil, ""
}
func allowedRaster(value string) bool {
	lower := strings.ToLower(value)
	for _, mime := range []string{"png", "jpeg", "gif", "webp", "avif"} {
		if strings.HasPrefix(lower, "data:image/"+mime+";base64,") {
			return true
		}
	}
	return false
}
func (s *session) inactive(p *page, n *html.Node, key, reason string) {
	removeAttribute(n, key)
	if n.Data == "img" {
		n.Data = "span"
		n.DataAtom = 0
		n.AppendChild(nodeText(attribute(n, "alt")))
	}
	s.explain(p, n, reason)
}
func (s *session) explain(p *page, n *html.Node, reason string) {
	label := &html.Node{Type: html.ElementNode, Data: "span", Attr: []html.Attribute{{Key: "class", Val: "hp-explanation"}}}
	label.AppendChild(nodeText(" [" + reason + "]"))
	if n.Parent != nil {
		n.Parent.InsertBefore(label, n.NextSibling)
	}
	s.log.warn("%q: %s", p.source.logical, reason)
}

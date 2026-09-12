// ABOUTME: Preserves authored HTML layout under a separate script-free resource policy.
// ABOUTME: Resolves base URLs, local stylesheets and raster sources before publication.
package preview

import (
	"bytes"
	"errors"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

const nativeElements = passiveElements + " html head body title style meta article aside footer header main nav address dl dt dd hgroup search picture label button select option optgroup textarea input form"
const nativeAttrs = passiveAttrs + " style role aria-label aria-labelledby aria-describedby aria-hidden aria-level tabindex datetime open reversed sizes media target rel checked disabled selected multiple placeholder name"
const nativePolicy = "default-src 'none'; script-src 'none'; style-src 'unsafe-inline'; img-src data:; font-src 'none'; form-action 'none'; base-uri 'none'; object-src 'none'"

func resolveHTMLReference(base, value string) string {
	b, err := url.Parse(base)
	if err != nil {
		return value
	}
	u, err := url.Parse(value)
	if err != nil {
		return value
	}
	return b.ResolveReference(u).String()
}

func (s *session) renderNativeHTML(p *page, data []byte) error {
	if int64(len(data)) > s.cfg.outputBytes-s.used {
		return errors.New("native HTML exceeds remaining byte budget")
	}
	s.used += int64(len(data))
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return err
	}
	base := fileReference(p.source.logical)
	for n := range doc.Descendants() {
		if n.Type == html.ElementNode && n.Data == "base" && attribute(n, "href") != "" {
			u, err := url.Parse(attribute(n, "href"))
			if err == nil && (u.Scheme == "" || u.Scheme == "file" || u.Scheme == "http" || u.Scheme == "https") {
				base = resolveHTMLReference(base, u.String())
				break
			}
		}
	}
	p.dom = doc
	s.cleanNativeChildren(p, doc, base)
	head := element(doc, "head")
	contentPolicy := nativePolicy
	if s.cfg.httpOrigin != "" {
		contentPolicy = strings.Replace(contentPolicy, "img-src data:", "img-src "+s.cfg.httpOrigin+" data:", 1)
	}
	policy := &html.Node{Type: html.ElementNode, Data: "meta", Attr: []html.Attribute{{Key: "http-equiv", Val: "Content-Security-Policy"}, {Key: "content", Val: contentPolicy}}}
	head.InsertBefore(policy, head.FirstChild)
	charset := &html.Node{Type: html.ElementNode, Data: "meta", Attr: []html.Attribute{{Key: "charset", Val: "utf-8"}}}
	head.InsertBefore(charset, head.FirstChild)
	p.ids = make(map[string][]string)
	p.headings = make(map[string][]string)
	p.orgIDs = make(map[string][]string)
	for n := range doc.Descendants() {
		if id := attribute(n, "id"); id != "" {
			p.ids[id] = append(p.ids[id], id)
		}
		if isHeading(n) && attribute(n, "id") != "" {
			title := strings.Join(strings.Fields(contentText(n)), " ")
			p.headings[title] = append(p.headings[title], attribute(n, "id"))
		}
	}
	return nil
}

func (s *session) cleanNativeChildren(p *page, parent *html.Node, base string) {
	for n := parent.FirstChild; n != nil; {
		next := n.NextSibling
		if n.Type == html.CommentNode {
			parent.RemoveChild(n)
		} else if n.Type == html.ElementNode {
			s.cleanNativeNode(p, n, base)
		}
		n = next
	}
}

func (s *session) cleanNativeNode(p *page, n *html.Node, base string) {
	if n.Namespace != "" || strings.Contains(" script iframe frame frameset object embed svg math base audio video canvas source track template noscript ", " "+n.Data+" ") {
		s.omitNative(p, n, "unsupported "+n.Data+" content")
		return
	}
	if n.Data == "link" {
		s.nativeStylesheet(p, n, base)
		return
	}
	if n.Data == "meta" {
		name := strings.ToLower(attribute(n, "name"))
		if attribute(n, "http-equiv") != "" || attribute(n, "charset") != "" || (name != "viewport" && name != "description") {
			n.Parent.RemoveChild(n)
		} else {
			n.Attr = []html.Attribute{{Key: "name", Val: name}, {Key: "content", Val: attribute(n, "content")}}
		}
		return
	}
	if !strings.Contains(" "+nativeElements+" ", " "+n.Data+" ") {
		// Unknown wrappers have no execution authority; preserve their plain children.
		n.Data, n.DataAtom = "div", 0
	}
	if n.Data == "form" {
		n.Data, n.DataAtom = "div", 0
	}
	if n.Data == "input" {
		n.Data, n.DataAtom = "span", 0
		n.AppendChild(nodeText(attribute(n, "value")))
	}
	if n.Data == "textarea" {
		n.Data, n.DataAtom = "pre", 0
	}
	if n.Data == "button" || n.Data == "select" {
		setAttribute(n, "disabled", "")
	}
	if n.Data == "style" {
		clean := s.passiveCSS(p, contentText(n), false, base)
		for n.FirstChild != nil {
			n.RemoveChild(n.FirstChild)
		}
		n.AppendChild(nodeText(clean))
	}
	style := attribute(n, "style")
	srcset := attribute(n, "srcset")
	attrs := n.Attr[:0]
	for _, a := range n.Attr {
		if a.Namespace == "" && strings.Contains(" "+nativeAttrs+" ", " "+a.Key+" ") {
			attrs = append(attrs, a)
		}
	}
	n.Attr = attrs
	if style != "" {
		setAttribute(n, "style", s.passiveCSS(p, style, true, base))
	}
	if n.Data == "a" {
		value := attribute(n, "href")
		if value != "" {
			resolved := resolveHTMLReference(base, value)
			if r, err := parseReference(resolved, filepath.Dir(p.source.logical)); err != nil || (r.url != nil && r.url.Scheme == "data") {
				removeAttribute(n, "href")
			} else {
				setAttribute(n, "href", resolved)
			}
		}
		setAttribute(n, "rel", "noopener noreferrer")
	}
	if n.Data == "img" {
		s.nativeImage(p, n, base, srcset)
	}
	s.cleanNativeChildren(p, n, base)
}

func (s *session) omitNative(p *page, n *html.Node, reason string) {
	placeholder := nodeText("[" + reason + " omitted]")
	if n.Parent.Data == "head" {
		body := element(p.dom, "body")
		body.InsertBefore(placeholder, body.FirstChild)
	} else {
		n.Parent.InsertBefore(placeholder, n)
	}
	n.Parent.RemoveChild(n)
	s.log.notice("%q: %s omitted", p.source.logical, reason)
}

func (s *session) nativeStylesheet(p *page, n *html.Node, base string) {
	if strings.ToLower(attribute(n, "rel")) != "stylesheet" {
		s.omitNative(p, n, "unsupported resource link")
		return
	}
	resolved := resolveHTMLReference(base, attribute(n, "href"))
	r, err := parseReference(resolved, filepath.Dir(p.source.logical))
	if err != nil || !r.local {
		s.omitNative(p, n, "non-local stylesheet")
		return
	}
	data, err := s.localAsset(p, r.path)
	if err != nil {
		s.omitNative(p, n, "unavailable or denied stylesheet")
		return
	}
	if err := validateText(data); err != nil {
		s.omitNative(p, n, "invalid stylesheet encoding")
		return
	}
	clean := s.passiveCSS(p, string(data), false, fileReference(r.path))
	n.Data, n.DataAtom = "style", 0
	n.Attr = nil
	n.AppendChild(nodeText(clean))
}

func (s *session) nativeImage(p *page, n *html.Node, base, srcset string) {
	if srcset != "" {
		var candidates []string
		for _, candidate := range parseSourceSet(srcset) {
			resolved, err := s.imageURL(p, resolveHTMLReference(base, candidate.url), "")
			if err != nil {
				s.log.notice("%q: srcset image omitted", p.source.logical)
				continue
			}
			candidates = append(candidates, resolved+" "+candidate.descriptor)
		}
		if len(candidates) > 0 {
			setAttribute(n, "srcset", strings.Join(candidates, ", "))
		}
	}
	value := attribute(n, "src")
	if value == "" && attribute(n, "srcset") != "" {
		return
	}
	resolved, err := s.imageURL(p, resolveHTMLReference(base, value), "")
	if err != nil {
		if attribute(n, "srcset") != "" {
			removeAttribute(n, "src")
			return
		}
		s.inactive(p, n, "src", err.Error())
	} else {
		setAttribute(n, "src", resolved)
		p.images[filepath.Dir(p.source.logical)+"\x00"+resolved] = resolved
	}
}

type sourceCandidate struct{ url, descriptor string }

// Parse the URL-plus-descriptor subset of HTML srcset. A URL consumes commas
// within a non-whitespace token (including data URLs); trailing commas separate
// candidates. Only one positive width or density descriptor is supported.
func parseSourceSet(input string) []sourceCandidate {
	var result []sourceCandidate
	for len(input) > 0 {
		input = strings.TrimLeft(input, " \t\r\n\f,")
		if input == "" {
			break
		}
		end := strings.IndexAny(input, " \t\r\n\f")
		if end < 0 {
			end = len(input)
		}
		value := input[:end]
		input = input[end:]
		if strings.HasSuffix(value, ",") {
			result = append(result, sourceCandidate{strings.TrimRight(value, ","), ""})
			continue
		}
		end = strings.IndexByte(input, ',')
		if end < 0 {
			end = len(input)
		}
		descriptor := strings.TrimSpace(input[:end])
		input = input[end:]
		if descriptor == "" {
			result = append(result, sourceCandidate{value, ""})
			continue
		}
		suffix := descriptor[len(descriptor)-1]
		number, err := strconv.ParseFloat(descriptor[:len(descriptor)-1], 64)
		if err == nil && number > 0 && (suffix == 'x' || (suffix == 'w' && !strings.ContainsAny(descriptor, ".eE+-"))) {
			result = append(result, sourceCandidate{value, descriptor})
		}
	}
	return result
}

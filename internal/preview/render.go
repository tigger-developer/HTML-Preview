// ABOUTME: Renders bounded source snapshots with isolated packaged Pandoc inputs.
// ABOUTME: Restores source metadata before final passive HTML publication.
package preview

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	bundle "github.com/tigger-developer/HTML-Preview"
	"golang.org/x/net/html"
)

type page struct {
	source             sourceContext
	name, url, startup string
	dom                *html.Node
	ready              bool
	ids, headings      map[string][]string
	orgIDs             map[string][]string
}

func (s *session) render(ctx context.Context, p *page, data []byte) error {
	format := markdownDialect
	preserved := preservation{startup: "showall"}
	if strings.EqualFold(filepath.Ext(p.source.logical), ".org") {
		format = "org"
		preserved = preserveOrg(data)
		data = []byte(preserved.text)
	}
	p.startup = preserved.startup
	for _, warning := range preserved.warnings {
		s.log.warn("%q: %s", p.source.logical, warning)
	}
	args := []string{"--defaults=" + filepath.Join(s.path, "defaults.yaml"), "--data-dir=" + s.path, "--from=" + format, "--lua-filter=" + filepath.Join(s.path, "fidelity.lua"), "+RTS", "-M512M", "-RTS"}
	output, err := s.host.Execute(ctx, Command{Path: s.pandoc, Args: args, Input: data, Dir: s.path, Limit: s.cfg.outputBytes - s.used})
	if err != nil {
		return fmt.Errorf("Pandoc conversion: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Staging contributes to the session budget before it is written.
	if err := s.write(p.name+".stage", output); err != nil {
		return err
	}
	doc, err := html.Parse(bytes.NewReader(output))
	if err != nil {
		return err
	}
	s.scrub(doc, p)
	p.dom = element(doc, "body")
	if p.dom == nil {
		return errors.New("Pandoc produced no HTML body")
	}
	p.ids = make(map[string][]string)
	p.headings = make(map[string][]string)
	p.orgIDs = make(map[string][]string)
	if err := restore(p, preserved); err != nil {
		return err
	}
	catalogue(p, s.log)
	return nil
}

func element(root *html.Node, name string) *html.Node {
	for n := range root.Descendants() {
		if n.Type == html.ElementNode && n.Data == name {
			return n
		}
	}
	return nil
}
func attribute(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
func setAttribute(n *html.Node, key, value string) {
	for i := range n.Attr {
		if n.Attr[i].Key == key {
			n.Attr[i].Val = value
			return
		}
	}
	n.Attr = append(n.Attr, html.Attribute{Key: key, Val: value})
}
func removeAttribute(n *html.Node, key string) {
	out := n.Attr[:0]
	for _, a := range n.Attr {
		if a.Key != key {
			out = append(out, a)
		}
	}
	n.Attr = out
}
func contentText(root *html.Node) string {
	var b strings.Builder
	for n := range root.Descendants() {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
	}
	return b.String()
}
func nodeText(text string) *html.Node { return &html.Node{Type: html.TextNode, Data: text} }

func restore(p *page, preserved preservation) error {
	markers := make(map[string]*orgHeading)
	for _, h := range preserved.heads {
		markers[h.marker] = h
	}
	var candidates []*html.Node
	for n := range p.dom.Descendants() {
		if n.Type == html.ElementNode && n.Data == "p" {
			candidates = append(candidates, n)
		}
	}
	for _, n := range candidates {
		text := strings.TrimSpace(contentText(n))
		if fragment, ok := preserved.fragments[text]; ok {
			doc, err := html.Parse(strings.NewReader(fragment))
			if err != nil {
				return err
			}
			body := element(doc, "body")
			for child := body.FirstChild; child != nil; {
				next := child.NextSibling
				body.RemoveChild(child)
				n.Parent.InsertBefore(child, n)
				child = next
			}
			n.Parent.RemoveChild(n)
		} else if h, ok := markers[text]; ok {
			parent := n.Parent
			setAttribute(parent, "data-hp-title", h.title)
			setAttribute(parent, "data-hp-level", fmt.Sprint(h.level))
			if h.visibility != "" {
				setAttribute(parent, "data-hp-visibility", h.visibility)
			}
			if h.custom != "" {
				setAttribute(parent, "data-hp-custom", h.custom)
			}
			if h.id != "" {
				setAttribute(parent, "data-hp-org-id", h.id)
			}
			n.Parent.RemoveChild(n)
		}
	}
	return nil
}

func catalogue(p *page, log *console) {
	used := make(map[string]int)
	var headings []*html.Node
	for n := range p.dom.Descendants() {
		if n.Type == html.ElementNode && len(n.Data) == 2 && n.Data[0] == 'h' && n.Data[1] >= '1' && n.Data[1] <= '6' {
			headings = append(headings, n)
		}
	}
	for _, h := range headings {
		sec := h.Parent
		originalLevel := int(h.Data[1] - '0')
		depth := 1
		for parent := sec.Parent; parent != nil && parent != p.dom; parent = parent.Parent {
			if parent.Data == "section" {
				depth++
			}
		}
		if sec.Data != "section" {
			depth = originalLevel
		}
		if attribute(sec, "data-hp-level") == "" {
			setAttribute(sec, "data-hp-level", fmt.Sprint(originalLevel))
		}
		if depth < 6 {
			h.Data = fmt.Sprintf("h%d", depth+1)
			h.DataAtom = 0
		} else {
			h.Data = "div"
			h.DataAtom = 0
			setAttribute(h, "role", "heading")
			setAttribute(h, "aria-level", fmt.Sprint(depth+1))
		}
		title := attribute(sec, "data-hp-title")
		if title == "" {
			title = strings.Join(strings.Fields(contentText(h)), " ")
		}
		id := attribute(sec, "id")
		if id == "" {
			id = attribute(h, "id")
		}
		custom := attribute(sec, "data-hp-custom")
		alias := attribute(sec, "data-hp-org-id")
		if validID(custom) {
			id = custom
		} else if validID(alias) {
			id = alias
		}
		if !validID(id) {
			id = fmt.Sprintf("heading-%d", len(p.headings)+1)
		}
		base := id
		for used[id] > 0 {
			id = fmt.Sprintf("%s-%d", base, used[base])
			used[base]++
		}
		used[id]++
		setAttribute(sec, "id", id)
		removeAttribute(h, "id")
		p.headings[title] = append(p.headings[title], id)
		if custom != "" {
			p.ids[custom] = append(p.ids[custom], id)
		}
		if alias != "" {
			target := id
			if alias != id && validID(alias) && used[alias] == 0 {
				span := &html.Node{Type: html.ElementNode, Data: "span", Attr: []html.Attribute{{Key: "id", Val: alias}}}
				h.AppendChild(span)
				used[alias]++
				target = alias
			}
			p.orgIDs[alias] = append(p.orgIDs[alias], target)
		}
		vis := attribute(sec, "data-hp-visibility")
		if vis != "" && vis != "folded" && vis != "children" && vis != "all" {
			log.warn("%q: unknown VISIBILITY; showing subtree", p.source.logical)
			setAttribute(sec, "data-hp-visibility", "all")
		}
		removeAttribute(sec, "data-hp-title")
		removeAttribute(sec, "data-hp-custom")
		removeAttribute(sec, "data-hp-org-id")
	}
	// Include actual writer IDs, including footnotes, in the destination catalogue.
	for n := range p.dom.Descendants() {
		id := attribute(n, "id")
		if id != "" {
			if len(p.ids[id]) == 0 {
				p.ids[id] = []string{id}
			}
		}
	}
}
func validID(id string) bool {
	return id != "" && !strings.ContainsAny(id, " \t\r\n\x00") && !strings.HasPrefix(id, "hp-")
}

// Packaged and frozen against the approved Pandoc 3.9.0.2 reader set.
var markdownDialect = func() string {
	data, err := bundle.Assets.ReadFile("assets/pandoc/markdown.txt")
	if err != nil {
		return "markdown"
	}
	return strings.TrimSpace(string(data))
}()

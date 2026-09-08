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
	resourceLinks      map[*html.Node]bool
}

func (s *session) render(ctx context.Context, p *page, data []byte) error {
	dialect, err := bundle.Assets.ReadFile("assets/pandoc/markdown.txt")
	if err != nil {
		return err
	}
	format := strings.TrimSpace(string(dialect))
	preserved := preservation{startup: "showall"}
	if strings.EqualFold(filepath.Ext(p.source.logical), ".org") {
		format = "org"
		preserved = preserveOrg(data)
		data = []byte(preserved.text)
	}
	p.startup = preserved.startup
	for _, warning := range preserved.warnings {
		s.log.notice("%q: %s", p.source.logical, warning)
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

type headingDestination struct {
	heading, section, aliasNode *html.Node
	title, custom, alias        string
}

func catalogue(p *page, log *console) {
	wrapBareHeadings(p.dom)
	flattenOutlineGaps(p.dom)
	var destinations []headingDestination
	for n := range p.dom.Descendants() {
		if isHeading(n) {
			destinations = append(destinations, prepareHeading(n, len(destinations)+1))
		}
	}
	p.ids = allocateIdentifiers(p.dom)
	for _, d := range destinations {
		target := attribute(d.section, "id")
		p.headings[d.title] = append(p.headings[d.title], target)
		if d.custom != "" && (!validID(d.custom) || len(p.ids[d.custom]) != 1) {
			p.ids[d.custom] = nil
			log.notice("%q: invalid or ambiguous source identifier", p.source.logical)
		}
		if d.alias != "" {
			if validID(d.alias) && len(p.ids[d.alias]) == 1 {
				if d.aliasNode != nil {
					target = attribute(d.aliasNode, "id")
				}
				p.orgIDs[d.alias] = append(p.orgIDs[d.alias], target)
			} else {
				p.orgIDs[d.alias] = nil
				log.notice("%q: invalid or ambiguous Org ID", p.source.logical)
			}
		}
		visibility := attribute(d.section, "data-hp-visibility")
		if visibility != "" && visibility != "folded" && visibility != "children" && visibility != "all" {
			log.notice("%q: unknown VISIBILITY; showing subtree", p.source.logical)
			setAttribute(d.section, "data-hp-visibility", "all")
		}
		for _, key := range []string{"data-hp-title", "data-hp-custom", "data-hp-org-id"} {
			removeAttribute(d.section, key)
		}
	}
}

func prepareHeading(h *html.Node, ordinal int) headingDestination {
	d := headingDestination{heading: h, section: h.Parent}
	if attribute(d.section, "data-hp-title") != "" {
		d.title = orgHeadingText(h)
	} else {
		d.title = strings.Join(strings.Fields(contentText(h)), " ")
	}
	d.custom = attribute(d.section, "data-hp-custom")
	d.alias = attribute(d.section, "data-hp-org-id")
	originalLevel := 1
	if len(h.Data) == 2 {
		originalLevel = int(h.Data[1] - '0')
	}
	depth := 1
	for parent := d.section.Parent; parent != nil; parent = parent.Parent {
		if parent.Data == "section" && directHeading(parent) != nil {
			depth++
		}
	}
	if attribute(d.section, "data-hp-level") == "" {
		setAttribute(d.section, "data-hp-level", fmt.Sprint(originalLevel))
	}
	h.DataAtom = 0
	if depth < 6 {
		h.Data = fmt.Sprintf("h%d", depth+1)
	} else {
		h.Data = "div"
		setAttribute(h, "role", "heading")
		setAttribute(h, "aria-level", fmt.Sprint(depth+1))
	}
	id := attribute(d.section, "id")
	if id == "" {
		id = attribute(h, "id")
	}
	if validID(d.custom) {
		id = d.custom
	} else if validID(d.alias) {
		id = d.alias
	}
	if !validID(id) {
		id = fmt.Sprintf("heading-%d", ordinal)
	}
	setAttribute(d.section, "id", id)
	removeAttribute(h, "id")
	if validID(d.alias) && d.alias != id {
		d.aliasNode = &html.Node{Type: html.ElementNode, Data: "span", Attr: []html.Attribute{{Key: "id", Val: d.alias}}}
		h.AppendChild(d.aliasNode)
	}
	return d
}

// Raw HTML headings need their own published section rather than an ID on body.
func wrapBareHeadings(root *html.Node) {
	var bare []*html.Node
	for n := range root.Descendants() {
		if isHeading(n) && (n.Parent.Data != "section" || directHeading(n.Parent) != n) {
			bare = append(bare, n)
		}
	}
	for _, h := range bare {
		parent := h.Parent
		section := &html.Node{Type: html.ElementNode, Data: "section"}
		parent.InsertBefore(section, h)
		for n := h; n != nil; {
			if n != h && (isHeading(n) || n.Data == "section") {
				break
			}
			next := n.NextSibling
			parent.RemoveChild(n)
			section.AppendChild(n)
			n = next
		}
	}
}

func orgHeadingText(root *html.Node) string {
	var out strings.Builder
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		for _, class := range strings.Fields(attribute(n, "class")) {
			if class == "todo" || class == "done" || class == "priority" || class == "tag" {
				return
			}
		}
		if n.Type == html.TextNode {
			out.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(root)
	return strings.Join(strings.Fields(out.String()), " ")
}

func allocateIdentifiers(root *html.Node) map[string][]string {
	reserved := make(map[string]bool)
	for n := range root.Descendants() {
		if id := attribute(n, "id"); id != "" {
			reserved[id] = true
		}
	}
	assigned := make(map[string]bool)
	catalogue := make(map[string][]string)
	for n := range root.Descendants() {
		original := attribute(n, "id")
		if original == "" {
			continue
		}
		id := original
		if !validID(id) {
			id = "source-identifier"
		}
		if assigned[id] || id != original {
			base := id
			for suffix := 1; ; suffix++ {
				id = fmt.Sprintf("%s-%d", base, suffix)
				if !reserved[id] && !assigned[id] {
					break
				}
			}
		}
		setAttribute(n, "id", id)
		assigned[id] = true
		if validID(original) {
			catalogue[original] = append(catalogue[original], id)
		} else {
			catalogue[original] = nil
		}
	}
	return catalogue
}

func flattenOutlineGaps(root *html.Node) {
	var empty []*html.Node
	for n := range root.Descendants() {
		if n.Type == html.ElementNode && n.Data == "section" && strings.Contains(attribute(n, "class"), "level") && directHeading(n) == nil {
			empty = append(empty, n)
		}
	}
	for i := len(empty) - 1; i >= 0; i-- {
		n := empty[i]
		for child := n.FirstChild; child != nil; {
			next := child.NextSibling
			n.RemoveChild(child)
			n.Parent.InsertBefore(child, n)
			child = next
		}
		n.Parent.RemoveChild(n)
	}
}

func directHeading(section *html.Node) *html.Node {
	for n := section.FirstChild; n != nil; n = n.NextSibling {
		if isHeading(n) || attribute(n, "role") == "heading" {
			return n
		}
	}
	return nil
}

func isHeading(n *html.Node) bool {
	return n.Type == html.ElementNode && ((len(n.Data) == 2 && n.Data[0] == 'h' && n.Data[1] >= '1' && n.Data[1] <= '6') || (n.Data == "p" && attribute(n, "class") == "heading" && n.Parent.Data == "section"))
}
func validID(id string) bool {
	return id != "" && !strings.ContainsAny(id, " \t\r\n\x00") && !strings.HasPrefix(id, "hp-")
}

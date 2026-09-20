// ABOUTME: Proves source-bound annotation blocks before exposing browser click targets.
// ABOUTME: Keeps temporary native footnotes out of published HTML and original files.
package preview

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/tigger-developer/HTML-Preview/internal/annotation"
	"golang.org/x/net/html"
)

type annotationBlock struct {
	boundary annotation.BlockBoundary
	position int
}
type blockHit struct {
	node, paragraph *html.Node
	key             string
}

// One conversion resolves every candidate, including proof that a native
// footnote parses there. Repeated prose never requires a search or retry loop.
func (s *session) markAnnotationBlocks(ctx context.Context, p *page, input []byte) error {
	id, err := annotation.NewID()
	if err != nil {
		return err
	}
	prefix := "HPBLOCK" + strings.ReplaceAll(id, "-", "") + "X"
	marked, boundaries, err := annotation.MarkBlocks(input, annotationFormat(p.source), prefix)
	if err != nil {
		return err
	}
	if len(boundaries) == 0 {
		return nil
	}
	probe := &page{source: p.source, name: p.name + "-blocks"}
	for label := range boundaries {
		probe.probeLabels = append(probe.probeLabels, label)
	}
	if err := s.render(ctx, probe, marked); err != nil {
		return err
	}
	labels := make(map[string]string)
	for n := range probe.dom.Descendants() {
		if n.Data != "li" || !insideEndnotes(n) {
			continue
		}
		text := strings.Fields(annotation.CanonicalText(n))
		if len(text) > 0 {
			if _, ok := boundaries[text[0]]; ok {
				labels["#"+attribute(n, "id")] = text[0]
			}
		}
	}
	var refs []*html.Node
	var hits []blockHit
	for n := range probe.dom.Descendants() {
		key, ok := labels[attribute(n, "href")]
		if !ok || !hasClass(n, "footnote-ref") {
			continue
		}
		refs = append(refs, n)
		parent := n.Parent
		if !eligibleBlock(parent) || insideEndnotes(parent) || !blockTail(n) {
			continue
		}
		boundary := boundaries[key]
		target := parent
		if boundary.Following {
			target = previousAnnotationBlock(parent, boundary.Kind)
			if target == nil {
				continue
			}
		}
		hits = append(hits, blockHit{target, parent, key})
	}
	for _, ref := range refs {
		ref.Parent.RemoveChild(ref)
	}
	// A line-end probe inside wrapped verbatim/code is literal text, not a
	// footnote. Remove only our exact unparsed tokens before comparing shape;
	// such tokens never become hits or writable boundaries themselves.
	markerPattern := regexp.MustCompile(`\[(?:fn:|\^)` + regexp.QuoteMeta(prefix) + `[0-9]+Q\]`)
	for n := range probe.dom.Descendants() {
		if n.Type == html.TextNode && strings.Contains(n.Data, prefix) {
			n.Data = markerPattern.ReplaceAllString(n.Data, "")
		}
	}
	for _, h := range hits {
		if boundaries[h.key].AfterBlock && h.paragraph.Parent != nil {
			h.paragraph.Parent.RemoveChild(h.paragraph)
		}
	}
	p.annotationBlocks = make(map[string]annotationBlock)
	revision := annotation.Digest(input)[:16]
	pairs := make(map[*html.Node]*html.Node)
	pairBlockElements(probe.dom, p.dom, pairs)
	positions := make(map[string]string)
	aliases := make(map[string]string)
	var positionMarkers []*html.Node
	for _, h := range hits {
		actual := pairs[h.node]
		if actual == nil || annotation.CanonicalText(actual) == "" || blockShape(actual) != blockShape(h.node) {
			continue
		}
		if actual.Data == "li" && element(actual, "p") != nil {
			continue
		}
		key := fmt.Sprintf("b%s-%d", revision, boundaries[h.key].Offset)
		setAttribute(actual, "data-hp-annotation-block", key)
		if boundaries[h.key].Kind == "quote" {
			// Browser selection takes the nearest paragraph, not its quote container.
			for child := range actual.Descendants() {
				if eligibleBlock(child) {
					setAttribute(child, "data-hp-annotation-block", key)
				}
			}
		}
		// A description label selects its first verified content block, using
		// the same server-owned boundary rather than inserting into the term.
		dd := actual
		if dd.Data != "dd" {
			dd = dd.Parent
		}
		if dd != nil && dd.Data == "dd" {
			term := dd.PrevSibling
			for term != nil && term.Type != html.ElementNode {
				term = term.PrevSibling
			}
			if term != nil && term.Data == "dt" && attribute(term, "data-hp-annotation-block") == "" {
				setAttribute(term, "data-hp-annotation-block", key+"-term")
				aliases[key+"-term"] = key
			}
		}
		marker := &html.Node{Type: html.ElementNode, Data: "span", Attr: []html.Attribute{{Key: "id", Val: prefix + key}}}
		var before *html.Node
		for n := actual.FirstChild; n != nil; n = n.NextSibling {
			if n.Data == "ul" || n.Data == "ol" || hasClass(n, "tag") {
				before = n
				break
			}
		}
		actual.InsertBefore(marker, before)
		positions[key] = prefix + key
		positionMarkers = append(positionMarkers, marker)
		p.annotationBlocks[key] = annotationBlock{boundary: boundaries[h.key]}
	}
	_, spans := annotation.CanonicalDocument(p.dom, positions)
	for _, marker := range positionMarkers {
		marker.Parent.RemoveChild(marker)
	}
	for key, block := range p.annotationBlocks {
		block.position = spans[key].End
		p.annotationBlocks[key] = block
	}
	for alias, key := range aliases {
		p.annotationBlocks[alias] = p.annotationBlocks[key]
	}
	return nil
}
func eligibleBlock(n *html.Node) bool {
	return n.Data == "p" || n.Data == "li" || n.Data == "dd" || isHeading(n)
}
func blockTail(ref *html.Node) bool {
	for n := ref.NextSibling; n != nil; n = n.NextSibling {
		// contentText visits descendants, so a sibling text node must be
		// checked directly to reject boundaries before a wrapped line.
		if n.Type == html.TextNode && strings.TrimSpace(n.Data) != "" {
			return false
		}
		if strings.TrimSpace(contentText(n)) != "" && n.Data != "ul" && n.Data != "ol" && !hasClass(n, "tag") && !hasClass(n, "footnote-ref") {
			return false
		}
	}
	return true
}
func previousAnnotationBlock(n *html.Node, kind string) *html.Node {
	n = n.PrevSibling
	for n != nil && n.Type != html.ElementNode {
		n = n.PrevSibling
	}
	if n == nil {
		return nil
	}
	if kind == "heading" && isHeading(n) || kind == "table" && n.Data == "table" || (kind == "quote" || kind == "org:QUOTE") && n.Data == "blockquote" || kind == "org:VERSE" && hasClass(n, "line-block") {
		return n
	}
	if kind == "code" || kind == "org:SRC" || kind == "org:EXAMPLE" {
		if n.Data == "pre" {
			return n
		}
		return element(n, "pre")
	}
	if strings.HasPrefix(kind, "org:") && n.Data == "div" {
		for _, class := range strings.Fields(attribute(n, "class")) {
			if strings.EqualFold(class, strings.TrimPrefix(kind, "org:")) {
				return n
			}
		}
	}
	return nil
}
func insideEndnotes(n *html.Node) bool {
	for ; n != nil; n = n.Parent {
		if hasClass(n, "footnotes") || attribute(n, "data-hp-org-drawer") != "" {
			return true
		}
	}
	return false
}
func blockChildren(n *html.Node) []*html.Node {
	var result []*html.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && !hasClass(c, "footnotes") && !hasClass(c, "footnote-ref") {
			result = append(result, c)
		}
	}
	return result
}
func pairBlockElements(from, to *html.Node, pairs map[*html.Node]*html.Node) {
	if from.Data != to.Data {
		return
	}
	pairs[from] = to
	a, b := blockChildren(from), blockChildren(to)
	if len(a) != len(b) {
		return
	}
	for i := range a {
		pairBlockElements(a[i], b[i], pairs)
	}
}
func blockShape(n *html.Node) string {
	var out strings.Builder
	var visit func(*html.Node)
	visit = func(v *html.Node) {
		if v.Type != html.ElementNode || hasClass(v, "footnote-ref") {
			return
		}
		out.WriteString("<" + v.Data + ">")
		for c := v.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
		out.WriteString("</" + v.Data + ">")
	}
	visit(n)
	return out.String() + annotation.CanonicalText(n)
}
func hasClass(n *html.Node, class string) bool {
	for _, value := range strings.Fields(attribute(n, "class")) {
		if value == class {
			return true
		}
	}
	return false
}

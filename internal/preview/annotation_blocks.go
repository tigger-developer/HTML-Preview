// ABOUTME: Proves source-bound annotation blocks before exposing browser click targets.
// ABOUTME: Keeps temporary native footnotes out of published HTML and original files.
package preview

import (
	"context"
	"fmt"
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
			target = previousAnnotationBlock(parent)
			if target == nil {
				continue
			}
		}
		hits = append(hits, blockHit{target, parent, key})
	}
	for _, ref := range refs {
		ref.Parent.RemoveChild(ref)
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
func previousAnnotationBlock(n *html.Node) *html.Node {
	n = n.PrevSibling
	for n != nil && n.Type != html.ElementNode {
		n = n.PrevSibling
	}
	if n == nil {
		return nil
	}
	if n.Data == "pre" || isHeading(n) {
		return n
	}
	return element(n, "pre")
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

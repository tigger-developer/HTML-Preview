// ABOUTME: Renders sidecar and legacy notes through the existing Pandoc pipeline.
// ABOUTME: Verifies virtual positions and keeps unplaced notes in the native endnotes list.
package preview

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/tigger-developer/HTML-Preview/internal/annotation"
	"golang.org/x/net/html"
)

func (s *session) renderFootnotes(ctx context.Context, p *page, snap annotation.Snapshot) (map[string]bool, error) {
	id, err := annotation.NewID()
	if err != nil {
		return nil, err
	}
	prefix := "HPPREVIEW" + strings.ReplaceAll(id, "-", "") + "X"
	body, headings := annotation.CanonicalDocument(p.dom, p.explicitIDs)
	input, markers, err := annotation.PreviewFootnotes(ctx, snap, annotationFormat(p.source), body, headings, prefix)
	if err != nil || input == nil {
		return nil, err
	}
	original, revision := p.sourceData, p.annotationSourceRevision
	if err := s.render(ctx, p, input); err != nil {
		return nil, err
	}
	p.sourceData, p.annotationSourceRevision = original, revision
	return finalizeVirtualNotes(p.dom, markers, prefix, body)
}

func finalizeVirtualNotes(root *html.Node, notes []annotation.VirtualNote, prefix, body string) (map[string]bool, error) {
	pattern := regexp.MustCompile(regexp.QuoteMeta(prefix) + `[0-9]+Z`)
	byMarker := make(map[string]annotation.VirtualNote)
	for _, note := range notes {
		byMarker[note.Marker] = note
	}
	textNodes, references := make(map[string]*html.Node), make(map[string]*html.Node)
	ids := make(map[string]*html.Node)
	for n := range root.Descendants() {
		if n.Type == html.ElementNode {
			ids[attribute(n, "id")] = n
		}
		if n.Type != html.TextNode {
			continue
		}
		for _, marker := range pattern.FindAllString(n.Data, -1) {
			if _, known := byMarker[marker]; !known || textNodes[marker] != nil {
				return nil, errors.New("invalid virtual footnote marker")
			}
			textNodes[marker] = n
			ref := n.NextSibling
			if ref == nil || ref.Data != "a" || attribute(ref, "role") != "doc-noteref" {
				return nil, errors.New("virtual footnote reference unavailable")
			}
			references[marker] = ref
		}
	}
	for _, note := range notes {
		n := textNodes[note.Marker]
		if n == nil {
			return nil, errors.New("virtual footnote marker missing")
		}
		if note.Position < 0 {
			parent := n.Parent
			if parent.Data != "p" || strings.TrimSpace(contentText(parent)) != note.Marker+contentText(references[note.Marker]) {
				return nil, errors.New("unplaced footnote reference changed document structure")
			}
			parent.Parent.RemoveChild(parent)
		}
	}
	canonical := annotation.CanonicalText(root)
	positions := make(map[string]int)
	var plain strings.Builder
	at, scalars := 0, 0
	for _, match := range pattern.FindAllStringIndex(canonical, -1) {
		part := canonical[at:match[0]]
		plain.WriteString(part)
		scalars += utf8.RuneCountInString(part)
		positions[canonical[match[0]:match[1]]] = scalars
		at = match[1]
	}
	plain.WriteString(canonical[at:])
	if plain.String() != body {
		return nil, errors.New("virtual footnotes changed authored text")
	}
	located := make(map[string]bool)
	for _, note := range notes {
		n, ref := textNodes[note.Marker], references[note.Marker]
		n.Data = strings.Replace(n.Data, note.Marker, "", 1)
		position, found := positions[note.Marker]
		located[note.AnnotationID] = note.Position >= 0 && found && position == note.Position
		if located[note.AnnotationID] {
			continue
		}
		endnote := ids[strings.TrimPrefix(attribute(ref, "href"), "#")]
		if endnote == nil {
			return nil, errors.New("virtual endnote missing")
		}
		if ref.Parent != nil {
			ref.Parent.RemoveChild(ref)
		}
		var backlinks []*html.Node
		for child := range endnote.Descendants() {
			if child.Type == html.ElementNode && attribute(child, "role") == "doc-backlink" {
				backlinks = append(backlinks, child)
			}
		}
		for _, child := range backlinks {
			child.Parent.RemoveChild(child)
		}
		label := &html.Node{Type: html.ElementNode, Data: "p"}
		label.AppendChild(nodeText("Unplaced annotation. Context: " + note.Context))
		endnote.InsertBefore(label, endnote.FirstChild)
	}
	return located, nil
}

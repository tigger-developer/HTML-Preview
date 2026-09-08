// ABOUTME: Validates and removes owned Pandoc transport records before sanitization.
// ABOUTME: Preserves code text and binds contents links to final heading identities.
package preview

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/net/html"
)

// Record values are string-only objects with exactly the named fields. Token
// parsing also rejects duplicate JSON keys rather than accepting last-wins data.
func transportRecord(text string, keys ...string) (map[string]string, error) {
	decoder := json.NewDecoder(strings.NewReader(text))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return nil, errors.New("invalid transport record object")
	}
	allowed := make(map[string]bool)
	for _, key := range keys {
		allowed[key] = true
	}
	values := make(map[string]string)
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return nil, errors.New("invalid transport field")
		}
		name, ok := key.(string)
		if !ok || !allowed[name] {
			return nil, errors.New("unknown transport field")
		}
		if _, exists := values[name]; exists {
			return nil, errors.New("duplicate transport field")
		}
		item, err := decoder.Token()
		value, isString := item.(string)
		if err != nil || !isString {
			return nil, errors.New("non-string transport value")
		}
		values[name] = value
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim('}') || len(values) != len(keys) {
		return nil, errors.New("incomplete transport record")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, errors.New("trailing transport data")
	}
	return values, nil
}

func (s *session) restoreTransport(doc *html.Node, p *page, token string) (map[string]*html.Node, error) {
	prefix := "htmlpreview-"
	owned := map[string]string{
		prefix + "code-" + token + "-":           "code",
		prefix + "heading-" + token + "-":        "heading",
		prefix + "code-record-" + token + "-":    "code-record",
		prefix + "heading-record-" + token + "-": "heading-record",
	}
	index := make(map[string]*html.Node)
	for n := range doc.Descendants() {
		id := attribute(n, "id")
		for start := range owned {
			if strings.HasPrefix(id, start) {
				if !positiveOrdinal(strings.TrimPrefix(id, start)) || index[id] != nil {
					return nil, errors.New("invalid or duplicate transport identifier")
				}
				index[id] = n
			}
		}
		if id == prefix+"toc-"+token {
			if p.toc != nil || n.Data != "div" {
				return nil, errors.New("invalid contents container")
			}
			p.toc = n
		}
	}
	if p.toc != nil {
		p.toc.Parent.RemoveChild(p.toc)
		// Heading images in the contents need no second resource activation. Their
		// alternative text stays in the label; only document images enter resolution.
		var images []*html.Node
		for n := range p.toc.Descendants() {
			if n.Data == "img" {
				images = append(images, n)
			}
		}
		for _, n := range images {
			n.Parent.InsertBefore(nodeText(attribute(n, "alt")), n)
			n.Parent.RemoveChild(n)
		}
		s.scrub(p.toc, p)
	}

	headings := make(map[string]*html.Node)
	consumed := make(map[string]bool)
	for id, owner := range index {
		kind := ""
		if strings.HasPrefix(id, prefix+"code-"+token+"-") {
			kind = "code"
		}
		if strings.HasPrefix(id, prefix+"heading-"+token+"-") {
			kind = "heading"
		}
		if kind == "" {
			continue
		}
		number := strings.TrimPrefix(id, prefix+kind+"-"+token+"-")
		recordID := prefix + kind + "-record-" + token + "-" + number
		record := index[recordID]
		if record == nil || record.Data != "pre" {
			return nil, errors.New("missing transport record")
		}
		for child := record.FirstChild; child != nil; child = child.NextSibling {
			if child.Type != html.TextNode {
				return nil, errors.New("transport record is not literal text")
			}
		}
		field := "text"
		if kind == "heading" {
			field = "originalID"
		}
		values, err := transportRecord(contentText(record), "id", field)
		if err != nil {
			return nil, fmt.Errorf("Pandoc transport: %w", err)
		}
		if values["id"] != id {
			return nil, errors.New("mismatched transport association")
		}
		if kind == "code" {
			if owner.Data != "div" || record.Parent != owner {
				return nil, errors.New("invalid code association")
			}
			if err := s.restoreCode(owner, values["text"], p); err != nil {
				return nil, err
			}
		} else {
			heading, parent := owner, owner.Parent
			if owner.Data == "section" {
				heading, parent = directHeading(owner), owner
			}
			// Pandoc leaves headers in quotations and lists as bare headings.
			// Catalogue wraps them later; retain the actual node across that step.
			if heading == nil || !isHeading(heading) || record.Parent != parent {
				return nil, errors.New("invalid heading association")
			}
			setAttribute(owner, "id", values[field])
			headings[id] = heading
		}
		record.Parent.RemoveChild(record)
		consumed[recordID] = true
		if kind == "code" {
			unwrap(owner)
		}
	}
	for id := range index {
		if (strings.HasPrefix(id, prefix+"code-record-"+token+"-") || strings.HasPrefix(id, prefix+"heading-record-"+token+"-")) && !consumed[id] {
			return nil, errors.New("orphan transport record")
		}
	}
	return headings, nil
}

func positiveOrdinal(value string) bool {
	if value == "" || value[0] < '1' || value[0] > '9' {
		return false
	}
	for _, ch := range value[1:] {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func (s *session) restoreCode(owner *html.Node, original string, p *page) error {
	var code *html.Node
	for n := range owner.Descendants() {
		if n.Data == "code" && n.Parent.Data == "pre" {
			if code != nil {
				return errors.New("multiple code payloads in transport association")
			}
			code = n
		}
	}
	if code == nil {
		return errors.New("missing code payload")
	}
	actual := contentText(code)
	switch {
	case actual == original:
	case actual+"\n" == original:
		code.AppendChild(nodeText("\n"))
	default:
		for code.FirstChild != nil {
			code.RemoveChild(code.FirstChild)
		}
		code.AppendChild(nodeText(original))
		s.log.notice("%q: code writer changed literal text; retained plain code", p.source.logical)
	}
	return nil
}

func unwrap(n *html.Node) {
	parent := n.Parent
	for n.FirstChild != nil {
		child := n.FirstChild
		n.RemoveChild(child)
		parent.InsertBefore(child, n)
	}
	parent.RemoveChild(n)
}

func restoreContents(p *page, headings map[string]*html.Node) error {
	if p.toc == nil {
		return nil
	}
	entries := 0
	for n := range p.toc.Descendants() {
		removeAttribute(n, "id")
		if n.Data != "a" {
			continue
		}
		href := attribute(n, "href")
		heading := headings[strings.TrimPrefix(href, "#")]
		if !strings.HasPrefix(href, "#") || heading == nil {
			return errors.New("contents link lost its heading association")
		}
		target := attribute(heading.Parent, "id")
		if target == "" {
			return errors.New("contents heading has no final identifier")
		}
		setAttribute(n, "href", "#"+target)
		entries++
	}
	if entries == 0 {
		p.toc = nil
	}
	return nil
}

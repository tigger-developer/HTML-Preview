// ABOUTME: Defines canonical authored text and deterministic passage reattachment.
// ABOUTME: Uses Unicode scalar offsets and only verified explicit heading provenance.
package annotation

import (
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
)

// BlockElements and WhiteSpace are mirrored by the browser mapper and shared fixtures.
const BlockElements = "address article aside blockquote br caption dd details div dl dt figcaption figure h1 h2 h3 h4 h5 h6 hr li main ol p pre section summary table tbody td tfoot th thead tr ul"

func WhiteSpace(r rune) bool {
	return (r >= 9 && r <= 13) || r == 32 || r == 0x85 || r == 0xa0 || r == 0x1680 || (r >= 0x2000 && r <= 0x200a) || r == 0x2028 || r == 0x2029 || r == 0x202f || r == 0x205f || r == 0x3000
}

func Normalize(value string) string {
	var output strings.Builder
	pending := false
	for _, r := range value {
		if WhiteSpace(r) {
			pending = output.Len() > 0
			continue
		}
		if pending {
			output.WriteByte(' ')
			pending = false
		}
		output.WriteRune(r)
	}
	return output.String()
}

func CanonicalText(root *html.Node) string {
	text, _ := CanonicalDocument(root, nil)
	return text
}

// CanonicalDocument captures scalar spans only for the supplied verified IDs.
func CanonicalDocument(root *html.Node, explicit map[string]string) (string, map[string]Span) {
	var out strings.Builder
	spans := make(map[string]Span)
	keys := make(map[string][]string)
	for key, id := range explicit {
		keys[id] = append(keys[id], key)
	}
	count, pending := 0, false
	emit := func(text string) {
		for _, r := range text {
			if WhiteSpace(r) {
				pending = count > 0
				continue
			}
			if pending {
				out.WriteByte(' ')
				count++
				pending = false
			}
			out.WriteRune(r)
			count++
		}
	}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			emit(n.Data)
			return
		}
		if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style" || n.Data == "button" || n.Data == "nav" || n.Data == "textarea" || n.Data == "template") {
			return
		}
		block := strings.Contains(" "+BlockElements+" ", " "+n.Data+" ")
		if block {
			pending = count > 0
		}
		start := count
		if pending {
			start++
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		for _, attr := range n.Attr {
			if attr.Key == "id" {
				for _, key := range keys[attr.Val] {
					spans[key] = Span{min(start, count), count}
				}
			}
		}
		if block {
			pending = count > 0
		}
	}
	walk(root)
	return out.String(), spans
}

type Span struct{ Start, End int }

func Resolve(target Target, text string, headings map[string]Span) (Target, string) {
	if target.Type == "document" {
		return target, "resolved"
	}
	if target.Validate() != nil {
		return target, "missing"
	}
	body := []rune(text)
	quote := []rune(target.Exact)
	if target.BodyRevision == Digest([]byte(text)) && target.Start >= 0 && target.End <= len(body) && string(body[target.Start:target.End]) == target.Exact {
		return target, "resolved"
	}
	var positions []int
	for cursor, scalar := 0, 0; cursor < len(text); {
		rel := strings.Index(text[cursor:], target.Exact)
		if rel < 0 {
			break
		}
		at := cursor + rel
		scalar += utf8.RuneCountInString(text[cursor:at])
		matches := strings.HasSuffix(text[:at], target.Prefix) && strings.HasPrefix(text[at+len(target.Exact):], target.Suffix)
		if area, exists := headings[target.HeadingID]; exists && (scalar < area.Start || scalar+len(quote) > area.End) {
			matches = false
		}
		if matches {
			positions = append(positions, scalar)
			if len(positions) == 2 {
				return target, "ambiguous"
			}
		}
		_, width := utf8.DecodeRuneInString(text[at:])
		cursor = at + width
		scalar++
	}
	if len(positions) == 0 {
		return target, "missing"
	}
	if len(positions) != 1 {
		return target, "ambiguous"
	}
	target.Start, target.End = positions[0], positions[0]+len(quote)
	target.BodyRevision = Digest([]byte(text))
	return target, "resolved"
}

// ABOUTME: Presents list task states as passive, consistently sized checkbox indicators.
// ABOUTME: Keeps source spelling intact and normalizes only the conversion DOM/input.
package preview

import (
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

var orgPartialCheckbox = regexp.MustCompile(`^(\s*(?:[-+*]|[0-9]+[.)])\s+(?:\[@[^\]]+\]\s+)?)\[-\](\s|$)`)

// Pandoc otherwise collapses Org's partial state to unchecked before HTML output.
func preservePartialCheckbox(line string) string {
	return orgPartialCheckbox.ReplaceAllString(line, "${1}[/]${2}")
}

func checkboxIndicator(state string) *html.Node {
	label, glyph := "Unchecked", "\u00a0"
	switch state {
	case "checked":
		label, glyph = "Checked", "✓"
	case "partial":
		label, glyph = "Partially completed", "−"
	}
	n := &html.Node{Type: html.ElementNode, Data: "span", Attr: []html.Attribute{
		{Key: "class", Val: "hp-checkbox hp-checkbox-" + state},
		{Key: "role", Val: "img"}, {Key: "aria-label", Val: label}, {Key: "title", Val: label},
	}}
	n.AppendChild(nodeText(glyph))
	return n
}

// Only the first inline of an actual list item can be a task marker. Literal
// code and standalone bracketed prose are deliberately left unchanged.
func decorateCheckboxes(root *html.Node) {
	for item := range root.Descendants() {
		if item.Type != html.ElementNode || item.Data != "li" {
			continue
		}
		first := firstCheckboxInline(item)
		if first == nil {
			continue
		}
		text := first.Data
		if first.Type == html.ElementNode {
			if first.Data != "span" || !hasClass(first, "cookie") {
				continue
			}
			text = contentText(first)
		}
		if len(text) < 3 || text[0] != '[' || text[2] != ']' || (len(text) > 3 && !strings.ContainsRune(" \t\r\n", rune(text[3]))) {
			continue
		}
		state := ""
		switch text[1] {
		case 'x', 'X':
			state = "checked"
		case '/', '-':
			state = "partial"
		}
		if state == "" {
			continue
		}
		parent := first.Parent
		parent.InsertBefore(checkboxIndicator(state), first)
		parent.InsertBefore(nodeText(" "), first)
		if first.Type == html.TextNode {
			first.Data = strings.TrimLeft(text[3:], " \t\r\n")
		} else {
			parent.RemoveChild(first)
		}
	}
}

func firstCheckboxInline(item *html.Node) *html.Node {
	n := item.FirstChild
	for n != nil && n.Type == html.TextNode && strings.TrimSpace(n.Data) == "" {
		n = n.NextSibling
	}
	if n != nil && n.Type == html.ElementNode && n.Data == "p" {
		return firstCheckboxInline(n)
	}
	return n
}

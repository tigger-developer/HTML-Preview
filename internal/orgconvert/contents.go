// ABOUTME: Builds the private contents tree from the same rendered heading records.
// ABOUTME: Removes nested links from labels while retaining inline presentation.
package orgconvert

import (
	"bytes"
	"fmt"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func node(tag string, attrs ...string) *html.Node {
	n := &html.Node{Type: html.ElementNode, Data: tag}
	for i := 0; i < len(attrs); i += 2 {
		n.Attr = append(n.Attr, html.Attribute{Key: attrs[i], Val: attrs[i+1]})
	}
	return n
}
func (w *writer) contents() (string, error) {
	if !w.request.TOC || len(w.headings) == 0 {
		return "", nil
	}
	root := node("div", "id", "htmlpreview-toc-"+w.request.Token)
	list := node("ul")
	root.AppendChild(list)
	type entry struct {
		level int
		item  *html.Node
		list  *html.Node
	}
	stack := []entry{{level: 0, list: list}}
	for _, h := range w.headings {
		if h.level > w.request.TOCDepth {
			continue
		}
		for len(stack) > 1 && stack[len(stack)-1].level >= h.level {
			stack = stack[:len(stack)-1]
		}
		parent := &stack[len(stack)-1]
		target := parent.list
		if parent.item != nil {
			target = node("ul")
			parent.item.AppendChild(target)
			parent.list = target
			parent.item = nil
		}
		item := node("li")
		link := node("a", "href", "#"+h.id)
		fragment, err := html.ParseFragment(strings.NewReader(h.title), &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body})
		if err != nil {
			return "", err
		}
		for _, n := range fragment {
			link.AppendChild(n)
		}
		var links []*html.Node
		for n := range link.Descendants() {
			if n.Data == "a" {
				links = append(links, n)
			}
		}
		for _, n := range links {
			footnote := false
			for _, a := range n.Attr {
				if a.Key == "role" && a.Val == "doc-noteref" {
					footnote = true
				}
			}
			if !footnote {
				for n.FirstChild != nil {
					c := n.FirstChild
					n.RemoveChild(c)
					n.Parent.InsertBefore(c, n)
				}
			}
			n.Parent.RemoveChild(n)
		}
		item.AppendChild(link)
		target.AppendChild(item)
		stack = append(stack, entry{level: h.level, item: item, list: target})
	}
	var out bytes.Buffer
	if err := html.Render(&out, root); err != nil {
		return "", err
	}
	return out.String(), nil
}

func footnoteBacklink(content string, number int, token string) (string, error) {
	root := node("div")
	fragment, err := html.ParseFragment(strings.NewReader(content), &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body})
	if err != nil {
		return "", err
	}
	for _, n := range fragment {
		root.AppendChild(n)
	}
	last := root.LastChild
	for last != nil && last.Type == html.TextNode && strings.TrimSpace(last.Data) == "" {
		last = last.PrevSibling
	}
	preserved := false
	if last != nil {
		for n := range last.Descendants() {
			if n.Type == html.TextNode && strings.HasPrefix(n.Data, "HTMLPREVIEW_DRAWER_"+token) {
				preserved = true
			}
		}
	}
	if last == nil || last.Data != "p" || preserved {
		last = node("p")
		root.AppendChild(last)
	}
	link := node("a", "href", fmt.Sprintf("#fnref%d", number), "class", "footnote-back", "role", "doc-backlink")
	link.AppendChild(&html.Node{Type: html.TextNode, Data: "↩︎"})
	last.AppendChild(link)
	var out bytes.Buffer
	for n := root.FirstChild; n != nil; n = n.NextSibling {
		if err := html.Render(&out, n); err != nil {
			return "", err
		}
	}
	return out.String(), nil
}

// Undefined authored references are ordinary literal text, as with the Org
// reader previously used here. They confer no annotation target authority.
func literalUnresolvedNotes(content string, unresolved map[string]string) (string, error) {
	root := node("div")
	fragment, err := html.ParseFragment(strings.NewReader(content), &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body})
	if err != nil {
		return "", err
	}
	for _, n := range fragment {
		root.AppendChild(n)
	}
	var replace, empty []*html.Node
	for n := range root.Descendants() {
		if n.Data == "a" {
			for _, a := range n.Attr {
				if a.Key == "id" && unresolved[a.Val] != "" {
					replace = append(replace, n)
				}
			}
		}
		if n.Data == "section" {
			for _, a := range n.Attr {
				if a.Key == "role" && a.Val == "doc-endnotes" {
					found := false
					for child := range n.Descendants() {
						if child.Data == "li" {
							found = true
						}
					}
					if !found {
						empty = append(empty, n)
					}
				}
			}
		}
	}
	for _, n := range replace {
		for _, a := range n.Attr {
			if a.Key == "id" {
				n.Parent.InsertBefore(&html.Node{Type: html.TextNode, Data: unresolved[a.Val]}, n)
			}
		}
		n.Parent.RemoveChild(n)
	}
	for _, n := range empty {
		n.Parent.RemoveChild(n)
	}
	var out bytes.Buffer
	for n := root.FirstChild; n != nil; n = n.NextSibling {
		if err := html.Render(&out, n); err != nil {
			return "", err
		}
	}
	return out.String(), nil
}

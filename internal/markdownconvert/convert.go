// ABOUTME: Converts the native Markdown subset without authored HTML execution.
// ABOUTME: Adapts library output to the shared heading, literal and footnote contract.
package markdownconvert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"strings"
	"unicode"

	"github.com/tigger-developer/HTML-Preview/internal/converthtml"
	"github.com/tigger-developer/HTML-Preview/internal/convertworker"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	dom "golang.org/x/net/html"
)

func Worker(input io.Reader, output, diagnostic io.Writer) int {
	return convertworker.Run(input, output, diagnostic, convert)
}

type codeRenderer struct {
	token   string
	ordinal int
}

func (c *codeRenderer) RegisterFuncs(r renderer.NodeRendererFuncRegisterer) {
	r.Register(ast.KindCodeBlock, c.render)
	r.Register(ast.KindFencedCodeBlock, c.render)
}
func (c *codeRenderer) render(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	language := ""
	if block, ok := n.(*ast.FencedCodeBlock); ok {
		language = string(block.Language(source))
	}
	literal := strings.TrimSuffix(string(n.Lines().Value(source)), "\n")
	c.ordinal++
	id := fmt.Sprintf("htmlpreview-code-%s-%d", c.token, c.ordinal)
	record, err := json.Marshal(map[string]string{"id": id, "text": literal})
	if err != nil {
		return ast.WalkStop, err
	}
	_, err = fmt.Fprintf(w, `<div id="%s"><pre><code class="sourceCode">%s</code></pre><pre id="htmlpreview-code-record-%s-%d">%s</pre></div>`+"\n", id, converthtml.Highlight(literal, language), c.token, c.ordinal, html.EscapeString(string(record)))
	return ast.WalkSkipChildren, err
}

func convert(r convertworker.Request) ([]byte, error) {
	source, metadata, err := extractMetadata([]byte(r.Text), r.OutputBytes)
	if err != nil {
		return nil, err
	}
	// Match the existing Markdown literal contract while the original snapshot
	// remains untouched for plaintext view and source-bound annotation writes.
	source = bytes.ReplaceAll(source, []byte("\r\n"), []byte("\n"))
	source = bytes.ReplaceAll(source, []byte("\r"), []byte("\n"))
	extensions := []goldmark.Extender{extension.Table, extension.Strikethrough, extension.TaskList, extension.DefinitionList, extension.Footnote}
	if !r.SmartOff {
		extensions = append(extensions, extension.Typographer)
	}
	md := goldmark.New(goldmark.WithExtensions(extensions...), goldmark.WithParserOptions(parser.WithAttribute()), goldmark.WithRendererOptions(renderer.WithNodeRenderers(util.Prioritized(&codeRenderer{token: r.Token}, 100))))
	tree := md.Parser().Parse(text.NewReader(source))
	omitted := false
	err = ast.Walk(tree, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering && (n.Kind() == ast.KindRawHTML || n.Kind() == ast.KindHTMLBlock) {
			omitted = true
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return nil, err
	}
	var raw bytes.Buffer
	if err = md.Renderer().Render(&raw, source, tree); err != nil {
		return nil, err
	}
	doc, err := dom.Parse(&raw)
	if err != nil {
		return nil, err
	}
	body := element(doc, "body")
	if err = prependMetadata(body, metadata, tree.HasChildren()); err != nil {
		return nil, err
	}
	normalizeFootnotes(body)
	headings, err := sectionHeadings(body, r.Token)
	if err != nil {
		return nil, err
	}
	if r.TOC {
		body.InsertBefore(contents(headings, r.Token, r.TOCDepth), body.FirstChild)
	}
	if omitted {
		body.AppendChild(node("span", "id", "htmlpreview-omitted-html-"+r.Token))
	}
	var output bytes.Buffer
	if err = dom.Render(&output, doc); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func node(tag string, attrs ...string) *dom.Node {
	n := &dom.Node{Type: dom.ElementNode, Data: tag}
	for i := 0; i < len(attrs); i += 2 {
		n.Attr = append(n.Attr, dom.Attribute{Key: attrs[i], Val: attrs[i+1]})
	}
	return n
}
func textNode(s string) *dom.Node { return &dom.Node{Type: dom.TextNode, Data: s} }
func attr(n *dom.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
func set(n *dom.Node, key, value string) {
	for i := range n.Attr {
		if n.Attr[i].Key == key {
			n.Attr[i].Val = value
			return
		}
	}
	n.Attr = append(n.Attr, dom.Attribute{Key: key, Val: value})
}
func element(n *dom.Node, tag string) *dom.Node {
	for c := range n.Descendants() {
		if c.Type == dom.ElementNode && c.Data == tag {
			return c
		}
	}
	return nil
}
func plain(n *dom.Node) string {
	var b strings.Builder
	for c := range n.Descendants() {
		if c.Type == dom.TextNode {
			b.WriteString(c.Data)
		}
	}
	return b.String()
}
func slug(s string) string {
	var b strings.Builder
	started := false
	for _, r := range strings.ToLower(s) {
		if !started && !unicode.IsLetter(r) {
			continue
		}
		started = true
		if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("_.-", r) {
			b.WriteRune(r)
		} else if unicode.IsSpace(r) {
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 {
		return "section"
	}
	return b.String()
}

type heading struct {
	level     int
	id, label string
}

func sectionHeadings(root *dom.Node, token string) ([]heading, error) {
	var headings []heading
	var visit func(*dom.Node) error
	visit = func(parent *dom.Node) error {
		var children []*dom.Node
		for n := parent.FirstChild; n != nil; n = n.NextSibling {
			children = append(children, n)
		}
		type section struct {
			level int
			n     *dom.Node
		}
		stack := []section{{0, parent}}
		for _, n := range children {
			if n.Type == dom.ElementNode && attr(n, "role") != "doc-endnotes" {
				if err := visit(n); err != nil {
					return err
				}
			}
			level := 0
			if n.Type == dom.ElementNode && len(n.Data) == 2 && n.Data[0] == 'h' && n.Data[1] >= '1' && n.Data[1] <= '6' {
				level = int(n.Data[1] - '0')
			}
			parent.RemoveChild(n)
			if level == 0 {
				stack[len(stack)-1].n.AppendChild(n)
				continue
			}
			for len(stack) > 1 && stack[len(stack)-1].level >= level {
				stack = stack[:len(stack)-1]
			}
			id := fmt.Sprintf("htmlpreview-heading-%s-%d", token, len(headings)+1)
			original := attr(n, "id")
			if original == "" {
				original = slug(plain(n))
			}
			n.Attr = nil
			sectionNode := node("section", "class", fmt.Sprintf("level%d", level), "id", id)
			stack[len(stack)-1].n.AppendChild(sectionNode)
			sectionNode.AppendChild(n)
			record, err := json.Marshal(map[string]string{"id": id, "originalID": original})
			if err != nil {
				return err
			}
			marker := node("pre", "id", fmt.Sprintf("htmlpreview-heading-record-%s-%d", token, len(headings)+1))
			marker.AppendChild(textNode(string(record)))
			sectionNode.AppendChild(marker)
			headings = append(headings, heading{level, id, plain(n)})
			stack = append(stack, section{level, sectionNode})
		}
		return nil
	}
	err := visit(root)
	return headings, err
}
func contents(headings []heading, token string, depth int) *dom.Node {
	root := node("div", "id", "htmlpreview-toc-"+token)
	list := node("ul")
	root.AppendChild(list)
	type entry struct {
		level      int
		list, item *dom.Node
	}
	stack := []entry{{0, list, nil}}
	for _, h := range headings {
		if h.level > depth {
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
		a := node("a", "href", "#"+h.id)
		a.AppendChild(textNode(h.label))
		item.AppendChild(a)
		target.AppendChild(item)
		stack = append(stack, entry{h.level, target, item})
	}
	return root
}
func normalizeFootnotes(root *dom.Node) {
	ids := make(map[string]string)
	for n := range root.Descendants() {
		id := attr(n, "id")
		if strings.HasPrefix(id, "fn:") || strings.HasPrefix(id, "fnref") {
			ids[id] = strings.ReplaceAll(id, ":", "-")
		}
	}
	var refs []*dom.Node
	for n := range root.Descendants() {
		if id := ids[attr(n, "id")]; id != "" {
			set(n, "id", id)
		}
		if dest := ids[strings.TrimPrefix(attr(n, "href"), "#")]; dest != "" {
			set(n, "href", "#"+dest)
		}
		if attr(n, "role") == "doc-endnotes" {
			n.Data = "section"
			set(n, "class", "footnotes footnotes-end-of-document")
		}
		if attr(n, "class") == "footnote-backref" {
			set(n, "class", "footnote-back")
		}
		if attr(n, "role") == "doc-noteref" && n.Parent.Data == "sup" {
			refs = append(refs, n)
		}
	}
	for _, a := range refs {
		sup := a.Parent
		parent := sup.Parent
		id := attr(sup, "id")
		sup.RemoveChild(a)
		parent.InsertBefore(a, sup)
		parent.RemoveChild(sup)
		sup.Attr = nil
		for a.FirstChild != nil {
			c := a.FirstChild
			a.RemoveChild(c)
			sup.AppendChild(c)
		}
		a.AppendChild(sup)
		set(a, "id", id)
	}
}

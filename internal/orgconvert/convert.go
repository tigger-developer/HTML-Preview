// ABOUTME: Adapts native Org writer output to the established preview HTML contract.
// ABOUTME: Preserves private transport, outline structure and ordinary footnote navigation.
package orgconvert

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/niklasfasching/go-org/org"
	dom "golang.org/x/net/html"
)

type heading struct {
	level     int
	id, title string
}
type writer struct {
	*org.HTMLWriter
	request     Request
	headings    []heading
	definitions map[string]org.FootnoteDefinition
	references  []org.FootnoteLink
	literals    map[string]bool
	done        map[string]bool
	unresolved  map[string]string
	failure     error
}

func (w *writer) write(text string) {
	if _, err := w.HTMLWriter.WriteString(text); err != nil {
		w.failure = err
	}
}

func convert(r Request) ([]byte, error) {
	config := org.New()
	config.MaxScanTokenSize = len(r.Text) + 1
	config.Log = log.New(io.Discard, "", 0)
	config.ReadFile = func(string) ([]byte, error) { return nil, errors.New("include disabled") }
	config.DefaultSettings["EXCLUDE_TAGS"] = ""
	config.DefaultSettings["TODO"] = todoKeywords(r.Text)
	text := strings.TrimPrefix(protectProbeReferences(r), "\ufeff")
	doc := config.Parse(strings.NewReader(text), "")
	if doc.Error != nil {
		return nil, errors.New("Org parsing failed")
	}
	// The outer page/preservation owns export settings and title display.
	doc.BufferSettings["OPTIONS"] = "toc:nil title:nil <:t e:t f:t pri:t todo:t tags:t"
	doc.BufferSettings["EXCLUDE_TAGS"] = ""
	w := &writer{HTMLWriter: org.NewHTMLWriter(), request: r, definitions: make(map[string]org.FootnoteDefinition), literals: make(map[string]bool), done: doneKeywords(r.Text)}
	w.ExtendingWriter = w
	body, err := doc.Write(w)
	if err != nil || w.failure != nil {
		return nil, errors.New("Org writer failed")
	}
	if len(w.unresolved) > 0 {
		body, err = literalUnresolvedNotes(body, w.unresolved)
		if err != nil {
			return nil, err
		}
	}
	toc, err := w.contents()
	if err != nil {
		return nil, err
	}
	return []byte("<!DOCTYPE html><html><head><title>Preview conversion</title></head><body>" + toc + body + "</body></html>"), nil
}
func protectProbeReferences(r Request) string {
	labels := make(map[string]bool, len(r.ProbeLabels))
	for _, label := range r.ProbeLabels {
		labels["[fn:"+label+"]"] = true
	}
	lines := strings.SplitAfter(r.Text, "\n")
	for i, line := range lines {
		if labels[strings.TrimSpace(line)] {
			lines[i] = "@@html:<!--htmlpreview-probe-" + r.Token + "-->@@" + line
		}
	}
	return strings.Join(lines, "")
}
func todoKeywords(text string) string {
	keywords := []string{"TODO", "DONE"}
	for _, line := range strings.Split(text, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		switch strings.ToUpper(key) {
		case "#+TODO", "#+SEQ_TODO", "#+TYP_TODO":
			for _, word := range strings.Fields(value) {
				word, _, _ = strings.Cut(word, "(")
				if word != "|" {
					keywords = append(keywords, word)
				}
			}
		}
	}
	return strings.Join(keywords, " ")
}
func (w *writer) Before(d *org.Document)   { w.HTMLWriter.Before(d) }
func (w *writer) WriteInclude(org.Include) {}
func (w *writer) WriteKeyword(k org.Keyword) {
	if k.Key == "HTML" {
		w.write(k.Value + "\n")
	}
}
func (w *writer) WriteHeadline(h org.Headline) {
	if h.IsComment {
		return
	}
	id := fmt.Sprintf("htmlpreview-heading-%s-%d", w.request.Token, len(w.headings)+1)
	title := w.WriteNodesAsString(h.Title...)
	original := headingID(title)
	w.headings = append(w.headings, heading{h.Lvl, id, title})
	w.write(fmt.Sprintf(`<section class="level%d" id="%s">`, h.Lvl, id))
	tag := "h" + strconv.Itoa(h.Lvl)
	if h.Lvl > 6 {
		tag = `p class="heading"`
	}
	w.write("<" + tag + ">")
	if h.Status != "" {
		class := "todo"
		if w.done[h.Status] {
			class = "done"
		}
		w.write(`<span class="` + class + `">` + html.EscapeString(h.Status) + `</span> `)
	}
	if h.Priority != "" {
		w.write(`<span class="priority">[#` + html.EscapeString(h.Priority) + `]</span> `)
	}
	w.write(title)
	for _, tag := range h.Tags {
		w.write(` <span class="tag">` + html.EscapeString(tag) + `</span>`)
	}
	w.write("</" + strings.Fields(tag)[0] + ">\n")
	w.record("heading", id, map[string]string{"id": id, "originalID": original})
	org.WriteNodes(w, h.Children...)
	w.write("</section>\n")
}
func (w *writer) record(kind, id string, value map[string]string) {
	data, err := json.Marshal(value)
	if err != nil {
		w.failure = err
		return
	}
	recordID := strings.Replace(id, "htmlpreview-"+kind+"-", "htmlpreview-"+kind+"-record-", 1)
	w.write(`<pre id="` + recordID + `">` + html.EscapeString(string(data)) + `</pre>`)
}
func headingID(title string) string {
	doc, err := dom.Parse(strings.NewReader(title))
	if err != nil {
		return "section"
	}
	var plain strings.Builder
	for n := range doc.Descendants() {
		if n.Type == dom.TextNode {
			plain.WriteString(n.Data)
		}
	}
	var id strings.Builder
	started := false
	for _, r := range strings.ToLower(plain.String()) {
		if !started && !unicode.IsLetter(r) {
			continue
		}
		started = true
		if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("_.-", r) {
			id.WriteRune(r)
		} else if unicode.IsSpace(r) {
			id.WriteByte('-')
		}
	}
	if id.Len() == 0 {
		return "section"
	}
	return id.String()
}
func (w *writer) WriteFootnoteDefinition(f org.FootnoteDefinition) { w.definitions[f.Name] = f }
func (w *writer) WriteFootnoteLink(f org.FootnoteLink) {
	w.references = append(w.references, f)
	n := len(w.references)
	w.write(fmt.Sprintf(`<a href="#fn%d" id="fnref%d" class="footnote-ref" role="doc-noteref"><sup>%d</sup></a>`, n, n, n))
}
func (w *writer) After(_ *org.Document) {
	if len(w.references) == 0 {
		return
	}
	w.write(`<section class="footnotes footnotes-end-of-document" role="doc-endnotes"><hr><ol>`)
	for i := 0; i < len(w.references); i++ {
		f := w.references[i]
		definition, ok := w.definitions[f.Name]
		if f.Definition != nil {
			definition, ok = *f.Definition, true
		}
		if !ok {
			if w.unresolved == nil {
				w.unresolved = make(map[string]string)
			}
			w.unresolved[fmt.Sprintf("fnref%d", i+1)] = "[fn:" + f.Name + "]"
			continue
		}
		if i > len(w.request.Text) {
			w.failure = errors.New("recursive footnote")
			return
		}
		w.write(fmt.Sprintf(`<li id="fn%d">`, i+1))
		content := w.WriteNodesAsString(definition.Children...)
		var err error
		content, err = footnoteBacklink(content, i+1, w.request.Token)
		if err != nil {
			w.failure = err
			return
		}
		w.write(content + "</li>")
	}
	w.write("</ol></section>")
}

var smartDashes = strings.NewReplacer("---", "—", "--", "–")

func (w *writer) WriteText(t org.Text) {
	if !t.IsRaw {
		t.Content = smartDashes.Replace(t.Content)
	}
	w.HTMLWriter.WriteText(t)
}
func (w *writer) WriteStatisticToken(s org.StatisticToken) {
	w.write(`<span class="cookie">[` + html.EscapeString(s.Content) + `]</span>`)
}
func (w *writer) WriteEmphasis(e org.Emphasis) {
	if e.Kind == "_" {
		w.write(`<span class="underline">`)
		org.WriteNodes(w, e.Content...)
		w.write("</span>")
		return
	}
	w.HTMLWriter.WriteEmphasis(e)
}

// Relative document names stay intact for the application's authorized resolver.
func (w *writer) WriteRegularLink(l org.RegularLink) {
	url := strings.TrimPrefix(l.URL, "file:")
	escaped := html.EscapeString(url)
	if l.Kind() == "image" && l.Description == nil {
		w.write(`<img src="` + escaped + `" alt="` + escaped + `">`)
		return
	}
	text := escaped
	if l.Description != nil {
		text = w.WriteNodesAsString(l.Description...)
	}
	w.write(`<a href="` + escaped + `">` + text + `</a>`)
}
func (w *writer) WriteListItem(li org.ListItem) {
	if li.Status != "" {
		marker := org.Node(org.Text{Content: "[" + li.Status + "] ", IsRaw: true})
		if li.Status == " " {
			marker = org.InlineBlock{Name: "export", Parameters: []string{"html"}, Children: []org.Node{org.Text{Content: `<input type="checkbox" disabled>`, IsRaw: true}}}
		}
		li.Status = ""
		if len(li.Children) > 0 {
			if p, ok := li.Children[0].(org.Paragraph); ok {
				p.Children = append([]org.Node{marker}, p.Children...)
				li.Children = append([]org.Node(nil), li.Children...)
				li.Children[0] = p
			}
		}
	}
	w.HTMLWriter.WriteListItem(li)
}
func (w *writer) WriteBlock(b org.Block) {
	if b.Name == "EXPORT" && len(b.Parameters) > 0 && b.Parameters[0] == "htmlpreview-code-"+w.request.Token {
		var value strings.Builder
		for _, n := range b.Children {
			if t, ok := n.(org.Text); ok {
				value.WriteString(t.Content)
			}
		}
		w.literalRecord(value.String())
		return
	}
	switch b.Name {
	case "COMMENT":
		return
	case "VERSE":
		w.write(`<div class="line-block">`)
		org.WriteNodes(w, b.Children...)
		w.write("</div>\n")
	case "CENTER":
		w.write(`<div class="center">`)
		org.WriteNodes(w, b.Children...)
		w.write("</div>\n")
	case "QUOTE", "EXPORT", "SRC", "EXAMPLE":
		w.HTMLWriter.WriteBlock(b)
	default:
		w.write(`<div class="` + html.EscapeString(strings.ToLower(b.Name)) + `">`)
		org.WriteNodes(w, b.Children...)
		w.write("</div>\n")
	}
}

var ordinalPattern = regexp.MustCompile(`^[1-9][0-9]*$`)

func (w *writer) literalRecord(text string) {
	if uniqueObject([]byte(text)) != nil {
		w.failure = errors.New("invalid literal record")
		return
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(text), &fields); err != nil || len(fields) != 3 {
		w.failure = errors.New("invalid literal fields")
		return
	}
	for _, key := range []string{"id", "text", "language"} {
		value := strings.TrimSpace(string(fields[key]))
		if len(value) < 2 || value[0] != '"' {
			w.failure = errors.New("invalid literal value")
			return
		}
	}
	var record struct {
		ID       string `json:"id"`
		Text     string `json:"text"`
		Language string `json:"language"`
	}
	d := json.NewDecoder(strings.NewReader(text))
	d.DisallowUnknownFields()
	prefix := "htmlpreview-code-" + w.request.Token + "-"
	if err := d.Decode(&record); err != nil || !strings.HasPrefix(record.ID, prefix) || !ordinalPattern.MatchString(strings.TrimPrefix(record.ID, prefix)) || w.literals[record.ID] {
		w.failure = errors.New("invalid literal association")
		return
	}
	w.literals[record.ID] = true
	w.write(`<div id="` + record.ID + `"><pre><code class="sourceCode">`)
	w.write(highlight(record.Text, record.Language))
	w.write("</code></pre>")
	w.record("code", record.ID, map[string]string{"id": record.ID, "text": record.Text})
	w.write("</div>\n")
}

func doneKeywords(text string) map[string]bool {
	result := map[string]bool{"DONE": true}
	declared := false
	for _, line := range strings.Split(text, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		switch strings.ToUpper(key) {
		case "#+TODO", "#+SEQ_TODO", "#+TYP_TODO":
			if !declared {
				result = make(map[string]bool)
				declared = true
			}
			words := strings.Fields(value)
			after := false
			hasPartition := false
			for _, word := range words {
				if word == "|" {
					hasPartition = true
				}
			}
			for i, word := range words {
				if word == "|" {
					after = true
					continue
				}
				word, _, _ = strings.Cut(word, "(")
				if after || !hasPartition && i == len(words)-1 {
					result[word] = true
				}
			}
		}
	}
	return result
}

// go-org retokenizes the description "I." as an empty ordered list. Preserve
// that literal item label so the normal and footnote-probed blocks agree.
func (w *writer) WriteDescriptiveListItem(item org.DescriptiveListItem) {
	details := nonemptyParagraphs(item.Details)
	if len(details) == 1 {
		if list, ok := details[0].(org.List); ok && list.Kind == "ordered" && len(list.Items) == 1 {
			if li, ok := list.Items[0].(org.ListItem); ok && len(nonemptyParagraphs(li.Children)) == 0 {
				item.Details = []org.Node{org.Paragraph{Children: []org.Node{org.Text{Content: li.Bullet}}}}
			}
		}
	}
	w.HTMLWriter.WriteDescriptiveListItem(item)
}
func nonemptyParagraphs(nodes []org.Node) []org.Node {
	var result []org.Node
	for _, n := range nodes {
		if p, ok := n.(org.Paragraph); ok && len(p.Children) == 0 {
			continue
		}
		result = append(result, n)
	}
	return result
}

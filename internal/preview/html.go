// ABOUTME: Applies a passive HTML allowlist before adding owned presentation.
// ABOUTME: Embedded font and script payloads receive exact CSP hashes.
package preview

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	bundle "github.com/tigger-developer/HTML-Preview"
	"golang.org/x/net/html"
)

// #nosec G101 -- These are HTML element names in the passive-content allowlist.
const passiveElements = "a abbr b blockquote br caption code col colgroup dd del details div dl dt em figcaption figure h1 h2 h3 h4 h5 h6 hr i img ins kbd li mark ol p pre q s samp section small span strong sub summary sup table tbody td tfoot th thead time tr u ul var wbr"

// #nosec G101 -- These are HTML attribute names, not credentials.
const passiveAttrs = "id class title lang dir alt width height colspan rowspan scope start value type href src"

func (s *session) scrub(root *html.Node, p *page) {
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		for c := n.FirstChild; c != nil; {
			next := c.NextSibling
			visit(c)
			c = next
		}
		if n.Type != html.ElementNode {
			return
		}
		switch n.Data {
		case "link":
			if p.resourceLinks == nil {
				p.resourceLinks = make(map[*html.Node]bool)
			}
			p.resourceLinks[n] = true
			n.Data = "a"
			n.DataAtom = 0
			n.AppendChild(nodeText("[Unsupported stylesheet/resource link]"))
			if n.Parent != nil && n.Parent.Data == "head" {
				n.Parent.RemoveChild(n)
				body := element(root, "body")
				body.InsertBefore(n, body.FirstChild)
			}
			s.log.notice("%q: stylesheet/resource retained as a link without automatic loading", p.source.logical)
		case "script", "style", "iframe", "frame", "frameset", "object", "embed", "form", "svg", "math", "base", "audio", "video", "canvas":
			if n.Parent != nil {
				placeholder := nodeText("[Unsupported " + n.Data + " content]")
				if n.Parent.Data == "head" {
					body := element(root, "body")
					body.InsertBefore(placeholder, body.FirstChild)
				} else {
					n.Parent.InsertBefore(placeholder, n)
				}
				n.Parent.RemoveChild(n)
			}
			s.log.notice("%q: unsupported %s content removed", p.source.logical, n.Data)
			return
		case "input":
			replacement := nodeText("[Unsupported input]")
			if attribute(n, "type") == "checkbox" {
				state := "unchecked"
				for _, a := range n.Attr {
					if a.Key == "checked" {
						state = "checked"
					}
				}
				replacement = checkboxIndicator(state)
			}
			if n.Parent != nil {
				n.Parent.InsertBefore(replacement, n)
				n.Parent.InsertBefore(nodeText(" "), n)
				n.Parent.RemoveChild(n)
			}
			return
		case "label":
			// Inputs are now passive indicators. Remove their obsolete wrapper
			// before verifying annotation blocks, not only in final sanitization.
			if n.Parent != nil {
				unwrap(n)
			}
			return
		}
		attrs := n.Attr[:0]
		for _, a := range n.Attr {
			if a.Namespace == "" && a.Key == "role" && nativeFootnoteRole(n.Data, a.Val) {
				attrs = append(attrs, a)
				continue
			}
			if !strings.Contains(" "+passiveAttrs+" ", " "+a.Key+" ") || a.Namespace != "" {
				s.log.notice("%q: unsupported attribute %s removed", p.source.logical, a.Key)
				continue
			}
			if len(a.Val) > 65536 && !(n.Data == "img" && a.Key == "src" && int64(len(a.Val)) <= 4*(maxAssetBytes+2)/3+128) {
				s.log.notice("%q: oversized attribute removed", p.source.logical)
				continue
			}
			if a.Key == "id" && strings.HasPrefix(a.Val, "hp-") {
				a.Val = "source-" + a.Val
			}
			if a.Key == "class" {
				var classes []string
				for _, class := range strings.Fields(a.Val) {
					if !strings.HasPrefix(class, "hp-") {
						classes = append(classes, class)
					}
				}
				a.Val = strings.Join(classes, " ")
			}
			attrs = append(attrs, a)
		}
		n.Attr = attrs
		if n.Data == "details" {
			setAttribute(n, "open", "")
		}
	}
	visit(root)
}

func sanitizeDocument(body *html.Node) (string, error) {
	policy := bluemonday.NewPolicy()
	policy.AllowElements(strings.Fields(passiveElements)...)
	policy.AllowAttrs(strings.Fields(passiveAttrs)...).Globally()
	policy.AllowAttrs("open").OnElements("details")
	policy.AllowAttrs("data-hp-footnote-label").OnElements("li")
	policy.AllowAttrs("data-hp-annotation-block").OnElements("p", "li", "dt", "dd", "pre", "h1", "h2", "h3", "h4", "h5", "h6", "div", "table", "blockquote")
	policy.AllowAttrs("data-hp-org-drawer").OnElements("details")
	policy.AllowAttrs("data-hp-level", "data-hp-visibility").OnElements("section")
	policy.AllowAttrs("role", "aria-level").OnElements("div")
	policy.AllowAttrs("role").Matching(regexp.MustCompile(`^img$`)).OnElements("span")
	policy.AllowAttrs("aria-label").OnElements("span")
	policy.AllowAttrs("role").Matching(regexp.MustCompile(`^doc-(noteref|backlink)$`)).OnElements("a")
	policy.AllowAttrs("role").Matching(regexp.MustCompile(`^doc-endnotes$`)).OnElements("section")
	policy.AllowURLSchemes("file", "http", "https", "mailto", "data")
	policy.AllowRelativeURLs(true)
	var rendered bytes.Buffer
	for n := body.FirstChild; n != nil; n = n.NextSibling {
		if err := html.Render(&rendered, n); err != nil {
			return "", err
		}
	}
	return policy.Sanitize(rendered.String()), nil
}

func nativeFootnoteRole(tag, role string) bool {
	return tag == "a" && (role == "doc-noteref" || role == "doc-backlink") || tag == "section" && role == "doc-endnotes"
}

func (s *session) document(p *page) ([]byte, error) {
	if p.source.input.kind == "html" {
		var output bytes.Buffer
		if err := html.Render(&output, p.dom); err != nil {
			return nil, err
		}
		return output.Bytes(), nil
	}
	body, err := sanitizeDocument(p.dom)
	if err != nil {
		return nil, err
	}
	if p.copyOriginal != nil {
		body, err = attachCopyPayload(body, p.copyOriginal)
		if err != nil {
			return nil, err
		}
	}
	toc := ""
	if p.toc != nil {
		toc, err = sanitizeDocument(p.toc)
		if err != nil {
			return nil, err
		}
	}
	css, err := presentationCSS()
	if err != nil {
		return nil, err
	}
	script, err := bundle.Assets.ReadFile("assets/web/preview.js")
	if err != nil {
		return nil, err
	}
	for _, name := range []string{"annotation-composer.js", "annotation-text.js", "annotations.js"} {
		module, err := bundle.Assets.ReadFile("assets/web/" + name)
		if err != nil {
			return nil, err
		}
		script = append(append(script, '\n'), module...)
	}
	layout, err := bundle.Assets.ReadFile("assets/web/page.html")
	if err != nil {
		return nil, err
	}
	fontNotices, err := embeddedFontNotices()
	if err != nil {
		return nil, err
	}
	policy := fmt.Sprintf("default-src 'none'; base-uri 'none'; form-action 'none'; object-src 'none'; script-src 'sha256-%s'; style-src 'sha256-%s'; font-src data:; img-src file: data:", hashBase64(script), hashBase64([]byte(css)))
	if s.cfg.httpOrigin != "" {
		policy = strings.Replace(policy, "img-src file: data:", "img-src "+s.cfg.httpOrigin+" data:", 1)
		if p.annotationData != "" {
			policy += "; connect-src 'self'"
		}
	}
	data := struct {
		OmittedHTML                                                             bool
		HTMLWarning                                                             string
		Folding                                                                 foldingOverride
		SourceData                                                              *string
		AnnotationData                                                          string
		Policy, Name, Source, Directory, Startup, Title, Subtitle, Author, Date string
		Format                                                                  string
		Frontmatter                                                             []metadataField
		CSS                                                                     template.CSS
		Script                                                                  template.JS
		Body, TOC, FontNotices                                                  template.HTML
	}{
		OmittedHTML: p.omittedHTML, HTMLWarning: markdownHTMLWarning,
		Folding:    s.cfg.folding,
		SourceData: p.sourceData,
		Policy:     policy, Name: filepath.Base(p.source.logical), Source: p.source.logical, Directory: displayDirectory(p.source.logical), Startup: p.startup, Title: p.title, Subtitle: p.subtitle, Author: p.author, Date: p.date, Frontmatter: p.frontmatter,
		Format:         p.format,
		AnnotationData: p.annotationData,
		// #nosec G203 -- CSS, script, and notices come only from embed.FS; body has passed the passive allowlist.
		CSS: template.CSS(css), Script: template.JS(script), Body: template.HTML(body), TOC: template.HTML(toc), FontNotices: template.HTML(fontNotices), // Only embedded assets and allowlisted HTML cross these trusted boundaries.
	}
	t, err := template.New("page").Parse(string(layout))
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	if err := t.Execute(&output, data); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

// Only the owned wrapper may attach a clipboard payload, after source HTML has
// passed sanitization. Source-authored data attributes cannot enter this channel.
func attachCopyPayload(body string, original []byte) (string, error) {
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return "", err
	}
	var code *html.Node
	for n := range doc.Descendants() {
		if n.Data == "code" && n.Parent.Data == "pre" {
			if code != nil {
				return "", errors.New("wrapper has multiple displayed code blocks")
			}
			code = n
		}
	}
	if code == nil {
		return "", errors.New("wrapper has no displayed code block")
	}
	setAttribute(code, "data-hp-copy-base64", base64.StdEncoding.EncodeToString(original))
	var out strings.Builder
	for n := element(doc, "body").FirstChild; n != nil; n = n.NextSibling {
		if err := html.Render(&out, n); err != nil {
			return "", err
		}
	}
	return out.String(), nil
}

func hashBase64(data []byte) string {
	sum := sha256.Sum256(data)
	return base64.StdEncoding.EncodeToString(sum[:])
}

// Embedded fonts retain their own notices. The application's Apache licence
// belongs to the distribution and is not inserted into the preview document.
func embeddedFontNotices() (string, error) {
	var out strings.Builder
	for _, path := range []string{"assets/fonts/asap/OFL.txt", "assets/fonts/iosevka-custom/OFL.md"} {
		data, err := bundle.Assets.ReadFile(path)
		if err != nil {
			return "", err
		}
		out.WriteString("<!--\n" + string(data) + "\n-->\n")
	}
	return out.String(), nil
}
func presentationCSS() (string, error) {
	var out strings.Builder
	faces := []struct{ file, family, style, weight, stretch string }{
		{"asap/Asap-VariableFont_wdth,wght.woff2", "Asap", "normal", "100 900", "75% 125%"},
		{"asap/Asap-Italic-VariableFont_wdth,wght.woff2", "Asap", "italic", "100 900", "75% 125%"},
		{"iosevka-custom/IosevkaCustom-Regular.woff2", "Iosevka Custom", "normal", "400", "normal"},
		{"iosevka-custom/IosevkaCustom-Italic.woff2", "Iosevka Custom", "italic", "400", "normal"},
		{"iosevka-custom/IosevkaCustom-Bold.woff2", "Iosevka Custom", "normal", "700", "normal"},
		{"iosevka-custom/IosevkaCustom-BoldItalic.woff2", "Iosevka Custom", "italic", "700", "normal"},
	}
	for _, face := range faces {
		data, err := bundle.Assets.ReadFile("assets/fonts/" + face.file)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&out, "@font-face{font-family:%q;font-style:%s;font-weight:%s;font-stretch:%s;font-display:swap;src:url(data:font/woff2;base64,%s) format('woff2')}\n", face.family, face.style, face.weight, face.stretch, base64.StdEncoding.EncodeToString(data))
	}
	style, err := bundle.Assets.ReadFile("assets/web/preview.css")
	if err != nil {
		return "", err
	}
	out.Write(style)
	return out.String(), nil
}

func displayDirectory(path string) string {
	directory := strings.TrimSuffix(path, filepath.Base(path))
	home, err := os.UserHomeDir()
	if err == nil && home != "" && strings.HasPrefix(directory, home+string(filepath.Separator)) {
		return "~" + strings.TrimPrefix(directory, home)
	}
	return directory
}

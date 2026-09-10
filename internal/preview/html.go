// ABOUTME: Applies a passive HTML allowlist before adding owned presentation.
// ABOUTME: Embedded font and script payloads receive exact CSP hashes.
package preview

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"html/template"
	"path/filepath"
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
			label := "[Unsupported input]"
			if attribute(n, "type") == "checkbox" {
				label = "[ ]"
				for _, a := range n.Attr {
					if a.Key == "checked" {
						label = "[x]"
					}
				}
			}
			if n.Parent != nil {
				n.Parent.InsertBefore(nodeText(label), n)
				n.Parent.RemoveChild(n)
			}
			return
		}
		attrs := n.Attr[:0]
		for _, a := range n.Attr {
			if !strings.Contains(" "+passiveAttrs+" ", " "+a.Key+" ") || a.Namespace != "" {
				s.log.notice("%q: unsupported attribute %s removed", p.source.logical, a.Key)
				continue
			}
			if len(a.Val) > 65536 {
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
	policy.AllowAttrs("data-hp-org-drawer").OnElements("details")
	policy.AllowAttrs("data-hp-level", "data-hp-visibility").OnElements("section")
	policy.AllowAttrs("role", "aria-level").OnElements("div")
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

func (s *session) document(p *page) ([]byte, error) {
	body, err := sanitizeDocument(p.dom)
	if err != nil {
		return nil, err
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
	layout, err := bundle.Assets.ReadFile("assets/web/page.html")
	if err != nil {
		return nil, err
	}
	policy := fmt.Sprintf("default-src 'none'; base-uri 'none'; form-action 'none'; object-src 'none'; script-src 'sha256-%s'; style-src 'sha256-%s'; font-src data:; img-src file: data:", hashBase64(script), hashBase64([]byte(css)))
	data := struct {
		Policy, Name, Source, Directory, Startup, Title, Subtitle, Author, Date string
		HasFrontmatter                                                          bool
		CSS                                                                     template.CSS
		Script                                                                  template.JS
		Body, TOC                                                               template.HTML
	}{
		Policy: policy, Name: filepath.Base(p.source.logical), Source: p.source.logical, Directory: strings.TrimSuffix(p.source.logical, filepath.Base(p.source.logical)), Startup: p.startup, Title: p.title, Subtitle: p.subtitle, Author: p.author, Date: p.date, HasFrontmatter: p.title != "" || p.subtitle != "" || p.author != "" || p.date != "",
		// #nosec G203 -- CSS, script, and notices come only from embed.FS; body has passed the passive allowlist.
		CSS: template.CSS(css), Script: template.JS(script), Body: template.HTML(body), TOC: template.HTML(toc), // Only embedded assets and allowlisted HTML cross these trusted boundaries.
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

func hashBase64(data []byte) string {
	sum := sha256.Sum256(data)
	return base64.StdEncoding.EncodeToString(sum[:])
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

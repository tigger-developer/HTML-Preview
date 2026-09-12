// ABOUTME: Exercises authored HTML fidelity and passive resources at the public preview boundary.
// ABOUTME: Uses synthetic local images, style sheets and request canaries to detect active content.
package preview

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRT008_6_PassiveNativeHTML(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { requests.Add(1); w.WriteHeader(204) }))
	defer server.Close()
	root := t.TempDir()
	png := rasterFixture(t)
	source(t, root, "styles/pixel.png", string(png))
	source(t, root, "styles/local.css", `.layout { display:grid; color:rgb(12,34,56); background-image:url(pixel.png) } @import "`+server.URL+`/remote.css";`)
	source(t, root, "child.org", "* Child\n")
	body := `<!doctype html><html><head><title>Authored title</title><link rel="stylesheet" href="styles/local.css"><style>.inline{padding:2rem;background:url(` + server.URL + `/image.png)}</style><meta http-equiv="refresh" content="0;url=` + server.URL + `/refresh"></head><body><article class="layout"><h1>Authored heading</h1><p class="inline" style="font-weight:bold;color:navy" onclick="bad()">Styled text</p><img alt="local" src="styles/pixel.png"><a href="child.org">Child</a><a href="` + server.URL + `/external" ping="` + server.URL + `/ping" target="_blank">External</a><script src="` + server.URL + `/script.js">bad()</script><form action="` + server.URL + `/submit"><input value="Inert value"><button>Submit</button></form><iframe src="` + server.URL + `/frame"></iframe><svg><script>bad()</script></svg></article></body></html>`
	r := run(t, root, []string{"PREVIEW_TEST_FAULT=native-no-conversion"}, source(t, root, "page.html", body))
	success(t, r, 1)
	doc := r.pages[0]
	if textOf(nodes(doc, "title")[0]) != "Authored title" || textOf(nodes(doc, "h1")[0]) != "Authored heading" || len(nodes(doc, "article")) != 1 {
		t.Fatal("authored document structure replaced")
	}
	for n := range doc.Descendants() {
		if attr(n, "id") == "hp-header" || attr(n, "id") == "hp-document" {
			t.Fatal("native HTML received application chrome")
		}
	}
	for _, tag := range []string{"script", "iframe", "svg", "form", "link", "base"} {
		if len(nodes(doc, tag)) != 0 {
			t.Errorf("active %s survived", tag)
		}
	}
	for n := range doc.Descendants() {
		for _, a := range n.Attr {
			if strings.HasPrefix(a.Key, "on") || a.Key == "ping" || a.Key == "action" {
				t.Errorf("active attribute survived: %s", a.Key)
			}
		}
	}
	styles := ""
	for _, style := range nodes(doc, "style") {
		styles += textOf(style)
	}
	if !strings.Contains(styles, "display:grid") || !strings.Contains(styles, "padding:2rem") || strings.Contains(styles, server.URL) || strings.Contains(styles, "@import") {
		t.Fatalf("passive styles differ: %.500s", styles)
	}
	if !strings.Contains(attr(nodes(doc, "p")[0], "style"), "font-weight:bold") {
		t.Fatal("inline passive CSS removed")
	}
	images := nodes(doc, "img")
	if len(images) != 1 || attr(images[0], "src") != "data:image/png;base64,"+base64.StdEncoding.EncodeToString(png) {
		t.Fatal("local raster lost original stylesheet/page context")
	}
	policy := ""
	for _, meta := range nodes(doc, "meta") {
		if strings.EqualFold(attr(meta, "http-equiv"), "Content-Security-Policy") {
			policy = attr(meta, "content")
		}
		if strings.EqualFold(attr(meta, "http-equiv"), "refresh") {
			t.Fatal("refresh remained")
		}
	}
	for _, directive := range []string{"script-src 'none'", "style-src 'unsafe-inline'", "font-src 'none'", "base-uri 'none'", "form-action 'none'"} {
		if !strings.Contains(policy, directive) {
			t.Errorf("missing CSP %s", directive)
		}
	}
	if requests.Load() != 0 {
		t.Fatal("preview conversion fetched source-controlled network resources")
	}
}

func TestRT008_6_RasterSignatures(t *testing.T) {
	avif := make([]byte, 20)
	binary.BigEndian.PutUint32(avif, 20)
	copy(avif[4:], "ftypavif")
	copy(avif[16:], "avif")
	for _, c := range []struct {
		suffix, mime string
		data         []byte
	}{
		{"png", "image/png", rasterFixture(t)}, {"jpg", "image/jpeg", []byte{255, 216, 255, 217}},
		{"gif", "image/gif", []byte("GIF89a1234")}, {"webp", "image/webp", []byte("RIFF0000WEBP")}, {"avif", "image/avif", avif},
	} {
		t.Run(c.suffix, func(t *testing.T) {
			root := t.TempDir()
			source(t, root, "good."+c.suffix, string(c.data))
			source(t, root, "truncated."+c.suffix, string(c.data[:2]))
			source(t, root, "wrong."+c.suffix, "wrong header")
			p := source(t, root, "images.html", `<img alt="good" src="good.`+c.suffix+`"><img alt="short" src="truncated.`+c.suffix+`"><img alt="wrong" src="wrong.`+c.suffix+`">`)
			r := run(t, root, nil, p)
			success(t, r, 1)
			images := nodes(r.pages[0], "img")
			if len(images) != 1 {
				t.Fatalf("admitted images=%d", len(images))
			}
			payload, ok := strings.CutPrefix(attr(images[0], "src"), "data:"+c.mime+";base64,")
			decoded, err := base64.StdEncoding.DecodeString(payload)
			if !ok || err != nil || !bytes.Equal(decoded, c.data) {
				t.Fatal("raster representation changed")
			}
			body := textOf(nodes(r.pages[0], "body")[0])
			if !strings.Contains(body, "short") || !strings.Contains(body, "wrong") || r.stderr == "" {
				t.Fatal("refused images lack visible alternatives and notices")
			}
		})
	}
}

// W006 replaces implicit graph generation; explicit mixed batches retain these routes.
func TestRT008_9_MixedExplicitFileBatch(t *testing.T) {
	root := t.TempDir()
	source(t, root, "text.txt", "* Literal\n[not a link](never.md)")
	source(t, root, "code.go", "package main\n")
	source(t, root, "data.json", `{"value":1}`)
	source(t, root, "native.html", `<h1>Native</h1><a href="index.md">Cycle</a><a href="other.org">Org child</a>`)
	source(t, root, "other.org", "* Other\n")
	entry := source(t, root, "index.md", "# Entry\n[text](text.txt) [code](code.go) [json](data.json) [native](native.html)\n")
	for _, enabled := range []bool{false, true} {
		env := []string{}
		count := 1
		entries := []string{entry}
		if enabled {
			count = 6
			for _, name := range []string{"text.txt", "code.go", "data.json", "native.html", "other.org"} {
				entries = append(entries, filepath.Join(root, name))
			}
		}
		r := run(t, root, env, entries...)
		success(t, r, count)
		main := documentNode(t, r.pages[0], "hp-document")
		for _, a := range nodes(main, "a") {
			href := attr(a, "href")
			if enabled && !strings.Contains(href, ".html") {
				t.Fatalf("target not converted: %s", href)
			}
			if !enabled && !strings.Contains(href, root) {
				t.Fatal("graph-off lost original target")
			}
		}
	}
}

func TestRT008_6_BaseAndEscapedCSS(t *testing.T) {
	root := t.TempDir()
	source(t, root, "nested/pixel.png", string(rasterFixture(t)))
	child := source(t, root, "nested/child.org", "* Linked heading\n")
	input := `<head><base href="nested/"><base href="https://example.invalid/"><meta charset="utf-8" onload="bad()"><style>.a{background:u\72l(pixel.png)} .b{background:url("pixel.png")} .c{background:URL(https://example.invalid/x)} @\69mport "https://example.invalid/remote"; @media(max-width:30rem){.a{display:block}} @font-face{font-family:bad;src:url(pixel.png)}</style></head><body><a href="child.org">Child</a><img alt="responsive" src="pixel.png" srcset="pixel.png 1x, https://example.invalid/x 2x"></body>`
	r := run(t, root, nil, source(t, root, "base.html", input), child)
	success(t, r, 2)
	style := ""
	for _, n := range nodes(r.pages[0], "style") {
		style += textOf(n)
	}
	if strings.Count(style, "data:image/png;base64,") != 2 || strings.Contains(style, "example.invalid") || strings.Contains(style, "font-face") || !strings.Contains(style, "display:block") {
		t.Errorf("escaped/static stylesheet handling: %s", style)
	}
	for n := range r.pages[0].Descendants() {
		for _, a := range n.Attr {
			if strings.HasPrefix(a.Key, "on") {
				t.Fatal("native metadata retained event handler")
			}
		}
	}
	img := nodes(r.pages[0], "img")[0]
	if !strings.Contains(attr(img, "srcset"), "data:image/png;base64,") || strings.Contains(attr(img, "srcset"), "example.invalid") {
		t.Fatal("srcset resources not admitted individually")
	}
	for _, a := range nodes(r.pages[0], "a") {
		if textOf(a) == "Child" && !strings.Contains(attr(a, "href"), "0002.html") {
			t.Fatal("first base lost child context")
		}
	}
}

func TestRT008_6_LargeRasterData(t *testing.T) {
	root := t.TempDir()
	data := append(rasterFixture(t), bytes.Repeat([]byte{0}, 70<<10)...)
	value := "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
	r := run(t, root, nil, source(t, root, "large.md", "![large]("+value+")\n"))
	success(t, r, 1)
	images := nodes(r.pages[0], "img")
	if len(images) != 1 || attr(images[0], "src") != value {
		t.Fatal("valid bounded raster data lost to generic attribute limit")
	}
}

func TestRT008_6_NativeEncodingAndRemoteBase(t *testing.T) {
	root := t.TempDir()
	source(t, root, "pixel.png", string(rasterFixture(t)))
	for _, metadata := range []string{"", `<meta charset="windows-1252"><meta charset="utf-16">`} {
		input := `<head>` + metadata + `<base href="https://example.invalid/docs/"><title>Taḋg</title></head><body><p>Éire</p><img alt="remote relative image" src="pixel.png"><a href="next.org">Next</a></body>`
		r := run(t, root, nil, source(t, root, "unicode.html", input))
		success(t, r, 1)
		charsets := 0
		for _, n := range nodes(r.pages[0], "meta") {
			if attr(n, "charset") != "" {
				charsets++
				if attr(n, "charset") != "utf-8" {
					t.Error("published encoding differs from actual UTF-8 bytes")
				}
			}
		}
		if charsets != 1 || textOf(nodes(r.pages[0], "title")[0]) != "Taḋg" {
			t.Error("native UTF-8 representation lacks one unambiguous encoding declaration")
		}
		if len(nodes(r.pages[0], "img")) != 0 || len(nodes(r.pages[0], "base")) != 0 {
			t.Error("remote base authorized local image or survived publication")
		}
		if attr(nodes(r.pages[0], "a")[0], "href") != "https://example.invalid/docs/next.org" {
			t.Error("remote base changed explicit ordinary navigation")
		}
	}
}

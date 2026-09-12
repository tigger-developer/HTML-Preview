// ABOUTME: Checks source-relative navigation and passive-document boundaries.
// ABOUTME: Fixtures cover real Pandoc output rather than template text.
package preview

import (
	"encoding/base64"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func TestRT001_6_ReferenceEncoding(t *testing.T) {
	root := t.TempDir()
	asset := source(t, root, "notes/a  Taḋg %#?.png", string(rasterFixture(t)))
	encoded := (&url.URL{Path: filepath.Base(asset)}).String()
	p := source(t, root, "notes/doc.md", "# Heading\n\n[local]("+encoded+"?q=1#part)\n\n![alt]("+encoded+")\n\n[here](#heading) [web](https://example.invalid/a?q=1#f) [mail](mailto:a@example.invalid)\n")
	r := run(t, root, nil, p)
	success(t, r, 1)
	want := map[string]string{"local": (&url.URL{Scheme: "file", Path: asset, RawQuery: "q=1", Fragment: "part"}).String(), "here": "#heading", "web": "https://example.invalid/a?q=1#f", "mail": "mailto:a@example.invalid"}
	for _, a := range nodes(r.pages[0], "a") {
		if expected, ok := want[textOf(a)]; ok {
			if attr(a, "href") != expected {
				t.Errorf("%s: %q != %q", textOf(a), attr(a, "href"), expected)
			}
			delete(want, textOf(a))
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing links %v", want)
	}
	images := nodes(r.pages[0], "img")
	if len(images) != 1 || attr(images[0], "src") != "data:image/png;base64,"+base64.StdEncoding.EncodeToString(rasterFixture(t)) {
		t.Fatal("image origin or encoding")
	}
}

func TestRT001_7_OrgSearches(t *testing.T) {
	root := t.TempDir()
	a := source(t, root, "a.org", "* Entry\n[[file:b.org::#custom][custom]] [[file:b.org::*Target][heading]] [[id:alias][id]] [[file:b.org::unsupported][search]]\n")
	b := source(t, root, "b.org", "* Target\n:PROPERTIES:\n:CUSTOM_ID: custom\n:ID: alias\n:END:\nbody\n")
	r := run(t, root, nil, a, b)
	success(t, r, 2)
	for _, link := range nodes(r.pages[0], "a") {
		label := textOf(link)
		if label == "custom" || label == "heading" || label == "id" {
			u, err := url.Parse(attr(link, "href"))
			if err != nil || !strings.HasSuffix(u.Path, "0002.html") || (u.Fragment != "custom" && u.Fragment != "alias") {
				t.Errorf("Org destination %s: %s", label, attr(link, "href"))
			}
		}
	}
	if !strings.Contains(textOf(nodes(r.pages[0], "main")[0]), "unsupported") {
		t.Fatal("search explanation missing")
	}
}

func TestRT001_8_PartialFailure(t *testing.T) {
	root := t.TempDir()
	p := source(t, root, "good.md", "hello")
	r := run(t, root, nil, "missing.md", p)
	if r.code != 1 || len(r.pages) != 1 || len(r.opens) != 1 || !strings.Contains(r.stderr, "missing.md") {
		t.Fatalf("partial failure: %+v", r.opens)
	}
	r = run(t, root, []string{"PREVIEW_TEST_OPEN_FAIL=1"}, p)
	if r.code != 1 || len(r.opens) != 1 {
		t.Fatal("opener failure not reported")
	}
}

func TestRT001_11_SourceAndOutputLimits(t *testing.T) {
	root := t.TempDir()
	p := source(t, root, "doc.md", "abc")
	for _, v := range []string{"3", "4"} {
		r := run(t, root, []string{"HTMLPREVIEW_MAX_SOURCE_BYTES=" + v}, p)
		success(t, r, 1)
	}
	for _, s := range []string{"HTMLPREVIEW_MAX_SOURCE_BYTES=2", "HTMLPREVIEW_MAX_TOTAL_SOURCE_BYTES=2", "HTMLPREVIEW_MAX_OUTPUT_BYTES=1"} {
		r := run(t, root, []string{s}, p)
		if r.code != 1 || len(r.opens) != 0 {
			t.Fatalf("budget %s: status=%d", s, r.code)
		}
	}
}

func TestRT001_13_PassiveSource(t *testing.T) {
	root := t.TempDir()
	p := source(t, root, "doc.md", "# Passive\n\n<script>window.bad=1</script>\n\n<form><input value=secret></form>\n\n<iframe src='https://example.invalid'></iframe>\n\n<a href='javascript:alert(1)' onclick='bad()'>active</a>\n\n<img src='https://example.invalid/x' onerror='bad()' srcset='other.png 2x'>\n\n<div data-hp-source='spoof' style='background:url(https://example.invalid)'>kept</div>\n\n<svg><script>bad()</script></svg>\n\n`<script>literal</script>`\n")
	r := run(t, root, nil, p)
	success(t, r, 1)
	body := nodes(r.pages[0], "main")[0]
	for _, tag := range []string{"script", "style", "form", "input", "iframe", "svg", "base"} {
		if len(nodes(body, tag)) != 0 {
			t.Errorf("active %s remains", tag)
		}
	}
	for n := range body.Descendants() {
		for _, a := range n.Attr {
			if strings.HasPrefix(a.Key, "on") || a.Key == "style" || a.Key == "srcset" || a.Key == "data-hp-source" || strings.HasPrefix(a.Val, "javascript:") {
				t.Errorf("unsafe attribute %+v", a)
			}
		}
	}
	if !strings.Contains(textOf(body), "<script>literal</script>") || !strings.Contains(textOf(body), "kept") || r.stderr == "" {
		t.Fatal("passive content or diagnostics lost")
	}
	if !strings.Contains(r.raw[0], "default-src &#39;none&#39;") && !strings.Contains(r.raw[0], "default-src 'none'") {
		t.Fatal("CSP missing")
	}
	secret := source(t, root, "secret.org", "NEVER_INCLUDE_THIS")
	org := source(t, root, "include.org", "#+INCLUDE: \""+secret+"\"\n#+SETUPFILE: \""+secret+"\"\n")
	r = run(t, root, nil, org)
	success(t, r, 1)
	frontmatter := documentNode(t, r.pages[0], "hp-frontmatter")
	if strings.Contains(r.raw[0], "NEVER_INCLUDE_THIS") || !strings.Contains(textOf(frontmatter), "INCLUDE") || !strings.Contains(textOf(frontmatter), secret) {
		t.Fatal("Org include policy")
	}
}

func TestRT001_15_InputCompatibility(t *testing.T) {
	root := t.TempDir()
	source(t, root, "bad.md", string([]byte{0xff}))
	// W008 admits .txt; retain the unsupported-input check with an unmapped suffix.
	source(t, root, "other.unsupported", "text")
	for _, name := range []string{"bad.md", "other.unsupported", "absent.md", "."} {
		r := run(t, root, nil, name)
		if r.code != 1 || len(r.opens) != 0 {
			t.Errorf("input %s status=%d", name, r.code)
		}
	}
}

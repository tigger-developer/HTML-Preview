// ABOUTME: Challenges identifier ambiguity, literal contexts, and source policy.
// ABOUTME: Tests rendered outcomes using synthetic documents and local resources.
package preview

import (
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"strings"
	"testing"
)

func TestRT001_7_BareHTMLHeadings(t *testing.T) {
	root := t.TempDir()
	p := source(t, root, "doc.md", `<h1 id="raw-one">One</h1><p>Body one.</p><h2 id="raw-two">Two</h2><p>Body two.</p>`)
	r := run(t, root, nil, p)
	success(t, r, 1)
	for _, want := range []string{"raw-one", "raw-two"} {
		found := false
		for n := range r.pages[0].Descendants() {
			if attr(n, "id") == want {
				found = true
			}
		}
		if !found {
			t.Errorf("heading destination %q disappeared", want)
		}
	}
	if len(nodes(r.pages[0], "h1")) != 1 {
		t.Fatal("source heading duplicated the identity h1")
	}
}

func TestRT001_7_FormattedOrgHeadingSearch(t *testing.T) {
	root := t.TempDir()
	a := source(t, root, "a.org", "[[file:b.org::*A formatted title][next]]\n")
	b := source(t, root, "b.org", "* TODO [#A] A /formatted/ *title* :tag:\nBody\n")
	r := run(t, root, nil, a, b)
	success(t, r, 2)
	for _, a := range nodes(r.pages[0], "a") {
		if textOf(a) == "next" {
			u, err := url.Parse(attr(a, "href"))
			if err != nil || !strings.HasSuffix(u.Path, "0002.html") || u.Fragment == "" {
				t.Fatal("formatted heading search did not resolve")
			}
			return
		}
	}
	t.Fatal("missing search link")
}

func TestRT001_10_ResourceLinksDoNotDiscover(t *testing.T) {
	root := t.TempDir()
	p := source(t, root, "a.md", "<link rel=\"stylesheet\" href=\"b.org\">\n")
	source(t, root, "b.org", "* Not an anchor target\n")
	r := run(t, root, []string{"HTMLPREVIEW_LINKS=1"}, p)
	success(t, r, 1)
}

func TestRT001_13_ResourceLinksKeepOriginalDestinations(t *testing.T) {
	root := t.TempDir()
	a := source(t, root, "a.md", "<link rel=\"stylesheet\" href=\"b.org\">\n")
	b := source(t, root, "b.org", "* Explicit document\n")
	r := run(t, root, []string{"HTMLPREVIEW_LINKS=1"}, a, b)
	success(t, r, 2)
	links := nodes(r.pages[0], "a")
	if len(links) != 1 {
		t.Fatal("resource placeholder missing")
	}
	u, err := url.Parse(attr(links[0], "href"))
	if err != nil || u.Scheme != "file" || u.Path != b {
		t.Fatalf("resource became document navigation: %s", attr(links[0], "href"))
	}
}

func TestRT001_13_BoundedSourceWarnings(t *testing.T) {
	root := t.TempDir()
	p := source(t, root, "doc.md", strings.Repeat("<span onclick=\"alert(1)\">text</span>\n", 1600))
	r := run(t, root, []string{"PREVIEW_TEST_FAULT=cleanup"}, p)
	if r.code != 1 || len(r.stderr) > 67000 || !strings.Contains(r.stderr, "further source warnings omitted") || !strings.Contains(r.stderr, "remaining directory") {
		t.Fatalf("warning bound/cleanup diagnostic: status %d bytes %d", r.code, len(r.stderr))
	}
}

func TestRT001_4_NestedOrgInclude(t *testing.T) {
	root := t.TempDir()
	secret := source(t, root, "secret.org", "DO_NOT_EXPAND")
	p := source(t, root, "doc.org", "#+begin_quote\n#+INCLUDE: \""+secret+"\"\n#+end_quote\n#+begin_comment\n#+begin_src\nSCHEDULED: <2026-09-08 Tue>\n#+end_src\n#+end_comment\n#+begin_example\n\t:LOGBOOK_:\n#+INCLUDE: literal\n")
	r := run(t, root, nil, p)
	success(t, r, 1)
	body := textOf(nodes(r.pages[0], "main")[0])
	if strings.Contains(body, "DO_NOT_EXPAND") || !strings.Contains(body, secret) || !strings.Contains(body, "\t:LOGBOOK_:") {
		t.Fatalf("literal/include boundary: %s", body)
	}
}

func TestRT001_4_LiteralBlockNames(t *testing.T) {
	root := t.TempDir()
	p := source(t, root, "doc.org", "* Before\n#+begin_srcother\nordinary\n#+end_srcother\n* After\n")
	r := run(t, root, nil, p)
	success(t, r, 1)
	if len(nodes(r.pages[0], "h2")) != 2 {
		t.Fatal("a block-name prefix swallowed the following heading")
	}
}

func TestRT001_7_InvalidAndDuplicateIDs(t *testing.T) {
	root := t.TempDir()
	p := source(t, root, "doc.org", "* First\n:PROPERTIES:\n:ID: invalid id\n:CUSTOM_ID: duplicate\n:END:\n* Second\n:PROPERTIES:\n:CUSTOM_ID: duplicate\n:END:\n[[id:invalid id][invalid]] [[file:doc.org::#duplicate][duplicate]]\n")
	r := run(t, root, nil, p)
	success(t, r, 1)
	for _, a := range nodes(r.pages[0], "a") {
		if textOf(a) == "invalid" && attr(a, "href") != "" {
			t.Fatal("invalid source ID became active")
		}
	}
	ids := make(map[string]bool)
	for n := range r.pages[0].Descendants() {
		id := attr(n, "id")
		if id != "" {
			if ids[id] {
				t.Fatalf("duplicate output ID %q", id)
			}
			ids[id] = true
		}
	}
	if !strings.Contains(r.stderr, "ambiguous") && !strings.Contains(r.stderr, "not resolved") {
		t.Fatal("ambiguous source IDs were not diagnosed")
	}
}

func TestRT001_13_StylesheetPlaceholder(t *testing.T) {
	root := t.TempDir()
	p := source(t, root, "doc.md", "Before.\n\n<link rel=\"stylesheet\" href=\"https://example.invalid/theme.css\">\n\nAfter.\n")
	r := run(t, root, nil, p)
	success(t, r, 1)
	found := false
	for _, a := range nodes(r.pages[0], "a") {
		if attr(a, "href") == "https://example.invalid/theme.css" {
			found = true
		}
	}
	if !found {
		t.Fatal("unsupported stylesheet lost its explanatory link")
	}
}

func TestRT001_5_CSPPayloadHashes(t *testing.T) {
	root := t.TempDir()
	p := source(t, root, "doc.md", "Text")
	r := run(t, root, nil, p)
	success(t, r, 1)
	policy := ""
	for _, m := range nodes(r.pages[0], "meta") {
		if attr(m, "http-equiv") == "Content-Security-Policy" {
			policy = attr(m, "content")
		}
	}
	for _, tag := range []string{"style", "script"} {
		payload := nodes(r.pages[0], tag)
		if len(payload) != 1 {
			t.Fatalf("expected one owned %s", tag)
		}
		sum := sha256.Sum256([]byte(textOf(payload[0])))
		if !strings.Contains(policy, "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'") {
			t.Fatalf("%s hash differs from emitted payload", tag)
		}
	}
	if strings.Contains(policy, "unsafe-") {
		t.Fatal("unsafe CSP directive")
	}
}

func TestRT001_7_UniqueRawIDs(t *testing.T) {
	root := t.TempDir()
	p := source(t, root, "doc.md", "<div id=\"same\">one</div>\n<div id=\"same\">two</div>\n\n# Title {#same}\n")
	r := run(t, root, nil, p)
	success(t, r, 1)
	ids := make(map[string]bool)
	for n := range r.pages[0].Descendants() {
		id := attr(n, "id")
		if id != "" {
			if ids[id] {
				t.Fatalf("duplicate output ID %q", id)
			}
			ids[id] = true
		}
	}
}

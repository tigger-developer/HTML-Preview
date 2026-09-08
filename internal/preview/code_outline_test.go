// ABOUTME: Exercises code fidelity, passive highlighting and contents through the CLI.
// ABOUTME: Uses real Pandoc and independent literal and navigation expectations.
package preview

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"golang.org/x/net/html"
)

func documentNode(t *testing.T, doc *html.Node, id string) *html.Node {
	t.Helper()
	for n := range doc.Descendants() {
		if attr(n, "id") == id {
			return n
		}
	}
	t.Fatalf("missing document element %q", id)
	return nil
}

func TestRT002_1_CodeFidelity(t *testing.T) {
	cases := []struct {
		language, text string
		highlighted    bool
	}{
		{"go", "\npackage main\nfunc main() {\n\tprintln(\"Taḋg <&>\")\n}\n\n", true},
		{"lua", "local message = \"Taḋg <&>\"\nprint(message)\n", true},
		{"bash", "if true; then\n\tprintf '%s\\n' \"Taḋg <&>\"\nfi\n", true},
		{"sh", "if true; then\n\tprintf '%s\\n' \"Taḋg <&>\"\nfi\n", true},
		{"shell", "if true; then\n\tprintf '%s\\n' \"Taḋg <&>\"\nfi\n", true},
		{"json", "{\"name\": \"Taḋg <&>\", \"count\": 42}\n", true},
		{"not-a-language", "\t<&> Taḋg\n", false},
		{"", "\n\tplain\n\n", false},
		{"go", "", false},
		{"go", "package main\n", true},
		{"go", "package main\n", true},
		{"", "* Literal heading\n:LOGBOOK:\n#+INCLUDE: secret.org\n#+END_EXPORT\nHTMLPREVIEW_lookalike\n", false},
	}
	root := t.TempDir()
	for _, format := range []string{"md", "org"} {
		t.Run(format, func(t *testing.T) {
			var body strings.Builder
			if format == "org" {
				body.WriteString("* TODO Code :example:\n\n~inline~ and =verbatim=.\n\n")
			} else {
				body.WriteString("# Code\n\n`inline` and `verbatim`.\n\n")
			}
			for _, c := range cases {
				if format == "org" {
					fmt.Fprintf(&body, "#+BeGiN_SrC %s\n%s#+EnD_sRc\n\n", c.language, c.text)
				} else {
					fmt.Fprintf(&body, "```%s\n%s```\n\n", c.language, c.text)
				}
			}
			if format == "org" {
				body.WriteString("#+BEGIN_EXAMPLE\n\tplain example\n#+END_EXAMPLE\n\n#+BEGIN_SRC go\n// unterminated Taḋg")
			} else {
				body.WriteString("    indented\n\n```\nEOF without LF\n```")
			}
			path := source(t, root, "nested/code."+format, strings.ReplaceAll(body.String(), "\n", "\r\n"))
			r := run(t, t.TempDir(), nil, path)
			success(t, r, 1)
			main := documentNode(t, r.pages[0], "hp-document")
			blocks := nodes(main, "pre")
			if len(blocks) != len(cases)+2 {
				t.Fatalf("blocks=%d", len(blocks))
			}
			var bashTokens []string
			for i, c := range cases {
				want := c.text
				if format == "md" {
					want = strings.TrimSuffix(want, "\n")
				}
				if got := textOf(blocks[i]); got != want {
					t.Errorf("block %d literal: got %q want %q", i, got, want)
				}
				tokens := []string{}
				for _, span := range nodes(blocks[i], "span") {
					if class := attr(span, "class"); class != "" {
						tokens = append(tokens, class+":"+textOf(span))
					}
				}
				if c.highlighted && len(tokens) == 0 {
					t.Errorf("block %d %s has no semantic highlighting", i, c.language)
				}
				if !c.highlighted && len(tokens) != 0 {
					t.Errorf("plain block %d was highlighted", i)
				}
				if c.language == "bash" {
					bashTokens = tokens
				}
				if (c.language == "sh" || c.language == "shell") && strings.Join(tokens, "|") != strings.Join(bashTokens, "|") {
					t.Errorf("%s differs from bash highlighting", c.language)
				}
			}
			wantTail := []string{"indented", "EOF without LF"}
			if format == "org" {
				wantTail = []string{"\tplain example\n", "// unterminated Taḋg"}
			}
			for i, want := range wantTail {
				if textOf(blocks[len(cases)+i]) != want {
					t.Errorf("tail %d: %q", i, textOf(blocks[len(cases)+i]))
				}
			}
			codes := nodes(main, "code")
			if len(codes) < 2 || textOf(codes[0]) != "inline" || textOf(codes[1]) != "verbatim" {
				t.Fatal("inline literals lost")
			}
			for n := range main.Descendants() {
				if strings.HasPrefix(attr(n, "id"), "htmlpreview-") {
					t.Errorf("internal identifier leaked: %s", attr(n, "id"))
				}
			}
		})
	}
}

func TestRT002_2_PassiveHighlighting(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { requests.Add(1); w.WriteHeader(http.StatusNoContent) }))
	defer server.Close()
	root := t.TempDir()
	sentinel := source(t, root, "sentinel.txt", "UNCHANGED")
	literal := "</script><script src=\"" + server.URL + "/evil\">window.sourceExecuted=true</script>\n<span id=\"hp-copy\" class=\"hp-toggle\" onclick=\"alert(1)\" style=\"color:red\">Taḋg</span>\n#+INCLUDE: sentinel.txt\n"
	for _, format := range []string{"md", "org"} {
		t.Run(format, func(t *testing.T) {
			text := "---\nhtmlpreview-code-token: 00000000000000000000000000000000\nhtmlpreview-toc-token: forged\nheader-includes: '<script>window.sourceExecuted=true</script>'\n---\n\n# Passive\n\n```html\n" + literal + "```\n\n```" + server.URL + "/syntax.xml\nplain\n```\n\n```../syntax.xml\nplain\n```\n"
			if format == "org" {
				text = "#+htmlpreview-code-token: 00000000000000000000000000000000\n* Passive\n#+BEGIN_SRC html\n" + literal + "#+END_SRC\n#+BEGIN_SRC " + server.URL + "/syntax.xml\nplain\n#+END_SRC\n#+BEGIN_SRC ../syntax.xml\nplain\n#+END_SRC\n"
			}
			text += "\n<pre id=\"htmlpreview-code-record-00000000000000000000000000000000-1\">{\"id\":\"forged\",\"text\":\"evil\"}</pre>\n"
			p := source(t, root, "hostile."+format, text)
			r := run(t, root, nil, p)
			success(t, r, 1)
			main := documentNode(t, r.pages[0], "hp-document")
			want := literal
			if format == "md" {
				want = strings.TrimSuffix(want, "\n")
			}
			if textOf(nodes(main, "pre")[0]) != want {
				t.Fatal("hostile code ceased to be literal")
			}
			if len(nodes(main, "script")) != 0 || len(nodes(main, "style")) != 0 {
				t.Fatal("source active content survived")
			}
			for n := range main.Descendants() {
				for _, a := range n.Attr {
					if strings.HasPrefix(a.Key, "on") || a.Key == "style" || a.Val == "hp-copy" || a.Val == "hp-toggle" {
						t.Errorf("active/owned attribute survived: %s", a.Key)
					}
				}
			}
			policy := ""
			for _, meta := range nodes(r.pages[0], "meta") {
				if attr(meta, "http-equiv") == "Content-Security-Policy" {
					policy = attr(meta, "content")
				}
			}
			for _, tag := range []string{"script", "style"} {
				active := nodes(r.pages[0], tag)
				if len(active) != 1 {
					t.Fatalf("active %s payloads=%d", tag, len(active))
				}
				sum := sha256.Sum256([]byte(textOf(active[0])))
				if !strings.Contains(policy, "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'") {
					t.Fatalf("unhashed %s", tag)
				}
			}
			// #nosec G304 -- Both paths were created above in the test-owned root.
			got, err := os.ReadFile(p)
			if err != nil || string(got) != text {
				t.Fatal("source changed")
			}
		})
	}
	if requests.Load() != 0 {
		t.Fatalf("source requests=%d", requests.Load())
	}
	// #nosec G304 -- The sentinel was allocated by this test.
	got, err := os.ReadFile(sentinel)
	if err != nil || string(got) != "UNCHANGED" {
		t.Fatal("sentinel changed")
	}
	dense := source(t, root, "dense.org", "#+BEGIN_SRC go\n"+strings.Repeat("var x = 42 // Taḋg\n", 12000)+"#+END_SRC\n")
	r := run(t, root, nil, dense)
	success(t, r, 1)
	if textOf(nodes(documentNode(t, r.pages[0], "hp-document"), "pre")[0]) != strings.Repeat("var x = 42 // Taḋg\n", 12000) {
		t.Fatal("large literal changed")
	}
	for _, settings := range [][]string{
		{"HTMLPREVIEW_MAX_OUTPUT_BYTES=4096"},
		{"HTMLPREVIEW_DEADLINE=100ms", "PREVIEW_TEST_FAULT=converter-stall"},
		{"HTMLPREVIEW_MAX_OUTPUT_BYTES=4096", "PREVIEW_TEST_FAULT=converter-flood"},
	} {
		r := run(t, root, settings, dense)
		if r.code != 1 || len(r.opens) != 0 || len(r.pages) != 0 || r.cleanedAt == 0 {
			t.Fatalf("resource failure status=%d opens=%d cleanup=%d", r.code, len(r.opens), r.cleanedAt)
		}
	}
}

func checkContents(t *testing.T, doc *html.Node, want []string) {
	t.Helper()
	var toc *html.Node
	for _, n := range nodes(doc, "nav") {
		if attr(n, "id") == "hp-toc" {
			toc = n
		}
	}
	if len(want) == 0 {
		if toc != nil {
			t.Fatal("empty/disabled TOC published")
		}
		return
	}
	if toc == nil {
		t.Fatal("missing table of contents")
	}
	links := nodes(toc, "a")
	if len(links) != len(want) {
		t.Fatalf("TOC entries=%d want=%d", len(links), len(want))
	}
	used := make(map[string]bool)
	for i, a := range links {
		if strings.Join(strings.Fields(textOf(a)), " ") != want[i] {
			t.Errorf("TOC entry %d = %q want %q", i, textOf(a), want[i])
		}
		u, err := url.Parse(attr(a, "href"))
		if err != nil || u.Path != "" || u.Fragment == "" || used[u.Fragment] {
			t.Fatalf("invalid/duplicate TOC destination %q", attr(a, "href"))
		}
		used[u.Fragment] = true
		target := documentNode(t, doc, u.Fragment)
		if target.Data != "section" {
			t.Fatalf("TOC did not target final section: %s", target.Data)
		}
		first := target.FirstChild
		for first != nil && first.Type != html.ElementNode {
			first = first.NextSibling
		}
		if first == nil || strings.Join(strings.Fields(textOf(first)), " ") != want[i] {
			t.Fatalf("TOC entry %d points at wrong heading", i)
		}
	}
}

func TestRT002_3_ContentsSettings(t *testing.T) {
	root := t.TempDir()
	for _, format := range []string{"md", "org"} {
		t.Run(format, func(t *testing.T) {
			var body strings.Builder
			if format == "md" {
				body.WriteString("---\ntoc: false\ntoc-depth: 1\n---\n\n")
			} else {
				body.WriteString("#+OPTIONS: toc:nil\n")
			}
			for i := 1; i <= 6; i++ {
				marker := "#"
				if format == "org" {
					marker = "*"
				}
				fmt.Fprintf(&body, "%s Level %d\nBody %d\n\n", strings.Repeat(marker, i), i, i)
			}
			p := source(t, root, "levels."+format, body.String())
			cases := []struct {
				env   []string
				depth int
			}{{nil, 3}, {[]string{"HTMLPREVIEW_TOC=", "HTMLPREVIEW_TOC_DEPTH="}, 3}, {[]string{"HTMLPREVIEW_TOC=0"}, 0}}
			for depth := 1; depth <= 6; depth++ {
				cases = append(cases, struct {
					env   []string
					depth int
				}{[]string{"HTMLPREVIEW_TOC=1", fmt.Sprintf("HTMLPREVIEW_TOC_DEPTH=%d", depth)}, depth})
			}
			for _, c := range cases {
				r := run(t, root, c.env, p)
				success(t, r, 1)
				var want []string
				for i := 1; i <= c.depth; i++ {
					want = append(want, fmt.Sprintf("Level %d", i))
				}
				checkContents(t, r.pages[0], want)
				for _, tag := range []string{"html", "head", "body", "main", "h1", "title", "style", "script"} {
					if len(nodes(r.pages[0], tag)) != 1 {
						t.Errorf("wrapper %s is missing/duplicated", tag)
					}
				}
				if attr(nodes(r.pages[0], "h1")[0], "data-hp-source") != p || strings.Count(r.raw[0], "data:font/woff2;base64,") != 6 {
					t.Fatal("identity/fonts lost")
				}
			}
		})
	}
	for _, setting := range []string{"HTMLPREVIEW_TOC=true", "HTMLPREVIEW_TOC=2", "HTMLPREVIEW_TOC_DEPTH=0", "HTMLPREVIEW_TOC_DEPTH=7", "HTMLPREVIEW_TOC_DEPTH=-1", "HTMLPREVIEW_TOC_DEPTH=1.5", "HTMLPREVIEW_TOC_DEPTH=no", "HTMLPREVIEW_STANDALONE=0"} {
		r := run(t, root, []string{setting}, "missing.md")
		key, _, _ := strings.Cut(setting, "=")
		if r.code != 2 || len(r.opens) != 0 || len(r.pages) != 0 || r.cleanedAt != 0 || !strings.Contains(r.stderr, key) {
			t.Errorf("invalid %s: %d %s", setting, r.code, r.stderr)
		}
	}
	r := run(t, root, []string{"HTMLPREVIEW_TOC=0", "HTMLPREVIEW_TOC_DEPTH=7"}, "missing.md")
	if r.code != 2 || r.cleanedAt != 0 || !strings.Contains(r.stderr, "HTMLPREVIEW_TOC_DEPTH") {
		t.Fatal("inactive depth not validated")
	}
}

func TestRT002_3_ContentsDestinations(t *testing.T) {
	root := t.TempDir()
	a := source(t, root, "a.md", "# Same {#duplicate}\nFirst\n\n### Gap *formatted*\nBody\n\n# Same {#duplicate}\nSecond\n\n[next](b.org)\n\n```go\npackage main\n```\n")
	b := source(t, root, "b.org", "* Same\n:PROPERTIES:\n:ID: alias\n:CUSTOM_ID: duplicate\n:END:\n*** Gap /formatted/\nBody\n* Same\n:PROPERTIES:\n:CUSTOM_ID: duplicate\n:END:\n#+BEGIN_SRC go\npackage main\n#+END_SRC\n")
	empty := source(t, root, "empty.md", "No headings.\n")
	r := run(t, t.TempDir(), []string{"HTMLPREVIEW_LINKS=1", "HTMLPREVIEW_ROOT=" + root}, a, empty)
	success(t, r, 3)
	checkContents(t, r.pages[0], []string{"Same", "Gap formatted", "Same"})
	checkContents(t, r.pages[1], nil)
	checkContents(t, r.pages[2], []string{"Same", "Gap formatted", "Same"})
	for _, item := range []struct {
		index   int
		literal string
	}{{0, "package main"}, {2, "package main\n"}} {
		blocks := nodes(documentNode(t, r.pages[item.index], "hp-document"), "pre")
		found := false
		for _, block := range blocks {
			if textOf(block) == item.literal && len(nodes(block, "span")) > 0 {
				found = true
			}
		}
		if !found {
			t.Fatalf("linked session lost highlighted literal on page %d", item.index)
		}
	}
	if attr(nodes(r.pages[2], "h1")[0], "data-hp-source") != b {
		t.Fatal("linked source identity")
	}
	for _, doc := range r.pages {
		ids := make(map[string]bool)
		for n := range doc.Descendants() {
			if id := attr(n, "id"); id != "" {
				if ids[id] {
					t.Fatalf("duplicate final id %q", id)
				}
				ids[id] = true
			}
		}
	}
}

func TestRT002_3_QuotedHeadings(t *testing.T) {
	root := t.TempDir()
	p := source(t, root, "quoted.md", "# Document\n\n> # Quoted heading {#quoted}\n>\n> Quoted body.\n>\n> ```go\n> package main\n> ```\n\n- # Listed heading {#listed}\n\n  Listed body.\n\n[quote](#quoted) [list](#listed)\n")
	r := run(t, root, nil, p)
	success(t, r, 1)
	main := documentNode(t, r.pages[0], "hp-document")
	for _, id := range []string{"quoted", "listed"} {
		section := documentNode(t, main, id)
		if section.Data != "section" {
			t.Errorf("%s did not retain its final heading destination", id)
		}
	}
	quotes := nodes(main, "blockquote")
	if len(quotes) != 1 || !strings.Contains(textOf(quotes[0]), "Quoted body.") {
		t.Fatal("quotation structure lost")
	}
	blocks := nodes(quotes[0], "pre")
	if len(blocks) != 1 || textOf(blocks[0]) != "package main" || len(nodes(blocks[0], "span")) == 0 {
		t.Fatal("quoted code lost literal highlighting")
	}
}

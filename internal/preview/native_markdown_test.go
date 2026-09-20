// ABOUTME: Exercises native Markdown migration through CLI and served HTML boundaries.
// ABOUTME: Protects optional dependencies, semantic adaptation and omission diagnostics.
package preview

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

const omittedHTMLWarning = "Embedded HTML omitted: This Markdown source contains HTML that is not rendered in this preview. Some content may be missing."

func markdownFixture(t *testing.T, name string) string {
	t.Helper()
	// #nosec G304 -- The caller selects a committed synthetic fixture basename.
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "native-markdown", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The opener is an external boundary already intercepted by the subprocess harness.
// A private executable PATH entry makes dependency absence independent of the host.
func nativeOnlyPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"open", "xdg-open"} {
		if err := os.Symlink(exe, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	return "PATH=" + dir
}

func TestRT014_1_NativeMarkdownCLI(t *testing.T) {
	root := t.TempDir()
	env := []string{nativeOnlyPath(t)}
	for _, suffix := range []string{"md", "markdown", "mdown", "mkd", "mkdn", "MD", "MarkDown"} {
		t.Run(suffix, func(t *testing.T) {
			path := source(t, root, "document."+suffix, "# Native Markdown\n\nA native paragraph.\n")
			r := run(t, root, env, path)
			success(t, r, 1)
			if !strings.Contains(textOf(documentNode(t, r.pages[0], "hp-document")), "A native paragraph.") {
				t.Fatal("native body missing")
			}
		})
	}
	t.Run("explicit reader", func(t *testing.T) {
		p := source(t, root, "document.unusual", "# Explicit native reader\n")
		success(t, run(t, root, env, "--from=markdown", p), 1)
	})
}

func TestRT014_1_ServedMarkdownNeverInvokesPandoc(t *testing.T) {
	host := NativeHost()
	execute := host.Execute
	var calls atomic.Int32
	host.Execute = func(ctx context.Context, cmd Command) ([]byte, error) {
		if filepath.Base(cmd.Path) == "pandoc" {
			calls.Add(1)
		}
		return execute(ctx, cmd)
	}
	s := startTestService(t, host)
	source(t, s.root, "next.md", "# Linked Markdown\n\nA paragraph to annotate.\n\n[Optional reader](later.markdown_strict)\n")
	source(t, s.root, "later.markdown_strict", "# Later optional document\n")
	p := source(t, s.root, "index.org", "* Index\n[[file:next.md][Next]]\n")
	endpoint := annotationRegistrationURL(t, s, p, "Reviewer")
	status, _, body := responseAsset(t, s, "GET", strings.Replace(endpoint, "/_annotations/v2/", "/", 1))
	if status != 200 {
		t.Fatal("index", status)
	}
	var link string
	for _, a := range nodes(documentNode(t, parseHTTPDocument(t, body), "hp-document"), "a") {
		if textOf(a) == "Next" {
			link = attr(a, "href")
		}
	}
	if link == "" {
		t.Fatal("linked destination missing")
	}
	status, _, body = responseAsset(t, s, "GET", link)
	if status != 200 || !strings.Contains(string(body), "A paragraph to annotate.") {
		t.Fatal("linked native preview", status)
	}
	endpoint = annotationRegistrationURL(t, s, filepath.Join(s.root, "next.md"), "Reviewer")
	_, state := markdownBrowserPage(t, s, strings.Replace(endpoint, "/_annotations/v2/", "/", 1))
	main, state := markdownBrowserPage(t, s, state["page_url"].(string))
	markdownCreateAtBlock(t, s, state, assertClickableAnnotationText(t, main, "A paragraph to annotate"), "reviewer-001", 1)
	main, _ = markdownBrowserPage(t, s, state["page_url"].(string))
	assertClickableAnnotationText(t, main, "A paragraph to annotate")
	if calls.Load() != 0 {
		t.Fatalf("Markdown invoked Pandoc %d times", calls.Load())
	}
}

func TestRT014_2_NativeDialect(t *testing.T) {
	root := t.TempDir()
	cases := []struct{ name, source, selector, want, absent string }{
		{"soft-break", "First line\nsecond line.\n", "", "First line second line.", ""},
		{"smart-on", "A range 1--3...\n", "markdown+smart", "1–3…", ""},
		{"smart-off", "A range 1--3...\n", "markdown-smart", "1--3...", ""},
		{"smart-last", "A range 1--3...\n", "markdown-smart+smart", "1–3…", ""},
		{"fenced-div", "::: {.special}\nText\n:::\n", "", "::: {.special}", ""},
		{"escaped", "A \\*literal\\* and <https://example.com>.\n", "", "*literal*", ""},
		{"empty", "", "", "", ""},
		{"unicode-crlf", "# Héllo\r\n\r\nTaḋg and café.\r\n", "", "Taḋg and café.", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := source(t, root, tc.name+".md", tc.source)
			args := []string{p}
			if tc.selector != "" {
				args = append([]string{"--from=" + tc.selector}, args...)
			}
			r := run(t, root, nil, args...)
			success(t, r, 1)
			main := documentNode(t, r.pages[0], "hp-document")
			if tc.name == "empty" && strings.TrimSpace(textOf(main)) != "" {
				t.Fatal("empty document gained body content")
			}
			if !strings.Contains(strings.Join(strings.Fields(textOf(main)), " "), tc.want) {
				t.Fatalf("want %q in %q", tc.want, textOf(main))
			}
			if tc.name == "soft-break" && len(nodes(main, "br")) != 0 {
				t.Fatal("soft newline became hard break")
			}
			if tc.name == "fenced-div" {
				for _, n := range nodes(main, "div") {
					if hasClass(n, "special") {
						t.Fatal("unsupported fenced div was interpreted")
					}
				}
			}
		})
	}
	for _, ending := range []string{"  \n", "\\\n"} {
		t.Run(fmt.Sprintf("hard-break-%q", ending), func(t *testing.T) {
			p := source(t, root, "break.md", "First line"+ending+"second line.\n")
			r := run(t, root, nil, p)
			success(t, r, 1)
			if len(nodes(documentNode(t, r.pages[0], "hp-document"), "br")) != 1 {
				t.Fatal("hard break missing")
			}
		})
	}
	for _, selector := range []string{"markdown+raw_html", "markdown+fenced_divs", "markdown+unknown", "markdown+" + strings.Repeat("x", 256)} {
		t.Run(selector, func(t *testing.T) {
			p := source(t, root, "option.md", "text\n")
			r := run(t, root, []string{nativeOnlyPath(t)}, "--from="+selector, p)
			if r.code != 2 {
				t.Fatalf("unsupported selector status=%d: %s", r.code, r.stderr)
			}
		})
	}
}

func TestRT014_2_MetadataAndHeadingContract(t *testing.T) {
	root := t.TempDir()
	p := source(t, root, "semantics.md", markdownFixture(t, "semantics.md"))
	r := run(t, root, nil, p)
	success(t, r, 1)
	main := documentNode(t, r.pages[0], "hp-document")
	for _, want := range []string{"Native sample", "Synthetic author", "2026-09-20", "en-IE", "Taḋg", "Definition body", "Second note paragraph"} {
		if !strings.Contains(textOf(main), want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, tag := range []string{"em", "strong", "blockquote", "ul", "ol", "table", "dl", "dt", "dd", "pre", "code"} {
		if len(nodes(main, tag)) == 0 {
			t.Errorf("missing semantic %s", tag)
		}
	}
	ids := map[string]bool{}
	for n := range main.Descendants() {
		if id := attr(n, "id"); id != "" {
			if ids[id] {
				t.Errorf("duplicate id %q", id)
			}
			ids[id] = true
		}
	}
	for _, id := range []string{"héllo-world", "héllo-world-1", "explicit"} {
		if !ids[id] {
			t.Errorf("missing heading ID %q", id)
		}
	}
	if len(nodes(r.pages[0], "title")) != 1 || textOf(nodes(r.pages[0], "title")[0]) != "semantics.md" {
		t.Fatal("filename title fallback changed")
	}
	var refs, backs int
	for _, a := range nodes(main, "a") {
		if hasClass(a, "footnote-ref") {
			refs++
			if a.Parent.Data == "sup" || len(nodes(a, "sup")) != 1 || attr(a, "role") != "doc-noteref" {
				t.Error("footnote reference contract")
			}
			if !ids[strings.TrimPrefix(attr(a, "href"), "#")] {
				t.Error("unresolved footnote")
			}
		}
		if hasClass(a, "footnote-back") {
			backs++
			if attr(a, "role") != "doc-backlink" || !ids[strings.TrimPrefix(attr(a, "href"), "#")] {
				t.Error("unresolved backlink")
			}
		}
	}
	if refs != 2 || backs != 2 {
		t.Fatalf("references=%d backlinks=%d", refs, backs)
	}
	endnotes := 0
	for _, n := range nodes(main, "section") {
		if hasClass(n, "footnotes") {
			endnotes++
			if attr(n, "role") != "doc-endnotes" {
				t.Error("endnote semantics")
			}
		}
	}
	if endnotes != 1 {
		t.Fatalf("endnote sections=%d", endnotes)
	}
}

func TestRT014_2_InvalidMetadata(t *testing.T) {
	root := t.TempDir()
	for _, body := range []string{"---\ntitle: [unterminated\n---\nBody\n", "---\n- not-a-map\n---\nBody\n", "---\ntitle: a\ntitle: b\n---\nBody\n", "---\na: &a [*a]\n---\nBody\n"} {
		p := source(t, root, "invalid.md", body)
		r := run(t, root, nil, p)
		if r.code == 0 || len(r.pages) != 0 {
			t.Fatal("invalid metadata published a page")
		}
	}
	p := source(t, root, "unclosed.md", "---\nOrdinary unclosed delimiter.\n")
	success(t, run(t, root, nil, p), 1)
}

func TestRT014_3_OmittedHTMLCLI(t *testing.T) {
	root := t.TempDir()
	for _, tc := range []struct {
		name, body string
		warn       bool
	}{
		{"inline", "Before <b>retained text</b> after.\n", true},
		{"block", "<div>OMITTED_BLOCK_CONTENT</div>\n\nVisible paragraph.\n", true},
		{"comment", "<!-- authored comment -->\n\nVisible paragraph.\n", true},
		{"script", "<script>OMITTED_BLOCK_CONTENT</script>\n\nVisible paragraph.\n", true},
		{"style", "<style>OMITTED_BLOCK_CONTENT</style>\n\nVisible paragraph.\n", true},
		{"literal", "`<b>example</b>`\n\n```html\n<div>code example</div>\n```\n\n    <i>indented</i>\n\n<https://example.com> <reader@example.com> 1 < 2\n", false},
		{"escaped", "\\<b>literal\\</b>\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := source(t, root, tc.name+".md", tc.body)
			r := run(t, root, nil, p)
			success(t, r, 1)
			want := 0
			if tc.warn {
				want = 1
			}
			if n := strings.Count(r.stderr, "Embedded HTML omitted:"); n != want {
				t.Errorf("CLI notices=%d want=%d stderr=%s", n, want, r.stderr)
			}
			page := r.pages[0]
			main := documentNode(t, page, "hp-document")
			if n := strings.Count(textOf(page), omittedHTMLWarning); n != want {
				t.Errorf("page warnings=%d want=%d", n, want)
			}
			if strings.Contains(textOf(main), omittedHTMLWarning) || strings.Contains(textOf(main), "OMITTED_BLOCK_CONTENT") {
				t.Error("warning or omitted payload entered canonical body")
			}
			if tc.name == "inline" && (!strings.Contains(textOf(main), "Before retained text after.") || len(nodes(main, "b")) != 0) {
				t.Error("inline omission lost adjacent text or retained tag")
			}
			if tc.name == "literal" && !strings.Contains(textOf(main), "<div>code example</div>") {
				t.Error("literal HTML code lost")
			}
		})
	}
}

func TestRT014_3_OmissionRefresh(t *testing.T) {
	s := startTestService(t, NativeHost())
	p := source(t, s.root, "warning.md", "<div>OMITTED_BLOCK_CONTENT</div>\n\nVisible paragraph.\n")
	endpoint := annotationRegistrationURL(t, s, p, "Reviewer")
	pageURL := strings.Replace(endpoint, "/_annotations/v2/", "/", 1)
	for _, body := range []string{"<div>OMITTED_BLOCK_CONTENT</div>\n\nVisible paragraph.\n", "Visible paragraph.\n", "Before <b>visible</b> after.\n"} {
		if err := os.WriteFile(p, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			status, _, data := responseAsset(t, s, "GET", pageURL)
			if status != 200 {
				t.Fatal(status)
			}
			want := 0
			if strings.Contains(body, "<") {
				want = 1
			}
			page := parseHTTPDocument(t, data)
			if strings.Count(textOf(page), omittedHTMLWarning) != want {
				t.Fatal("warning did not follow source revision")
			}
			if strings.Contains(textOf(documentNode(t, page, "hp-document")), omittedHTMLWarning) {
				t.Fatal("warning pollutes annotation body")
			}
		}
	}
}

func TestRT014_6_OptionalDependencyCLI(t *testing.T) {
	root := t.TempDir()
	env := []string{nativeOnlyPath(t)}
	for _, tc := range []struct{ name, body string }{{"native.org", "* Native Org\n"}, {"native.txt", "literal\n"}, {"native.go", "package main\n"}, {"native.html", "<p>Native HTML</p>"}} {
		t.Run(tc.name, func(t *testing.T) {
			r := run(t, root, env, source(t, root, tc.name, tc.body))
			success(t, r, 1)
			if tc.name == "native.go" {
				p := nodes(documentNode(t, r.pages[0], "hp-document"), "pre")
				if len(p) != 1 || len(nodes(p[0], "span")) == 0 {
					t.Fatal("bundled source highlighting unavailable")
				}
			}
		})
	}
	r := run(t, root, env, "--list-input-formats")
	success(t, r, 0)
	for _, reader := range []string{"org", "markdown", "html"} {
		if !strings.Contains("\n"+r.stdout, "\n"+reader+"\n") {
			t.Errorf("native reader %s absent", reader)
		}
	}
	p := source(t, root, "office.docx", "not a real archive")
	r = run(t, root, env, p)
	if r.code == 0 || !strings.Contains(strings.ToLower(r.stderr), "pandoc") {
		t.Fatal("optional dependency failure not explained", r.code, r.stderr)
	}
}

func TestRT014_6_ChromaDoesNotNeedPandocLanguageAdmission(t *testing.T) {
	root := t.TempDir()
	p := source(t, root, "source.go", "package main\nfunc main() { println(42) }\n")
	r := run(t, root, []string{"PREVIEW_TEST_FAULT=missing-highlighter"}, p)
	success(t, r, 1)
	main := documentNode(t, r.pages[0], "hp-document")
	blocks := nodes(main, "pre")
	if len(blocks) != 1 || len(nodes(blocks[0], "span")) == 0 {
		t.Fatal("native Go highlighting depends on Pandoc language catalogue")
	}
	if strings.Contains(r.stderr, "highlighting language") {
		t.Fatal("native lexer incorrectly reported missing")
	}
}

func TestRT014_3_CLIServiceRegistrationWarning(t *testing.T) {
	s := startTestService(t, NativeHost())
	path := source(t, s.root, "registered.md", "<div>Omitted body</div>\n\nOrdinary paragraph.\n")
	configPath := source(t, s.root, "preview-config.yaml", "version: 1\nserve:\n  roots:\n    - "+s.root+"\n")
	host := NativeHost()
	execute := host.Execute
	opened := false
	host.Execute = func(ctx context.Context, cmd Command) ([]byte, error) {
		if filepath.Base(cmd.Path) == "open" || filepath.Base(cmd.Path) == "xdg-open" {
			if len(cmd.Args) != 1 || !strings.HasPrefix(cmd.Args[0], s.origin+"/") {
				return nil, fmt.Errorf("expected served preview URL")
			}
			opened = true
			return nil, nil
		}
		return execute(ctx, cmd)
	}
	var out, diagnostics strings.Builder
	env := []string{"HTMLPREVIEW_CONFIG=" + configPath, "HTMLPREVIEW_RUNTIME_DIR=" + s.runtime}
	code := Main([]string{path}, env, &out, &diagnostics, "test", "test", host)
	if code != 0 || !opened {
		t.Fatalf("served CLI handoff status=%d opened=%v: %s", code, opened, diagnostics.String())
	}
	if strings.Count(diagnostics.String(), "Embedded HTML omitted:") != 1 {
		t.Fatal("CLI registration did not report one warning", diagnostics.String())
	}
}

func TestRT014_2_ServedSelectorRejection(t *testing.T) {
	host := NativeHost()
	execute := host.Execute
	var calls atomic.Int32
	var reject atomic.Bool
	host.Execute = func(ctx context.Context, cmd Command) ([]byte, error) {
		if reject.Load() && filepath.Base(cmd.Path) == "pandoc" {
			calls.Add(1)
			return nil, fmt.Errorf("native qualifier must not invoke Pandoc")
		}
		return execute(ctx, cmd)
	}
	s := startTestService(t, host)
	target := s.register(t, "options.md", "# Options\n")
	reject.Store(true)
	for _, selector := range []string{"markdown+raw_html", "markdown+fenced_divs", "markdown+unknown", "markdown+" + strings.Repeat("x", 256)} {
		status, _, _ := responseAsset(t, s, "GET", target+"?htmlpreview-format="+url.QueryEscape(selector))
		if status != 400 {
			t.Errorf("unsupported native selector %s status=%d want=400", selector, status)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("native selector delegated to Pandoc", calls.Load())
	}
}

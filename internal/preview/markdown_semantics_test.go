// ABOUTME: Checks Markdown metadata, copy payloads, fragments and warning lifecycle.
// ABOUTME: Uses synthetic sources and HTTP without invoking a desktop browser.
package preview

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRT014_2_MetadataLayout(t *testing.T) {
	root := t.TempDir()
	metadata := "---\ntitle: Synthetic title\nauthor: Synthetic author\ndate: 2026-09-20\nlang: en-IE\n---\n"
	for _, body := range []string{"\nBody paragraph.\n", ""} {
		r := run(t, root, nil, source(t, root, "metadata.md", metadata+body))
		success(t, r, 1)
		main := documentNode(t, r.pages[0], "hp-document")
		expected := []string{"Synthetic title", "Synthetic author", "2026-09-20", "en-IE"}
		if body != "" {
			paragraphs := nodes(main, "p")
			if len(paragraphs) < 5 {
				t.Fatal("metadata paragraphs missing")
			}
			for i, want := range expected {
				if textOf(paragraphs[i]) != want {
					t.Errorf("metadata paragraph %d: %q want %q", i, textOf(paragraphs[i]), want)
				}
			}
		} else {
			if len(nodes(r.pages[0], "dl")) != 1 {
				t.Fatal("metadata-only definition list missing")
			}
			values := nodes(r.pages[0], "dd")
			expected = []string{"Synthetic author", "2026-09-20", "en-IE", "Synthetic title"}
			if len(values) != 4 {
				t.Fatal("metadata values missing")
			}
			for i, want := range expected {
				if textOf(values[i]) != want {
					t.Errorf("metadata value %d: %q", i, textOf(values[i]))
				}
			}
		}
	}
}

func TestRT014_2_CodeCopyContract(t *testing.T) {
	root := t.TempDir()
	for _, language := range []string{"go", "unknown-language"} {
		literal := "package main\n// <b>literal</b> and Taḋg"
		r := run(t, root, nil, source(t, root, "code.md", "```"+language+"\n"+literal+"\n```\n"))
		success(t, r, 1)
		blocks := nodes(documentNode(t, r.pages[0], "hp-document"), "pre")
		if len(blocks) != 1 || textOf(blocks[0]) != literal {
			t.Fatal("fenced code literal changed")
		}
		code := nodes(blocks[0], "code")[0]
		// Browser copy uses an owned base64 payload when present, otherwise textContent.
		copyText := textOf(code)
		if payload := attr(code, "data-hp-copy-base64"); payload != "" {
			copied, err := base64.StdEncoding.DecodeString(payload)
			if err != nil {
				t.Fatal(err)
			}
			copyText = string(copied)
		}
		if copyText != literal {
			t.Fatalf("copy payload %q", copyText)
		}
		if (len(nodes(code, "span")) > 0) != (language == "go") {
			t.Fatal("known/unknown lexer contract", language)
		}
	}
}

func TestRT014_2_FragmentLinks(t *testing.T) {
	s := startTestService(t, NativeHost())
	source(t, s.root, "other.md", "# Héllo world\n")
	target := s.register(t, "index.md", "# Local heading\n\n[local](#local-heading) [other](other.md#héllo-world)\n")
	status, _, body := responseAsset(t, s, "GET", target)
	if status != 200 {
		t.Fatal(status)
	}
	page := parseHTTPDocument(t, body)
	for _, a := range nodes(documentNode(t, page, "hp-document"), "a") {
		if textOf(a) != "local" && textOf(a) != "other" {
			continue
		}
		u, err := url.Parse(attr(a, "href"))
		if err != nil {
			t.Fatal(err)
		}
		fragment := u.Fragment
		if fragment == "" {
			t.Fatal("fragment dropped")
		}
		u.Fragment = ""
		dest := page
		if u.String() != "" {
			status, _, body = responseAsset(t, s, "GET", u.String())
			if status != 200 {
				t.Fatal(status)
			}
			dest = parseHTTPDocument(t, body)
		}
		documentNode(t, dest, fragment)
	}
}

func TestRT014_3_MultipleHTMLRegions(t *testing.T) {
	root := t.TempDir()
	body := "Before <b>inline</b> after.\n\n<div>OMITTED</div>\n\n<!-- comment -->\n\nVisible.\n"
	a := source(t, root, "first.md", body)
	b := source(t, root, "second.md", body)
	r := run(t, root, nil, a, b)
	success(t, r, 2)
	if strings.Count(r.stderr, "Embedded HTML omitted:") != 2 {
		t.Fatal("expected one CLI notice per document", r.stderr)
	}
	for _, page := range r.pages {
		if strings.Count(textOf(page), omittedHTMLWarning) != 1 {
			t.Fatal("expected one banner per page")
		}
	}
	r = run(t, root, nil, "--from=markdown-raw_html", a)
	success(t, r, 1)
	if strings.Count(r.stderr, "Embedded HTML omitted:") != 1 || strings.Contains(textOf(documentNode(t, r.pages[0], "hp-document")), "OMITTED") {
		t.Fatal("-raw_html bypassed omission notice")
	}
}

func TestRT014_3_LinkSaveAndStreamWarning(t *testing.T) {
	var logs bytes.Buffer
	s := startObservedTestService(t, NativeHost(), &logs)
	p := source(t, s.root, "warn.md", "Before <b>visible</b> after.\n\nA paragraph to annotate.\n")
	index := s.register(t, "index.org", "[[file:warn.md][Warning page]]\n")
	status, _, data := responseAsset(t, s, "GET", index)
	if status != 200 {
		t.Fatal(status)
	}
	var link string
	for _, a := range nodes(parseHTTPDocument(t, data), "a") {
		if textOf(a) == "Warning page" {
			link = attr(a, "href")
		}
	}
	if link == "" {
		t.Fatal("missing document link")
	}
	status, _, data = responseAsset(t, s, "GET", link)
	if status != 200 || strings.Count(textOf(parseHTTPDocument(t, data)), omittedHTMLWarning) != 1 {
		t.Fatal("linked preview warning missing", status)
	}
	endpoint := annotationRegistrationURL(t, s, p, "Reviewer")
	main, state := markdownBrowserPage(t, s, strings.Replace(endpoint, "/_annotations/v2/", "/", 1))
	markdownCreateAtBlock(t, s, state, assertClickableAnnotationText(t, main, "A paragraph to annotate"), "reviewer-001", 1)
	status, _, data = responseAsset(t, s, "GET", state["page_url"].(string))
	if status != 200 {
		t.Fatal(status)
	}
	page := parseHTTPDocument(t, data)
	if strings.Count(textOf(page), omittedHTMLWarning) != 1 {
		t.Fatal("save refresh lost warning")
	}
	front := documentNode(t, page, "hp-frontmatter")
	for n := range front.Descendants() {
		if attr(n, "data-hp-annotation-block") != "" {
			t.Fatal("warning region became writable")
		}
	}
	// Reconnect to unchanged source through the actual SSE boundary. No browser runs.
	for range 2 {
		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
		req, err := http.NewRequestWithContext(ctx, "GET", endpoint+"?events=1", nil)
		if err != nil {
			t.Fatal(err)
		}
		res, err := s.client.Do(req)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		reader := bufio.NewReader(res.Body)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				t.Fatal(err)
			}
			if line == "data: {}\n" {
				break
			}
		}
		if err := res.Body.Close(); err != nil {
			t.Fatal(err)
		}
		cancel()
	}
	if code := s.stop(); code != 0 {
		t.Fatal(code)
	}
	if strings.Contains(logs.String(), "Embedded HTML omitted:") {
		t.Fatal("background service traffic repeated CLI warning")
	}
}

func TestRT014_6_NativeHTMLAndPlainMappings(t *testing.T) {
	root := t.TempDir()
	for _, ext := range []string{"fish", "vim"} {
		literal := "<b>literal</b>\n* literal markup\n"
		r := run(t, root, []string{nativeOnlyPath(t)}, source(t, root, "source."+ext, literal))
		success(t, r, 1)
		blocks := nodes(documentNode(t, r.pages[0], "hp-document"), "pre")
		if len(blocks) != 1 || textOf(blocks[0]) != literal || len(nodes(blocks[0], "span")) != 0 {
			t.Fatal("plaintext mapping interpreted markup", ext)
		}
	}
	r := run(t, root, []string{nativeOnlyPath(t)}, source(t, root, "native.html", "<p><em>Authored HTML</em></p>"))
	success(t, r, 1)
	if len(nodes(documentNode(t, r.pages[0], "hp-document"), "em")) != 1 || strings.Contains(r.stderr, "Embedded HTML omitted:") || strings.Contains(textOf(r.pages[0]), omittedHTMLWarning) {
		t.Fatal("native HTML omission policy changed")
	}
}

func TestRT014_7_OutOfRootMarkdownImage(t *testing.T) {
	s := startTestService(t, NativeHost())
	outside := source(t, t.TempDir(), "outside.png", string(rasterFixture(t)))
	relative, err := filepath.Rel(s.root, outside)
	if err != nil {
		t.Fatal(err)
	}
	target := s.register(t, "rooted.md", "![outside]("+filepath.ToSlash(relative)+")\n\nSafe paragraph.\n")
	status, _, data := responseAsset(t, s, "GET", target)
	if status != 200 {
		t.Fatal(status)
	}
	for _, img := range nodes(parseHTTPDocument(t, data), "img") {
		if attr(img, "src") != "" {
			t.Fatal("out-of-root raster granted a served URL")
		}
	}
}

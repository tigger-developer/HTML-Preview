// ABOUTME: Exercises native Org routing and normalized HTML through public preview boundaries.
// ABOUTME: Keeps independent semantic oracles for converter differences and passive content.
package preview

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"golang.org/x/net/html"
)

func TestRT011_1_NativeOrgRouting(t *testing.T) {
	root := t.TempDir()
	for _, tc := range []struct {
		name, text string
		args       []string
	}{
		{"source.org", "* Native heading\nText.\n", nil},
		{"source.data", "* Native heading\nText.\n", []string{"--from=org"}},
		{"source.txt", "Plain literal text.\n", nil},
		{"source.go", "package main\n", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := source(t, root, tc.name, tc.text)
			r := run(t, root, []string{"PREVIEW_TEST_FAULT=reject-pandoc-org"}, append(tc.args, path)...)
			success(t, r, 1)
			if !strings.Contains(textOf(documentNode(t, r.pages[0], "hp-document")), strings.TrimSpace(strings.TrimPrefix(tc.text, "* "))) && tc.name != "source.org" && tc.name != "source.data" {
				t.Fatal("source content missing")
			}
		})
	}
}

func TestRT011_1_ServedOrgAndMarkdown(t *testing.T) {
	host := NativeHost()
	execute := host.Execute
	var native, pandoc atomic.Int32
	host.Execute = func(ctx context.Context, cmd Command) ([]byte, error) {
		if len(cmd.Args) > 0 && cmd.Args[0] == "--internal-org-convert" {
			native.Add(1)
		}
		if filepath.Base(cmd.Path) == "pandoc" {
			for _, arg := range cmd.Args {
				if arg == "--from=org" {
					return nil, fmt.Errorf("Org conversion still invoked Pandoc")
				}
				if strings.HasPrefix(arg, "--from=markdown") {
					pandoc.Add(1)
				}
			}
		}
		return execute(ctx, cmd)
	}
	s := startTestService(t, host)
	source(t, s.root, "next.org", "* Linked native heading\n")
	path := source(t, s.root, "index.org", "* Index\n[[file:next.org][Next]]\n")
	endpoint := annotationRegistrationURL(t, s, path, "Reviewer")
	status, _, data := responseAsset(t, s, "GET", strings.Replace(endpoint, "/_annotations/v2/", "/", 1))
	if status != 200 {
		t.Fatalf("Org page status %d", status)
	}
	doc := parseHTTPDocument(t, data)
	linked := ""
	for _, a := range nodes(documentNode(t, doc, "hp-document"), "a") {
		if textOf(a) == "Next" {
			linked = attr(a, "href")
		}
	}
	if linked == "" {
		t.Fatal("linked Org destination absent")
	}
	if strings.HasPrefix(linked, "/") {
		linked = s.origin + linked
	}
	status, _, data = responseAsset(t, s, "GET", linked)
	if status != 200 || !strings.Contains(string(data), "Linked native heading") {
		t.Fatal("linked native document unavailable", status)
	}
	md := source(t, s.root, "notes.md", "# Markdown\n\nText[^a].\n\n[^a]: Note.\n")
	endpoint = annotationRegistrationURL(t, s, md, "Reviewer")
	status, _, data = responseAsset(t, s, "GET", strings.Replace(endpoint, "/_annotations/v2/", "/", 1))
	if status != 200 || !strings.Contains(string(data), "Note.") || native.Load() == 0 || pandoc.Load() == 0 {
		t.Fatalf("mixed routing status=%d native=%d pandoc=%d", status, native.Load(), pandoc.Load())
	}
}

// RT011.2 - Explicit converter-delta assertions, independent of literal IDs.
func TestRT011_2_FootnoteHTMLContract(t *testing.T) {
	root := t.TempDir()
	path := source(t, root, "notes.org", "A sentence.[fn:note] Again.[fn:note]\n\n[fn:note] A comment.\n\nSecond paragraph with /emphasis/.\n")
	r := run(t, root, nil, path)
	success(t, r, 1)
	main := documentNode(t, r.pages[0], "hp-document")
	ids := make(map[string]*html.Node)
	var refs, backs []*html.Node
	var section *html.Node
	for n := range main.Descendants() {
		if id := attr(n, "id"); id != "" {
			if ids[id] != nil {
				t.Fatalf("duplicate id %q", id)
			}
			ids[id] = n
		}
		if hasClass(n, "footnotes") {
			if section != nil || n.Data != "section" {
				t.Fatal("endnotes are not one section")
			}
			section = n
		}
		if hasClass(n, "footnote-ref") {
			refs = append(refs, n)
		}
		if hasClass(n, "footnote-back") {
			backs = append(backs, n)
		}
		if hasClass(n, "footnote-definition") || hasClass(n, "footnote-body") {
			t.Fatal("unnormalized go-org endnotes survived")
		}
	}
	if section == nil || len(refs) != 2 || len(backs) != 2 {
		t.Fatalf("section=%v refs=%d backlinks=%d", section != nil, len(refs), len(backs))
	}
	for _, ref := range refs {
		if ref.Data != "a" || attr(ref, "role") != "doc-noteref" || ref.Parent.Data == "sup" || len(nodes(ref, "sup")) != 1 {
			t.Fatal("reference nesting or semantics changed")
		}
		definition := ids[strings.TrimPrefix(attr(ref, "href"), "#")]
		if definition == nil || definition.Data != "li" || definition.Parent.Data != "ol" || definition.Parent.Parent != section {
			t.Fatal("reference does not resolve to an ordered endnote")
		}
		if len(nodes(definition, "p")) != 2 || !strings.Contains(textOf(definition), "Second paragraph") || len(nodes(definition, "em")) != 1 {
			t.Fatal("multi-paragraph note lost markup or content")
		}
		found := false
		for _, back := range backs {
			if attr(back, "href") == "#"+attr(ref, "id") && attr(back, "role") == "doc-backlink" {
				found = true
			}
		}
		if !found {
			t.Fatal("reference lacks its backlink")
		}
	}
}

func TestRT011_3_AdditionalHighlightLanguages(t *testing.T) {
	root := t.TempDir()
	for _, tc := range []struct{ language, code string }{
		{"javascript", "const text = 'Taḋg <&>';\n"},
		{"python", "def greet():\n\treturn 'Taḋg'\n"},
		{"java", "class Example { int count = 42; }\n"},
	} {
		t.Run(tc.language, func(t *testing.T) {
			p := source(t, root, tc.language+".org", "#+BEGIN_SRC "+tc.language+"\n"+tc.code+"#+END_SRC\n")
			r := run(t, root, nil, p)
			success(t, r, 1)
			blocks := nodes(documentNode(t, r.pages[0], "hp-document"), "pre")
			if len(blocks) != 1 || textOf(blocks[0]) != tc.code || len(nodes(blocks[0], "span")) == 0 {
				t.Fatal("highlighting or literal payload lost")
			}
		})
	}
}

func TestRT011_6_NativeOrgCannotFetchOrExecute(t *testing.T) {
	var calls atomic.Int32
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); w.WriteHeader(204) }))
	defer remote.Close()
	root := t.TempDir()
	secret := source(t, root, "secret.org", "NEVER_INCLUDE_THIS_CANARY")
	body := "#+INCLUDE: \"" + secret + "\"\n#+SETUPFILE: " + remote.URL + "/setup\n* Content\n[[" + remote.URL + "/image.png]]\n#+BEGIN_EXPORT html\n<script>SECRET_SCRIPT()</script><form action=\"" + remote.URL + "\"><input value=\"secret\"></form>\n<a href=\"javascript:SECRET_SCRIPT()\">unsafe</a>\n#+END_EXPORT\n"
	path := source(t, root, "security.org", body)
	r := run(t, root, nil, path)
	success(t, r, 1)
	main := documentNode(t, r.pages[0], "hp-document")
	if calls.Load() != 0 || strings.Contains(textOf(main), "NEVER_INCLUDE_THIS_CANARY") || len(nodes(main, "script")) != 0 || len(nodes(main, "form")) != 0 || len(nodes(main, "input")) != 0 {
		t.Fatal("converter crossed passive-content boundary")
	}
	for _, a := range nodes(main, "a") {
		if strings.HasPrefix(attr(a, "href"), "javascript:") {
			t.Fatal("active URL survived")
		}
	}
	if strings.Contains(r.stderr, "SECRET_SCRIPT") || strings.Contains(r.stderr, "NEVER_INCLUDE_THIS_CANARY") {
		t.Fatal("source content leaked into diagnostics")
	}
}

func TestRT011_7_NativeWorkerFailureAndRecovery(t *testing.T) {
	host := NativeHost()
	execute := host.Execute
	var fail atomic.Bool
	fail.Store(true)
	host.Execute = func(ctx context.Context, cmd Command) ([]byte, error) {
		if len(cmd.Args) > 0 && cmd.Args[0] == "--internal-org-convert" && fail.Load() {
			return nil, fmt.Errorf("forced native worker failure")
		}
		return execute(ctx, cmd)
	}
	s := startTestService(t, host)
	path := source(t, s.root, "recovery.org", "* Recovery\n\nUnchanged paragraph.\n")
	endpoint := annotationRegistrationURL(t, s, path, "Reviewer")
	pageURL := strings.Replace(endpoint, "/_annotations/v2/", "/", 1)
	status, _, _ := responseAsset(t, s, "GET", pageURL)
	if status != 422 {
		t.Fatalf("failed worker status=%d, want 422", status)
	}
	fail.Store(false)
	status, _, data := responseAsset(t, s, "GET", pageURL)
	if status != 200 || !strings.Contains(string(data), "Unchanged paragraph.") {
		t.Fatal("service failed to recover after native worker error", status)
	}
}

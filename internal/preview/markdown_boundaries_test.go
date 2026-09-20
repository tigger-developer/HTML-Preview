// ABOUTME: Verifies native Markdown worker failure and optional-reader isolation.
// ABOUTME: Exercises passive content and service recovery through HTTP outcomes.
package preview

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRT014_6_NativeServiceWithoutPandoc(t *testing.T) {
	host := NativeHost()
	look := host.LookPath
	host.LookPath = func(name string) (string, error) {
		if name == "pandoc" {
			return "", os.ErrNotExist
		}
		return look(name)
	}
	s := startTestService(t, host)
	for _, tc := range []struct{ name, body string }{{"first.md", "# Native Markdown\n"}, {"second.org", "* Native Org\n"}} {
		url := s.register(t, tc.name, tc.body)
		status, _, body := responseAsset(t, s, "GET", url)
		if status != 200 || !strings.Contains(string(body), "Native ") {
			t.Fatal("native preview unavailable without Pandoc", status)
		}
		// A failed optional request must leave this running native service usable.
		source(t, s.root, "optional.docx", "unreadable office fixture")
		optionalURL := url[:strings.LastIndex(url, "/")+1] + "optional.docx"
		status, _, body = responseAsset(t, s, "GET", optionalURL)
		if status != 415 || !strings.Contains(string(body), "Pandoc") {
			t.Fatal("missing optional dependency has no actionable diagnostic", status)
		}
		status, _, body = responseAsset(t, s, "GET", url)
		if status != 200 || !strings.Contains(string(body), "Native ") {
			t.Fatal("failed optional request poisoned native service", status)
		}
	}
}

func TestRT014_6_FailedCatalogueIsNotAnEmptySuccess(t *testing.T) {
	host := NativeHost()
	execute := host.Execute
	host.Execute = func(ctx context.Context, cmd Command) ([]byte, error) {
		if filepath.Base(cmd.Path) == "pandoc" && len(cmd.Args) > 0 && cmd.Args[0] == "--list-input-formats" {
			return nil, errors.New("catalogue unavailable")
		}
		return execute(ctx, cmd)
	}
	var out, diagnostics strings.Builder
	status := Main([]string{"--list-input-formats"}, nil, &out, &diagnostics, "test", "test", host)
	if status == 0 || !strings.Contains(diagnostics.String(), "catalogue unavailable") {
		t.Fatal("failed catalogue presented as success", status, diagnostics.String())
	}
}

func TestRT014_3_OptionalMarkdownReadersOmitHTML(t *testing.T) {
	root := t.TempDir()
	p := source(t, root, "optional.data", "<div>OMITTED_BLOCK_CONTENT</div>\n\nVisible paragraph.\n\n`<i>example</i>`\n")
	for _, reader := range []string{"gfm", "commonmark", "markdown_strict"} {
		t.Run(reader, func(t *testing.T) {
			r := run(t, root, nil, "--from="+reader, p)
			success(t, r, 1)
			main := documentNode(t, r.pages[0], "hp-document")
			if strings.Contains(textOf(main), "OMITTED_BLOCK_CONTENT") || !strings.Contains(textOf(main), "<i>example</i>") {
				t.Fatal("optional reader omission differs")
			}
			if strings.Count(r.stderr, "Embedded HTML omitted:") != 1 || strings.Count(textOf(r.pages[0]), omittedHTMLWarning) != 1 {
				t.Fatal("optional reader notices missing")
			}
		})
	}
}

func TestRT014_7_MarkdownWorkerFailureAndRecovery(t *testing.T) {
	for _, fault := range []string{"failure", "malformed-protocol"} {
		t.Run(fault, func(t *testing.T) {
			host := NativeHost()
			execute := host.Execute
			var fail atomic.Bool
			fail.Store(true)
			host.Execute = func(ctx context.Context, cmd Command) ([]byte, error) {
				if len(cmd.Args) > 0 && cmd.Args[0] == "--internal-markdown-convert" && fail.Load() {
					if fault == "failure" {
						return nil, fmt.Errorf("forced Markdown conversion failure")
					}
					cmd.Input = []byte(`{"version":1,"version":2}`)
				}
				return execute(ctx, cmd)
			}
			s := startTestService(t, host)
			p := source(t, s.root, "recovery.md", "# Recovery\n\nUnchanged source.\n")
			endpoint := annotationRegistrationURL(t, s, p, "Reviewer")
			url := strings.Replace(endpoint, "/_annotations/v2/", "/", 1)
			status, _, data := responseAsset(t, s, "GET", url)
			if status != 422 || strings.Contains(string(data), "Unchanged source.") {
				t.Fatalf("worker failure published content or incorrect status: %d", status)
			}
			if readMarkdownSource(t, p) != "# Recovery\n\nUnchanged source.\n" {
				t.Fatal("conversion changed source")
			}
			fail.Store(false)
			status, _, data = responseAsset(t, s, "GET", url)
			if status != 200 || !strings.Contains(string(data), "Unchanged source.") {
				t.Fatal("worker slot did not recover", status)
			}
		})
	}
}

func TestRT014_7_MarkdownPassiveContent(t *testing.T) {
	var calls atomic.Int32
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); w.WriteHeader(204) }))
	defer remote.Close()
	root := t.TempDir()
	body := "---\ntitle: '<script>METADATA_CANARY</script>'\n---\n\n# Safe\n\n![remote](" + remote.URL + "/image.png)\n\n[unsafe](javascript:alert(1))\n\n<div data-hp-annotation-block=\"forged\">UNTRUSTED_CONTENT</div>\n\n<script>UNTRUSTED_SCRIPT()</script>\n\nSafe paragraph.\n"
	p := source(t, root, "passive.md", body)
	r := run(t, root, nil, p)
	success(t, r, 1)
	main := documentNode(t, r.pages[0], "hp-document")
	if calls.Load() != 0 || len(nodes(main, "script")) != 0 || strings.Contains(textOf(main), "UNTRUSTED_CONTENT") {
		t.Fatal("passive boundary violated")
	}
	for n := range main.Descendants() {
		if attr(n, "data-hp-annotation-block") == "forged" || strings.HasPrefix(attr(n, "href"), "javascript:") {
			t.Fatal("authored control or unsafe URL survived")
		}
	}
	if strings.Contains(r.stderr, "UNTRUSTED_SCRIPT") || strings.Contains(r.stderr, "METADATA_CANARY") {
		t.Fatal("source leaked to diagnostic")
	}
	if readMarkdownSource(t, p) != body {
		t.Fatal("reading rewrote source")
	}
}

func TestRT014_7_StartedMarkdownWorkerBounds(t *testing.T) {
	testStartedNativeWorkerBounds(t, "md")
}

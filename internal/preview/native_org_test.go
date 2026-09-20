// ABOUTME: Exercises native Org routing and normalized HTML through public preview boundaries.
// ABOUTME: Keeps independent semantic oracles for converter differences and passive content.
package preview

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/sys/unix"
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
			text := strings.Join(strings.Fields(textOf(documentNode(t, r.pages[0], "hp-document"))), " ")
			want := strings.Join(strings.Fields(strings.TrimPrefix(tc.text, "* ")), " ")
			if !strings.Contains(text, want) {
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
	// W014 - Native Markdown supersedes only the previous Pandoc-call expectation.
	if status != 200 || !strings.Contains(string(data), "Note.") || native.Load() == 0 || pandoc.Load() != 0 {
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

// Test-only child acknowledgement proves that cancellation occurs after Start.
func writeWorkerPID() {
	if len(os.Args) == 3 {
		if err := os.WriteFile(os.Args[2], []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
			os.Exit(90)
		}
	}
}

func TestRT011_7_StartedNativeWorkerBounds(t *testing.T) {
	testStartedNativeWorkerBounds(t, "org")
}

func testStartedNativeWorkerBounds(t *testing.T, format string) {
	worker := "--internal-org-convert"
	body := "* Worker\n\nUnchanged source.\n"
	if format == "md" {
		worker = "--internal-markdown-convert"
		body = "# Worker\n\nUnchanged source.\n"
	}
	for _, tc := range []struct {
		mode   string
		status int
	}{{"blocker", 504}, {"flood", 413}} {
		t.Run(tc.mode, func(t *testing.T) {
			host := NativeHost()
			actual, temp := host.Execute, host.Temp
			var fail atomic.Bool
			fail.Store(true)
			pidFile := filepath.Join(t.TempDir(), "child-pid")
			var mu sync.Mutex
			var dirs []string
			host.Temp = func() (string, error) {
				path, err := temp()
				mu.Lock()
				dirs = append(dirs, path)
				mu.Unlock()
				return path, err
			}
			host.Execute = func(ctx context.Context, cmd Command) ([]byte, error) {
				if len(cmd.Args) > 0 && cmd.Args[0] == worker && fail.Load() {
					bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
					defer cancel()
					cmd.Args, cmd.Input, cmd.Limit = []string{"--process-" + tc.mode, pidFile}, nil, 4096
					return actual(bounded, cmd)
				}
				return actual(ctx, cmd)
			}
			s := startTestService(t, host)
			path := source(t, s.root, "bounded."+format, body)
			endpoint := annotationRegistrationURL(t, s, path, "Reviewer")
			pageURL := strings.Replace(endpoint, "/_annotations/v2/", "/", 1)
			status, _, data := responseAsset(t, s, "GET", pageURL)
			if status != tc.status || strings.Contains(string(data), "Unchanged source.") {
				t.Fatalf("failed worker published a page or wrong status: %d", status)
			}
			// #nosec G304 -- Test helper writes only this test-owned acknowledgement path.
			pidBytes, err := os.ReadFile(pidFile)
			if err != nil {
				t.Fatal("worker never started", err)
			}
			pid, err := strconv.Atoi(string(pidBytes))
			if err != nil || unix.Kill(pid, 0) != unix.ESRCH {
				t.Fatal("worker was not reaped", err)
			}
			mu.Lock()
			for _, dir := range dirs {
				if _, err := os.Stat(dir); !os.IsNotExist(err) {
					t.Errorf("failed conversion retained scratch directory %s", dir)
				}
			}
			mu.Unlock()
			fail.Store(false)
			status, _, data = responseAsset(t, s, "GET", pageURL)
			if status != 200 || !strings.Contains(string(data), "Unchanged source.") {
				t.Fatal("worker slot not reusable", status)
			}
		})
	}
}

func TestRT011_5_RepeatedMultiParagraphFootnoteLifecycle(t *testing.T) {
	s := startTestService(t, NativeHost())
	prefix := "A sentence.[fn:note] Again.[fn:note]\n\n"
	definition := "[fn:note] A comment.\n\nSecond paragraph with /emphasis/.\n\n\n"
	tail := "* Next\n\nUnrelated bytes: Taḋg.\n"
	path := source(t, s.root, "lifecycle.org", prefix+definition+tail)
	endpoint := annotationRegistrationURL(t, s, path, "Reviewer")
	for index, action := range []string{"edit", "delete"} {
		status, state := annotationJSON(t, s, "GET", endpoint, nil, nil)
		notes, ok := state["footnotes"].([]any)
		if status != 200 || !ok || len(notes) != 1 {
			t.Fatal("note identification", status, state)
		}
		note := notes[0].(map[string]any)
		if note["label"] != "note" || !strings.Contains(note["text"].(string), "Second paragraph with /emphasis/.") {
			t.Fatal("note content lost", note)
		}
		pageStatus, _, pageData := responseAsset(t, s, "GET", strings.Replace(endpoint, "/_annotations/v2/", "/", 1))
		if pageStatus != 200 {
			t.Fatal("page unavailable", pageStatus)
		}
		mapped := 0
		for _, li := range nodes(parseHTTPDocument(t, pageData), "li") {
			if attr(li, "data-hp-footnote-label") == "note" {
				mapped++
			}
		}
		if mapped != 2 {
			t.Fatal("repeated note association", mapped)
		}
		text := strings.Replace(note["text"].(string), "A comment.", "Updated Taḋg.", 1) + "\n"
		if action == "delete" {
			text = ""
		}
		headers := map[string]string{"Origin": s.origin, "X-HTMLPreview-Annotation-Token": state["write_token"].(string), "X-HTMLPreview-Composer-Token": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}
		request := map[string]any{"operation_id": fmt.Sprintf("10000000-0000-4000-8000-%012d", index+1), "annotation_id": "20000000-0000-4000-8000-000000000001", "composer_id": "30000000-0000-4000-8000-000000000001", "sequence": index + 1, "revision": state["revision"], "source_revision": state["source_revision"], "body_revision": state["body_revision"], "action": action, "label": "note", "target": map[string]any{"type": "footnote", "exact": note["revision"], "run": "embedded"}, "text": text}
		status, result := annotationJSON(t, s, "POST", endpoint, request, headers)
		if status != 201 {
			t.Fatal(action, status, result)
		}
		// #nosec G304 -- Synthetic source is allocated in the isolated service root.
		data, err := os.ReadFile(path)
		want := prefix + strings.Replace(definition, "A comment.", "Updated Taḋg.", 1) + tail
		if action == "delete" {
			want = strings.ReplaceAll(prefix, "[fn:note]", "") + "\n\n" + tail
		}
		if err != nil || string(data) != want {
			t.Fatalf("%s changed unrelated bytes: %v\n%s", action, err, data)
		}
	}
	status, state := annotationJSON(t, s, "GET", endpoint, nil, nil)
	if status != 200 || len(state["footnotes"].([]any)) != 0 {
		t.Fatal("deleted note still listed", status, state)
	}
}

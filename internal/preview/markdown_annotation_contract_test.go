// ABOUTME: Checks Markdown annotation creation from browser-selectable served blocks.
// ABOUTME: Verifies exact source boundaries through successive authorized HTTP writes.
package preview

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

type markdownBlockCase struct{ name, source, click, boundary string }

func markdownBlockCases() []markdownBlockCase {
	return []markdownBlockCase{
		{"paragraph", "First sentence with *emphasized text*, `inline code` and [a local link](other.md).\nSecond sentence ends the wrapped paragraph.\n", "emphasized text", "Second sentence ends the wrapped paragraph."},
		{"tight-first", "- First tight item.\n- Second tight item.\n", "First tight item", "- First tight item."},
		{"tight-second", "- First tight item.\n- Second tight item.\n", "Second tight item", "- Second tight item."},
		{"nested", "- Parent item\n  - Nested child with *child emphasis*.\n", "child emphasis", "  - Nested child with *child emphasis*."},
		{"loose", "- Loose item first paragraph.\n\n  Loose item second paragraph.\n", "Loose item second", "  Loose item second paragraph."},
		{"task", "- [x] Checked task.\n", "Checked task", "- [x] Checked task."},
		{"definition", "Term\n: Definition body with *definition emphasis*.\n", "definition emphasis", ": Definition body with *definition emphasis*."},
		{"heading", "## Annotation heading\n", "Annotation heading", "## Annotation heading\n\nAnnotations: "},
		{"table", "| Column | Value |\n|--------|-------|\n| Cell | Other |\n", "Cell", "| Cell | Other |\n\nAnnotations: "},
		{"quote", "> Quoted paragraph.\n>\n> Another quoted paragraph.\n", "Quoted paragraph", "> Another quoted paragraph.\n\nAnnotations: "},
		{"code", "```go\nprintln(\"source block\")\n```\n", "source block", "```\n\nAnnotations: "},
	}
}

func TestRT014_4_ClickableMarkdownCreation(t *testing.T) {
	s := startTestService(t, NativeHost())
	cases := markdownBlockCases()
	for _, mixed := range []bool{false, true} {
		for _, tc := range cases {
			t.Run(fmt.Sprintf("mixed=%v/%s", mixed, tc.name), func(t *testing.T) {
				body := tc.source
				if mixed {
					var blocks []string
					for _, c := range cases {
						if (tc.name == "tight-first" && c.name == "tight-second") || (tc.name != "tight-first" && c.name == "tight-first") {
							continue
						}
						blocks = append(blocks, c.source)
					}
					body = strings.Join(blocks, "\n")
				}
				body += "\nA subsequent body paragraph remains writable.\n"
				path := source(t, s.root, fmt.Sprintf("%v-%s.md", mixed, tc.name), body)
				endpoint := annotationRegistrationURL(t, s, path, "Reviewer")
				pageURL := strings.Replace(endpoint, "/_annotations/v2/", "/", 1)
				main, bootstrap := markdownBrowserPage(t, s, pageURL)
				assertClickableAnnotationText(t, main, tc.click)
				// The actual advertised URL is used for refresh, not a guessed equivalent.
				main, bootstrap = markdownBrowserPage(t, s, bootstrap["page_url"].(string))
				block := assertClickableAnnotationText(t, main, tc.click)
				markdownCreateAtBlock(t, s, bootstrap, block, "reviewer-001", 1)
				data := readMarkdownSource(t, path)
				if !strings.Contains(data, tc.boundary+"[^reviewer-001]") {
					t.Fatalf("annotation not at full block boundary %q: %s", tc.boundary, data)
				}
				main, bootstrap = markdownBrowserPage(t, s, bootstrap["page_url"].(string))
				assertClickableAnnotationText(t, main, tc.click)
				next := assertClickableAnnotationText(t, main, "A subsequent body paragraph")
				markdownCreateAtBlock(t, s, bootstrap, next, "reviewer-002", 2)
				data = readMarkdownSource(t, path)
				if !strings.Contains(data, "A subsequent body paragraph remains writable.[^reviewer-002]") {
					t.Fatal("subsequent insertion moved away from paragraph end")
				}
				// Check the unchanged first source line; exact unrelated-byte preservation
				// for edits is covered by TestRT014_5_RepeatedMarkdownNoteLifecycle.
				if !strings.HasPrefix(data, strings.Split(body, "\n")[0]) {
					t.Fatal("source prefix changed")
				}
			})
		}
	}
}

func markdownBrowserPage(t *testing.T, s *runningTestService, pageURL string) (*html.Node, map[string]any) {
	t.Helper()
	status, _, data := responseAsset(t, s, "GET", pageURL)
	if status != 200 {
		t.Fatalf("page status=%d", status)
	}
	page := parseHTTPDocument(t, data)
	var bootstrap map[string]any
	if err := json.Unmarshal([]byte(textOf(documentNode(t, page, "hp-annotation-data"))), &bootstrap); err != nil {
		t.Fatal(err)
	}
	status, state := annotationJSON(t, s, "GET", bootstrap["endpoint"].(string), nil, nil)
	if status != 200 || state["writable"] != true {
		t.Fatal("annotation endpoint unavailable", status, state["reason"])
	}
	for _, key := range []string{"revision", "source_revision", "body_revision"} {
		if bootstrap[key] == "" || bootstrap[key] != state[key] {
			t.Fatalf("browser revision differs for %s", key)
		}
	}
	bootstrap["write_token"] = state["write_token"]
	return documentNode(t, page, "hp-document"), bootstrap
}

func markdownCreateAtBlock(t *testing.T, s *runningTestService, state map[string]any, block, label string, sequence int) {
	t.Helper()
	headers := map[string]string{"Origin": s.origin, "X-HTMLPreview-Annotation-Token": state["write_token"].(string), "X-HTMLPreview-Composer-Token": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}
	request := map[string]any{"operation_id": fmt.Sprintf("10000000-0000-4000-8000-%012d", sequence), "annotation_id": fmt.Sprintf("20000000-0000-4000-8000-%012d", sequence), "composer_id": fmt.Sprintf("30000000-0000-4000-8000-%012d", sequence), "sequence": 1, "revision": state["revision"], "source_revision": state["source_revision"], "body_revision": state["body_revision"], "action": "upsert", "label": label, "target": map[string]any{"type": "point", "block_id": block}, "text": "Synthetic review note.\n\nSecond note paragraph."}
	status, result := annotationJSON(t, s, "POST", state["endpoint"].(string), request, headers)
	if status != 201 {
		t.Fatalf("creation from browser-selectable block: %d %v", status, result)
	}
}

func readMarkdownSource(t *testing.T, path string) string {
	t.Helper()
	// #nosec G304 -- Paths originate in this test's synthetic service root.
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRT014_5_RepeatedMarkdownNoteLifecycle(t *testing.T) {
	s := startTestService(t, NativeHost())
	original := "A sentence.[^note] Again.[^note]\n\n> Quote reference.[^note]\n\n[^note]: Original *note*.\n\n    Second paragraph.\n\n# Next\n\nUnrelated Taḋg bytes.\n"
	path := source(t, s.root, "lifecycle.md", original)
	endpoint := annotationRegistrationURL(t, s, path, "Reviewer")
	for i, action := range []string{"edit", "delete"} {
		status, state := annotationJSON(t, s, "GET", endpoint, nil, nil)
		if status != 200 {
			t.Fatal(status, state)
		}
		notes, ok := state["footnotes"].([]any)
		if !ok || len(notes) != 1 {
			t.Fatal("expected exactly one stored footnote", state)
		}
		note := notes[0].(map[string]any)
		if note["label"] != "note" || !strings.Contains(note["text"].(string), "Second paragraph.") {
			t.Fatal("lost footnote association or paragraphs", note)
		}
		pageStatus, _, pageData := responseAsset(t, s, "GET", strings.Replace(endpoint, "/_annotations/v2/", "/", 1))
		if pageStatus != 200 {
			t.Fatal(pageStatus)
		}
		main := documentNode(t, parseHTTPDocument(t, pageData), "hp-document")
		refs := 0
		for _, a := range nodes(main, "a") {
			if hasClass(a, "footnote-ref") {
				refs++
			}
		}
		if refs != 3 || strings.Contains(textOf(main), "Unplaced annotation") {
			t.Fatal("repeated or quote reference lost", refs)
		}
		value := strings.Replace(note["text"].(string), "Original", "Updated", 1)
		if action == "delete" {
			value = ""
		}
		request := map[string]any{"operation_id": fmt.Sprintf("10000000-0000-4000-8000-%012d", i+1), "annotation_id": "20000000-0000-4000-8000-000000000001", "composer_id": "30000000-0000-4000-8000-000000000001", "sequence": i + 1, "revision": state["revision"], "source_revision": state["source_revision"], "body_revision": state["body_revision"], "action": action, "label": "note", "target": map[string]any{"type": "footnote", "exact": note["revision"], "run": "embedded"}, "text": value}
		headers := map[string]string{"Origin": s.origin, "X-HTMLPreview-Annotation-Token": state["write_token"].(string), "X-HTMLPreview-Composer-Token": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}
		status, result := annotationJSON(t, s, "POST", endpoint, request, headers)
		if status != 201 {
			t.Fatal(action, status, result)
		}
		if action == "edit" {
			want := strings.Replace(original, "Original", "Updated", 1)
			if readMarkdownSource(t, path) != want {
				t.Fatal("edit rewrote unrelated bytes")
			}
			status, result = annotationJSON(t, s, "POST", endpoint, request, headers)
			if status != 200 {
				t.Fatal("idempotent retry", status, result)
			}
			request["text"] = "Stale revision replacement"
			status, _ = annotationJSON(t, s, "POST", endpoint, request, headers)
			if status != 409 {
				t.Fatal("stale revision accepted", status)
			}
			if readMarkdownSource(t, path) != want {
				t.Fatal("conflict changed source")
			}
		} else {
			got := readMarkdownSource(t, path)
			if strings.Contains(got, "[^note]") || !strings.Contains(got, "A sentence. Again.\n\n> Quote reference.\n") || !strings.HasSuffix(got, "# Next\n\nUnrelated Taḋg bytes.\n") {
				t.Fatal("delete removed wrong source bytes")
			}
		}
	}
	status, state := annotationJSON(t, s, "GET", endpoint, nil, nil)
	if status != 200 || len(state["footnotes"].([]any)) != 0 {
		t.Fatal("deleted footnote still listed")
	}
}

func TestRT014_4_MarkdownRejectsUnverifiedTargets(t *testing.T) {
	s := startTestService(t, NativeHost())
	body := "---\ntitle: Metadata only\n---\n\n<div data-hp-annotation-block=\"forged\">Omitted content</div>\n\nA normal paragraph.[^existing]\n\n[^existing]: Endnote body.\n"
	path := source(t, s.root, "negative.md", body)
	endpoint := annotationRegistrationURL(t, s, path, "Reviewer")
	main, state := markdownBrowserPage(t, s, strings.Replace(endpoint, "/_annotations/v2/", "/", 1))
	for n := range main.Descendants() {
		if attr(n, "data-hp-annotation-block") != "" && (insideEndnotes(n) || strings.Contains(textOf(n), "Metadata only") || strings.Contains(textOf(n), "Omitted content")) {
			t.Fatal("excluded content offered as an annotation target")
		}
	}
	headers := map[string]string{"Origin": s.origin, "X-HTMLPreview-Annotation-Token": state["write_token"].(string), "X-HTMLPreview-Composer-Token": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}
	request := map[string]any{"operation_id": "10000000-0000-4000-8000-000000000001", "annotation_id": "20000000-0000-4000-8000-000000000001", "composer_id": "30000000-0000-4000-8000-000000000001", "sequence": 1, "revision": state["revision"], "source_revision": state["source_revision"], "body_revision": state["body_revision"], "action": "upsert", "label": "reviewer-001", "target": map[string]any{"type": "point", "block_id": "forged"}, "text": "Must not save."}
	status, _ := annotationJSON(t, s, "POST", endpoint, request, headers)
	if status != 409 || readMarkdownSource(t, path) != body {
		t.Fatal("forged target accepted or modified source", status)
	}
	block := assertClickableAnnotationText(t, main, "A normal paragraph")
	request["target"] = map[string]any{"type": "point", "block_id": block}
	changed := strings.Replace(body, "A normal paragraph", "An externally changed paragraph", 1)
	if err := os.WriteFile(path, []byte(changed), 0600); err != nil {
		t.Fatal(err)
	}
	status, _ = annotationJSON(t, s, "POST", endpoint, request, headers)
	if status != 409 || readMarkdownSource(t, path) != changed {
		t.Fatal("stale target accepted or external edits overwritten", status)
	}
}

func TestRT014_5_TwoNotesAtParagraphEnd(t *testing.T) {
	s := startTestService(t, NativeHost())
	path := source(t, s.root, "two.md", "An entire paragraph.\n\nAn unrelated paragraph.\n")
	endpoint := annotationRegistrationURL(t, s, path, "Reviewer")
	pageURL := strings.Replace(endpoint, "/_annotations/v2/", "/", 1)
	for i := 1; i <= 2; i++ {
		main, state := markdownBrowserPage(t, s, pageURL)
		markdownCreateAtBlock(t, s, state, assertClickableAnnotationText(t, main, "An entire paragraph"), fmt.Sprintf("reviewer-%03d", i), i)
	}
	saved := readMarkdownSource(t, path)
	if !strings.HasPrefix(saved, "An entire paragraph.[^reviewer-001][^reviewer-002]\n\nAn unrelated paragraph.\n") {
		t.Fatal("successive references not at exact paragraph end")
	}
	status, state := annotationJSON(t, s, "GET", endpoint, nil, nil)
	if status != 200 {
		t.Fatal(status)
	}
	notes := state["footnotes"].([]any)
	if len(notes) != 2 {
		t.Fatal("sidebar note count", len(notes))
	}
	for i, n := range notes {
		if n.(map[string]any)["label"] != fmt.Sprintf("reviewer-%03d", i+1) {
			t.Fatal("sidebar label mismatch")
		}
	}
}

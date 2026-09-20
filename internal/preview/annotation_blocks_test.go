// ABOUTME: Exercises source-bound annotation blocks through served HTML and HTTP writes.
// ABOUTME: Checks duplicate prose, native syntax boundaries and source preservation.
package preview

import (
	"bytes"
	"fmt"
	"golang.org/x/net/html"
	"os"
	"strings"
	"testing"
)

func TestHTTPBlockAnnotations(t *testing.T) {
	s := startTestService(t, NativeHost())
	for _, tc := range []struct{ name, ext, text, target, want string }{
		{"short-definition", "org", "- Source part :: I.\n", "I.", "- Source part :: I.[fn:reviewer-001]"},
		{"wrapped-page-number", "org", "- Wrapped prose on\n  p. 32 and more prose.\n", "Wrapped prose on p. 32 and more prose.", "p. 32 and more prose.[fn:reviewer-001]"},
		{"mixed-definition", "org", "- Named :: A description.\n- Ordinary item.\n", "A description.", "- Named :: A description.[fn:reviewer-001]\n- Ordinary item."},
		{"mixed-ordinary", "org", "- Named :: A description.\n- Ordinary item.\n", "Ordinary item.", "- Ordinary item.[fn:reviewer-001]"},
		{"task-checked", "org", "* Tasks\n# Group\n- [X] Checked item.\n", "✓ Checked item.", "- [X] Checked item.[fn:reviewer-001]"},
		{"task-unchecked", "org", "- [ ] Unchecked item.\n", "Unchecked item.", "- [ ] Unchecked item.[fn:reviewer-001]"},
		{"task-lowercase", "org", "+ [x] Checked item.\n", "✓ Checked item.", "+ [x] Checked item.[fn:reviewer-001]"},
		{"task-partial", "org", "- [/] Partial item.\n", "− Partial item.", "- [/] Partial item.[fn:reviewer-001]"},
		{"task-partial-dash", "org", "- [-] Partial item.\n", "− Partial item.", "- [-] Partial item.[fn:reviewer-001]"},
		{"task-checked", "md", "- [X] Checked item.\n", "✓ Checked item.", "- [X] Checked item.[^reviewer-001]"},
		{"task-unchecked", "md", "+ [ ] Unchecked item.\n", "Unchecked item.", "+ [ ] Unchecked item.[^reviewer-001]"},
		{"task-lowercase", "md", "* [x] Checked item.\n", "✓ Checked item.", "* [x] Checked item.[^reviewer-001]"},
		{"task-partial", "md", "1. [/] Partial item.\n", "− Partial item.", "1. [/] Partial item.[^reviewer-001]"},
		{"task-wrapped", "org", "- [X] Checked item\n  with a continuation.\n", "✓ Checked item with a continuation.", "with a continuation.[fn:reviewer-001]"},
		{"task-wrapped", "md", "- [x] Checked item\n  with a continuation.\n", "✓ Checked item with a continuation.", "with a continuation.[^reviewer-001]"},
		{"table", "org", "| Item | Value |\n|------+-------|\n| One | Two |\n", "Item Value One Two", "| One | Two |\n\nAnnotations: [fn:reviewer-001]"},
		{"table", "md", "| Item | Value |\n|------|-------|\n| One | Two |\n", "Item Value One Two", "| One | Two |\n\nAnnotations: [^reviewer-001]"},
		{"table-no-outer-pipes", "md", "Item | Value\n-----|------\nOne | Two\n", "Item Value One Two", "One | Two\n\nAnnotations: [^reviewer-001]"},
		{"quote", "org", "#+BEGIN_QUOTE\nQuoted text.\n#+END_QUOTE\n", "Quoted text.", "#+END_QUOTE\n\nAnnotations: [fn:reviewer-001]"},
		{"verse", "org", "#+BEGIN_VERSE\nOne line\nTwo lines\n#+END_VERSE\n", "One line Two lines", "#+END_VERSE\n\nAnnotations: [fn:reviewer-001]"},
		{"centre", "org", "#+BEGIN_CENTER\nCentred text.\n#+END_CENTER\n", "Centred text.", "#+END_CENTER\n\nAnnotations: [fn:reviewer-001]"},
		{"custom", "org", "#+BEGIN_SPECIAL\nSpecial text.\n#+END_SPECIAL\n", "Special text.", "#+END_SPECIAL\n\nAnnotations: [fn:reviewer-001]"},
		{"custom-lowercase", "org", "#+begin_special\nSpecial text.\n#+end_special\n", "Special text.", "#+end_special\n\nAnnotations: [fn:reviewer-001]"},
		{"nested-block", "org", "#+BEGIN_QUOTE\nOutside.\n\n#+BEGIN_QUOTE\nInside.\n#+END_QUOTE\n#+END_QUOTE\n", "Outside. Inside.", "#+END_QUOTE\n#+END_QUOTE\n\nAnnotations: [fn:reviewer-001]"},
		{"wrapped", "org", "First sentence.\nSecond sentence continues\nto the paragraph end.\n", "First sentence. Second sentence continues to the paragraph end.", "to the paragraph end.[fn:reviewer-001]"},
		{"wrapped", "md", "First sentence.\nSecond sentence continues\nto the paragraph end.\n", "First sentence. Second sentence continues to the paragraph end.", "to the paragraph end.[^reviewer-001]"},
		{"duplicates", "org", strings.Repeat("Same paragraph.\n\n", 40) + "=P= means the numbered /paragraph/.\n", "P means the numbered paragraph.", "/paragraph/.[fn:reviewer-001]"},
		{"typography", "org", "A range of 129--168 in /emphasis/.\n", "A range of 129–168 in emphasis.", "129--168 in /emphasis/.[fn:reviewer-001]"},
		{"inline", "md", "A *word* and `code` and [link](missing.md).\n", "A word and code and link.", "[link](missing.md).[^reviewer-001]"},
		{"heading", "org", "* TODO Heading /text/ :tag:\n", "TODO Heading text tag", "Heading /text/[fn:reviewer-001] :tag:"},
		{"heading", "md", "## Heading *text*\n", "Heading text", "Heading *text*\n\nAnnotations: [^reviewer-001]"},
		{"nested", "org", "- Parent\n  - Child /text/.\n- Sibling\n", "Child text.", "Child /text/.[fn:reviewer-001]"},
		{"nested", "md", "- Parent\n  - Child *text*.\n- Sibling\n", "Child text.", "Child *text*.[^reviewer-001]"},
		{"code", "org", "#+BEGIN_SRC go\nfmt.Println(1)\n#+END_SRC\n", "fmt.Println(1)", "#+END_SRC\n\nAnnotations: [fn:reviewer-001]"},
		{"code", "md", "```go\nfmt.Println(1)\n```\n", "fmt.Println(1)", "```\n\nAnnotations: [^reviewer-001]"},
	} {
		t.Run(tc.name+"-"+tc.ext, func(t *testing.T) {
			path := source(t, s.root, tc.name+"."+tc.ext, tc.text)
			endpoint := annotationRegistrationURL(t, s, path, "Reviewer")
			pageURL := strings.Replace(endpoint, "/_annotations/v2/", "/", 1)
			status, data, err := testHTTPBody(s.client, pageURL)
			if err != nil || status != 200 {
				t.Fatal(status, err)
			}
			dom, err := html.Parse(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			block := ""
			for n := range dom.Descendants() {
				if id := attribute(n, "data-hp-annotation-block"); id != "" && strings.Join(strings.Fields(contentText(n)), " ") == tc.target {
					block = id
					break
				}
			}
			if block == "" {
				t.Fatalf("no eligible block for %q", tc.target)
			}
			status, state := annotationJSON(t, s, "GET", endpoint, nil, nil)
			if status != 200 {
				t.Fatal(status, state)
			}
			headers := map[string]string{"Origin": s.origin, "X-HTMLPreview-Annotation-Token": state["write_token"].(string), "X-HTMLPreview-Composer-Token": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}
			request := map[string]any{"operation_id": "10000000-0000-4000-8000-000000000001", "annotation_id": "20000000-0000-4000-8000-000000000001", "composer_id": "30000000-0000-4000-8000-000000000001", "sequence": 1, "revision": state["revision"], "source_revision": state["source_revision"], "body_revision": state["body_revision"], "action": "upsert", "label": "reviewer-001", "target": map[string]any{"type": "point", "block_id": block}, "text": "Review note."}
			status, result := annotationJSON(t, s, "POST", endpoint, request, headers)
			if status != 201 {
				t.Fatalf("save %d %#v", status, result)
			}
			// #nosec G304 -- Source path is created inside this test service's temporary root.
			saved, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(saved), tc.want) {
				t.Fatalf("wrong placement: %s", saved)
			}
		})
	}
}

func TestHTTPBlockRejectsUnverifiedTargets(t *testing.T) {
	s := startTestService(t, NativeHost())
	path := source(t, s.root, "zones.org", "* Heading\n:PROPERTIES:\n:CUSTOM_ID: heading\n:END:\n\n| A | B |\n|---+---|\n| 1 | 2 |\n\nOrdinary paragraph.\n")
	endpoint := annotationRegistrationURL(t, s, path, "Reviewer")
	status, data, err := testHTTPBody(s.client, strings.Replace(endpoint, "/_annotations/v2/", "/", 1))
	if err != nil || status != 200 {
		t.Fatal(status, err)
	}
	dom, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for n := range dom.Descendants() {
		if attribute(n, "data-hp-annotation-block") == "" {
			continue
		}
		count++
		if n.Data == "td" || n.Data == "th" || insideEndnotes(n) {
			t.Fatal("unsupported content offered as a target")
		}
	}
	if count != 3 {
		t.Fatalf("want heading, table and paragraph only, got %d", count)
	}
	if strings.Contains(string(data), "HPBLOCK") {
		t.Fatal("temporary markers leaked into output")
	}
	status, state := annotationJSON(t, s, "GET", endpoint, nil, nil)
	if status != 200 {
		t.Fatal(status)
	}
	headers := map[string]string{"Origin": s.origin, "X-HTMLPreview-Annotation-Token": state["write_token"].(string), "X-HTMLPreview-Composer-Token": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}
	request := map[string]any{"operation_id": "10000000-0000-4000-8000-000000000001", "annotation_id": "20000000-0000-4000-8000-000000000001", "composer_id": "30000000-0000-4000-8000-000000000001", "sequence": 1, "revision": state["revision"], "source_revision": state["source_revision"], "body_revision": state["body_revision"], "action": "upsert", "label": "reviewer-001", "target": map[string]any{"type": "point", "block_id": "forged"}, "text": "Must not save."}
	// #nosec G304 -- Source path is created inside this test service's temporary root.
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	status, _ = annotationJSON(t, s, "POST", endpoint, request, headers)
	if status != 409 {
		t.Fatalf("forged block status %d", status)
	}
	// #nosec G304 -- Source path is created inside this test service's temporary root.
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("rejected write modified source")
	}
}

func TestHTTPRepeatedBlockNotesAndHeadingAnchor(t *testing.T) {
	s := startTestService(t, NativeHost())
	for _, tc := range []struct{ name, ext, source, tag string }{
		{"heading", "md", "## Heading *text*\n", "h2"},
		{"code", "md", "```go\nfmt.Println(1)\n```\n", "pre"},
		{"table", "md", "| A | B |\n|---|---|\n| 1 | 2 |\n", "table"},
		{"table-org", "org", "| A | B |\n|---+---|\n| 1 | 2 |\n", "table"},
		{"quote", "org", "#+BEGIN_QUOTE\nQuoted text.\n#+END_QUOTE\n", "blockquote"},
		{"task-org", "org", "- [X] Checked item.\n", "li"},
		{"task-md", "md", "- [X] Checked item.\n", "li"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := source(t, s.root, tc.name+"-repeated."+tc.ext, tc.source)
			endpoint := annotationRegistrationURL(t, s, path, "Reviewer")
			pageURL := strings.Replace(endpoint, "/_annotations/v2/", "/", 1)
			anchor := ""
			for i := 1; i <= 3; i++ {
				status, data, err := testHTTPBody(s.client, pageURL)
				if err != nil || status != 200 {
					t.Fatal(status, err)
				}
				dom, err := html.Parse(bytes.NewReader(data))
				if err != nil {
					t.Fatal(err)
				}
				block := ""
				for n := range dom.Descendants() {
					if n.Data != tc.tag || attribute(n, "data-hp-annotation-block") == "" {
						continue
					}
					block = attribute(n, "data-hp-annotation-block")
					if tc.name == "heading" {
						id := attribute(n.Parent, "id")
						if i == 1 {
							anchor = id
						} else if id != anchor {
							t.Fatalf("heading anchor changed: %s -> %s", anchor, id)
						}
					}
					break
				}
				if block == "" {
					t.Fatalf("block unavailable after %d saves", i-1)
				}
				status, state := annotationJSON(t, s, "GET", endpoint, nil, nil)
				if status != 200 {
					t.Fatal(status, state)
				}
				suffix := fmt.Sprintf("%012d", i)
				headers := map[string]string{"Origin": s.origin, "X-HTMLPreview-Annotation-Token": state["write_token"].(string), "X-HTMLPreview-Composer-Token": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}
				request := map[string]any{"operation_id": "10000000-0000-4000-8000-" + suffix, "annotation_id": "20000000-0000-4000-8000-" + suffix, "composer_id": "30000000-0000-4000-8000-" + suffix, "sequence": 1, "revision": state["revision"], "source_revision": state["source_revision"], "body_revision": state["body_revision"], "action": "upsert", "label": fmt.Sprintf("reviewer-%03d", i), "target": map[string]any{"type": "point", "block_id": block}, "text": "Review note."}
				status, result := annotationJSON(t, s, "POST", endpoint, request, headers)
				if status != 201 {
					t.Fatal(status, result)
				}
			}
			// #nosec G304 -- Source path is created inside this test service's temporary root.
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			paragraphs := 1
			if tc.tag == "li" {
				paragraphs = 0
			}
			if strings.Count(string(data), "Annotations: ") != paragraphs {
				t.Fatalf("reference paragraph duplicated: %s", data)
			}
			want := "[^reviewer-001][^reviewer-002][^reviewer-003]"
			if tc.ext == "org" {
				want = "[fn:reviewer-001][fn:reviewer-002][fn:reviewer-003]"
			}
			if !strings.Contains(string(data), want) {
				t.Fatalf("missing successive references: %s", data)
			}
			if tc.tag == "li" && !strings.Contains(string(data), "- [X] Checked item."+want) {
				t.Fatalf("task syntax or item-end placement changed: %s", data)
			}
		})
	}
}

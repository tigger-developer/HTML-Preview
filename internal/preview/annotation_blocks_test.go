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
	if count != 2 {
		t.Fatalf("want heading and paragraph only, got %d", count)
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
	for _, tc := range []struct{ name, source, tag string }{
		{"heading", "## Heading *text*\n", "h2"},
		{"code", "```go\nfmt.Println(1)\n```\n", "pre"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := source(t, s.root, tc.name+"-repeated.md", tc.source)
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
			if strings.Count(string(data), "Annotations: ") != 1 {
				t.Fatalf("reference paragraph duplicated: %s", data)
			}
			if !strings.Contains(string(data), "[^reviewer-001][^reviewer-002][^reviewer-003]") {
				t.Fatalf("missing successive references: %s", data)
			}
		})
	}
}

// ABOUTME: Verifies heading, definition-list and after-link footnote saves through real HTTP and Pandoc.
// ABOUTME: Protects source markup and rejects task-marker and tag insertion.
package preview

import (
	"os"
	"strings"
	"testing"
)

func TestHTTPHeadingAndLinkAnnotations(t *testing.T) {
	s := startTestService(t, NativeHost())
	for _, tc := range []struct {
		name, ext, source, run, want string
		offset, position             int
		link                         bool
		status                       int
	}{
		{"heading", "md", "## Heading text\n", "Heading text", "## Heading[^reviewer-001] text", 7, 7, false, 201},
		{"prose-after-code", "org", "=P= means the numbered /paragraph/ in the export.\n", "means the numbered", "=P= means[fn:reviewer-001] the numbered", 5, 7, false, 201},
		{"prose-emphasis", "org", "=P= means the numbered /paragraph/ in the export.\n", "paragraph", "/para[fn:reviewer-001]graph/", 4, 25, false, 201},
		{"prose-after-emphasis", "org", "=P= means the numbered /paragraph/ in the export.\n", "in the export.", "in the[fn:reviewer-001] export.", 6, 37, false, 201},
		{"repeated-fragment", "org", "* TODO Standardize =dad= capitalization\n\nA capitalization preference.\n", "capitalization", "=dad= capital[fn:reviewer-001]ization", 7, 28, false, 201},
		{"later-fragment", "org", "* Heading\n\nA capitalization preference.\n\nAnother capitalization choice.\n", "capitalization", "Another capital[fn:reviewer-001]ization choice.", 7, 52, false, 201},
		{"wrong-position", "org", "* Heading\n\nA capitalization preference.\n\nAnother capitalization choice.\n", "capitalization", "", 7, 1, false, 409},
		{"candidate-limit", "org", strings.Repeat("Word.\n\n", 8), "Word.", "W[fn:reviewer-001]ord.", 1, 43, false, 201},
		{"candidate-overflow", "org", strings.Repeat("Word.\n\n", 9), "Word.", "", 1, 1, false, 409},
		{"repeated-fragment", "md", "## Standardize `dad` capitalization\n\nA capitalization preference.\n", "capitalization", "`dad` capital[^reviewer-001]ization", 7, 23, false, 201},
		{"heading", "org", "* TODO Heading text :tag:\n", "Heading text", "* TODO Heading[fn:reviewer-001] text :tag:", 7, 12, false, 201},
		{"definition-term", "org", "- Term :: Definition text.\n", "Term", "- Term[fn:reviewer-001] :: Definition text.", 4, 4, false, 201},
		{"definition-body", "org", "- Term :: Definition text.\n", "Definition text.", ":: Definition[fn:reviewer-001] text.", 10, 15, false, 201},
		{"definition-term", "md", "Term\n: Definition text.\n", "Term", "Term[^reviewer-001]\n: Definition text.", 4, 4, false, 201},
		{"definition-body", "md", "Term\n: Definition text.\n", "Definition text.", ": Definition[^reviewer-001] text.", 10, 15, false, 201},
		{"link", "md", "See [the guide](notes.md) next.\n", "the guide", "[the guide](notes.md)[^reviewer-001]", 9, 13, true, 201},
		{"link", "org", "See [[file:notes.org][the guide]] next.\n", "the guide", "[[file:notes.org][the guide]][fn:reviewer-001]", 9, 13, true, 201},
		{"formatted-link", "md", "See [the **guide**](notes.md) next.\n", "guide", "[the **guide**](notes.md)[^reviewer-001]", 5, 13, true, 201},
		{"heading-link", "md", "## See [guide](notes.md)\n", "guide", "[guide](notes.md)[^reviewer-001]", 5, 9, true, 201},
		{"todo", "org", "* TODO Heading :tag:\n", "TODO", "", 2, 2, false, 409},
		{"tag", "org", "* TODO Heading :tag:\n", "tag", "", 1, 14, false, 409},
	} {
		t.Run(tc.name+"-"+tc.ext, func(t *testing.T) {
			path := source(t, s.root, tc.name+"."+tc.ext, tc.source)
			endpoint := annotationRegistrationURL(t, s, path, "Reviewer")
			status, state := annotationJSON(t, s, "GET", endpoint, nil, nil)
			if status != 200 {
				t.Fatalf("state %d %#v", status, state)
			}
			headers := map[string]string{"Origin": s.origin, "X-HTMLPreview-Annotation-Token": state["write_token"].(string), "X-HTMLPreview-Composer-Token": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}
			event := map[string]any{"operation_id": "10000000-0000-4000-8000-000000000001", "annotation_id": "20000000-0000-4000-8000-000000000001", "composer_id": "30000000-0000-4000-8000-000000000001", "sequence": 1, "revision": state["revision"], "source_revision": state["source_revision"], "body_revision": state["body_revision"], "action": "upsert", "label": "reviewer-001", "target": map[string]any{"type": "point", "position": tc.position, "run": tc.run, "run_offset": tc.offset, "after_link": tc.link}, "text": "Review note."}
			status, saved := annotationJSON(t, s, "POST", endpoint, event, headers)
			if status != tc.status {
				t.Fatalf("save %d %#v", status, saved)
			}
			// #nosec G304 -- Synthetic source in this test service's temporary root.
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if tc.status == 201 {
				if !strings.Contains(string(data), tc.want) {
					t.Fatalf("source placement: %s", data)
				}
			} else if string(data) != tc.source {
				t.Fatal("refused save changed source")
			}
		})
	}
}

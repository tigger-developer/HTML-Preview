// ABOUTME: Checks confirmed native-footnote deletion at the authenticated HTTP boundary.
// ABOUTME: Protects source preservation, literal references and harmless retries.
package preview

import (
	"os"
	"strings"
	"testing"
)

func TestHTTPDeleteFootnote(t *testing.T) {
	s := startTestService(t, NativeHost())
	for _, format := range []string{"org", "md"} {
		t.Run(format, func(t *testing.T) {
			original := "A[fn:note] and B[fn:note].\n\nAnnotations: [fn:note]\n\n=literal [fn:note]=\n\n#+BEGIN_SRC text\n[fn:note]\n#+END_SRC\n\n[fn:keep] Keep this.\n\n\n[fn:note] Delete this.\n"
			want := "A and B.\n\n\n=literal [fn:note]=\n\n#+BEGIN_SRC text\n[fn:note]\n#+END_SRC\n\n[fn:keep] Keep this.\n\n\n"
			if format == "md" {
				original = "A[^note] and B[^note].\n\nAnnotations: [^note]\n\n`literal [^note]`\n\n```text\n[^note]\n```\n\n[^keep]: Keep this.\n\n[^note]: Delete this.\n"
				want = "A and B.\n\n\n`literal [^note]`\n\n```text\n[^note]\n```\n\n[^keep]: Keep this.\n\n"
			}
			path := source(t, s.root, "delete."+format, original)
			endpoint := annotationRegistrationURL(t, s, path, "Reviewer")
			status, state := annotationJSON(t, s, "GET", endpoint, nil, nil)
			if status != 200 {
				t.Fatal(status, state)
			}
			revision := ""
			for _, value := range state["footnotes"].([]any) {
				note := value.(map[string]any)
				if note["label"] == "note" {
					revision = note["revision"].(string)
				}
			}
			if revision == "" {
				t.Fatal("missing target footnote")
			}
			headers := map[string]string{"Origin": s.origin, "X-HTMLPreview-Annotation-Token": state["write_token"].(string), "X-HTMLPreview-Composer-Token": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}
			request := map[string]any{"operation_id": "10000000-0000-4000-8000-000000000001", "annotation_id": "20000000-0000-4000-8000-000000000001", "composer_id": "30000000-0000-4000-8000-000000000001", "sequence": 1, "revision": state["revision"], "source_revision": state["source_revision"], "body_revision": state["body_revision"], "action": "delete", "label": "note", "target": map[string]any{"type": "footnote", "exact": revision, "run": "embedded"}, "text": ""}
			status, result := annotationJSON(t, s, "POST", endpoint, request, headers)
			if status != 201 {
				t.Fatalf("delete %d %#v", status, result)
			}
			// #nosec G304 -- Synthetic source created within the isolated test root.
			data, err := os.ReadFile(path)
			if err != nil || string(data) != want {
				t.Fatalf("unexpected deletion bytes: %v\n%s", err, data)
			}
			status, result = annotationJSON(t, s, "POST", endpoint, request, headers)
			if status != 200 {
				t.Fatalf("retry %d %#v", status, result)
			}
			status, state = annotationJSON(t, s, "GET", endpoint, nil, nil)
			if status != 200 || strings.Contains(string(data), "Delete this.") {
				t.Fatal(status, state)
			}
			for _, value := range state["footnotes"].([]any) {
				if value.(map[string]any)["label"] == "note" {
					t.Fatal("deleted note still listed")
				}
			}
		})
	}
}

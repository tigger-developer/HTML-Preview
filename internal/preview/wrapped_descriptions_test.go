// ABOUTME: Checks annotation click targets around wrapped inline markup and indented quotes.
// ABOUTME: Verifies source-preserving HTTP writes and repeated block-end references.
package preview

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestHTTPWrappedDescriptions(t *testing.T) {
	fixture, err := os.ReadFile("../../testdata/annotation-targets/wrapped-descriptions.org")
	if err != nil {
		t.Fatal(err)
	}
	s := startTestService(t, NativeHost())
	for _, tc := range []struct{ name, text, tail string }{
		{"paragraph", "This is the end of the description.", "This is the end of the description."},
		{"label", "Attention", "This is the end of the description."},
		{"code", "description tail.", "span~ has a description tail."},
		{"quote", "Last quoted paragraph.", "  #+END_QUOTE\n\n  Annotations: "},
		{"quote-label", "Draft", "  #+END_QUOTE\n\n  Annotations: "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := source(t, s.root, tc.name+".org", string(fixture))
			endpoint := annotationRegistrationURL(t, s, path, "Reviewer")
			pageURL := strings.Replace(endpoint, "/_annotations/v2/", "/", 1)
			for i := 1; i <= 2; i++ {
				status, _, data := responseAsset(t, s, "GET", pageURL)
				if status != 200 {
					t.Fatal(status)
				}
				main := documentNode(t, parseHTTPDocument(t, data), "hp-document")
				if strings.Contains(textOf(main), "HPBLOCK") {
					t.Fatal("temporary probe marker leaked into the served document")
				}
				assertClickableAnnotationText(t, main, "last ordinary bullet")
				assertClickableAnnotationText(t, main, "Keep this description unchanged.")
				block := assertClickableAnnotationText(t, main, tc.text)
				if block == "" {
					t.Fatal("required click target unavailable")
				}
				status, state := annotationJSON(t, s, "GET", endpoint, nil, nil)
				if status != 200 {
					t.Fatal(status)
				}
				suffix := fmt.Sprintf("%012d", i)
				label := fmt.Sprintf("reviewer-%03d", i)
				headers := map[string]string{"Origin": s.origin, "X-HTMLPreview-Annotation-Token": state["write_token"].(string), "X-HTMLPreview-Composer-Token": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}
				request := map[string]any{"operation_id": "10000000-0000-4000-8000-" + suffix, "annotation_id": "20000000-0000-4000-8000-" + suffix, "composer_id": "30000000-0000-4000-8000-" + suffix, "sequence": 1, "revision": state["revision"], "source_revision": state["source_revision"], "body_revision": state["body_revision"], "action": "upsert", "label": label, "target": map[string]any{"type": "point", "block_id": block}, "text": "Synthetic annotation."}
				status, result := annotationJSON(t, s, "POST", endpoint, request, headers)
				if status != 201 {
					t.Fatal(status, result)
				}
			}
			// #nosec G304 -- Reads the synthetic source in the isolated service root.
			saved, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			refs := "[fn:reviewer-001][fn:reviewer-002]"
			if !strings.Contains(string(saved), tc.tail+refs) {
				t.Fatal("references not at required block end")
			}
			body := strings.Split(string(saved), "\n\n[fn:reviewer-001] ")[0]
			body = strings.Replace(body, refs, "", 1)
			if strings.HasPrefix(tc.name, "quote") {
				body = strings.Replace(body, "\n\n  Annotations: ", "", 1)
			}
			if body != string(fixture) {
				t.Fatal("unrelated source bytes changed")
			}
		})
	}
}

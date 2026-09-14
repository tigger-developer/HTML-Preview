// ABOUTME: Verifies that only authenticated annotation requests can modify a source.
// ABOUTME: Exercises real HTTP origin, capability, schema and selector boundaries.
package preview

import (
	"bytes"
	"os"
	"testing"

	"github.com/tigger-developer/HTML-Preview/internal/annotation"
)

func TestRT007_7_AnnotationHTTPAuthority(t *testing.T) {
	s := startTestService(t, NativeHost())
	path := source(t, s.root, "secure.org", "* Secure source\nA passage to review.\n")
	endpoint := annotationRegistrationURL(t, s, path, "Reviewer")
	_, state := annotationJSON(t, s, "GET", endpoint, nil, nil)
	request := annotation.Request{OperationID: "10000000-0000-4000-8000-000000000001", AnnotationID: "20000000-0000-4000-8000-000000000001", ComposerID: "30000000-0000-4000-8000-000000000001", Sequence: 1, Revision: state["revision"].(string), SourceRevision: state["source_revision"].(string), BodyRevision: state["body_revision"].(string), Action: "upsert", Label: "reviewer-001", Target: annotation.Target{Type: "point", Position: 15, Run: "A passage to review.", RunOffset: 1}, Text: "Review comment"}
	base := map[string]string{"Origin": s.origin, "X-HTMLPreview-Annotation-Token": state["write_token"].(string), "X-HTMLPreview-Composer-Token": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}
	// #nosec G304 -- This test creates the source in its own temporary root.
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, key, value string }{{"missing origin", "Origin", ""}, {"foreign origin", "Origin", "https://foreign.invalid"}, {"null origin", "Origin", "null"}, {"missing write token", "X-HTMLPreview-Annotation-Token", ""}, {"wrong write token", "X-HTMLPreview-Annotation-Token", "forged"}, {"missing composer token", "X-HTMLPreview-Composer-Token", ""}, {"foreign fetch metadata", "Sec-Fetch-Site", "cross-site"}} {
		t.Run(tc.name, func(t *testing.T) {
			headers := map[string]string{}
			for k, v := range base {
				headers[k] = v
			}
			headers[tc.key] = tc.value
			status, _ := annotationJSON(t, s, "POST", endpoint, request, headers)
			if status != 403 {
				t.Fatalf("forbidden request=%d", status)
			}
			// #nosec G304 -- Reads only the fixture created above to detect forbidden writes.
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(original, after) {
				t.Fatal("forbidden request changed bytes")
			}
		})
	}
	status, saved := annotationJSON(t, s, "POST", endpoint, request, base)
	if status != 201 {
		t.Fatalf("authorized control=%d %#v", status, saved)
	}
}

func TestRT007_9_AnnotationSelectorValidation(t *testing.T) {
	s := startTestService(t, NativeHost())
	path := source(t, s.root, "selection.md", "# Heading\n\nA café passage.\n")
	endpoint := annotationRegistrationURL(t, s, path, "Reviewer")
	_, state := annotationJSON(t, s, "GET", endpoint, nil, nil)
	base := map[string]string{"Origin": s.origin, "X-HTMLPreview-Annotation-Token": state["write_token"].(string), "X-HTMLPreview-Composer-Token": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}
	request := annotation.Request{OperationID: "10000000-0000-4000-8000-000000000001", AnnotationID: "20000000-0000-4000-8000-000000000001", ComposerID: "30000000-0000-4000-8000-000000000001", Sequence: 1, Revision: state["revision"].(string), SourceRevision: state["source_revision"].(string), BodyRevision: state["body_revision"].(string), Action: "upsert", Label: "reviewer-001", Target: annotation.Target{Type: "point", BodyRevision: state["body_revision"].(string), Position: 14, Run: "A café passage.", RunOffset: 6}, Text: "A selected comment"}
	request.Target.HeadingID = "heading"
	if status, _ := annotationJSON(t, s, "POST", endpoint, request, base); status != 400 {
		t.Fatalf("generated heading accepted as provenance: %d", status)
	}
	request.Target.HeadingID = ""
	status, result := annotationJSON(t, s, "POST", endpoint, request, base)
	if status != 201 {
		t.Fatalf("valid selection=%d %#v", status, result)
	}
}

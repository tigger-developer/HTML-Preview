// ABOUTME: Exercises malformed and oversized events through authenticated HTTP.
// ABOUTME: Verifies rejected events leave real source bytes unchanged.
package preview

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestRT007_7_10_EventSchemaAndLimits(t *testing.T) {
	s := startTestService(t, NativeHost())
	path := source(t, s.root, "limits.org", "* Limits\nA passage.\n")
	endpoint := annotationRegistrationURL(t, s, path, "Reviewer")
	_, state := annotationJSON(t, s, "GET", endpoint, nil, nil)
	headers := map[string]string{"Origin": s.origin, "X-HTMLPreview-Annotation-Token": state["write_token"].(string), "X-HTMLPreview-Composer-Token": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}
	original := []byte("* Limits\nA passage.\n")
	base := map[string]any{"operation_id": "10000000-0000-4000-8000-000000000001", "annotation_id": "20000000-0000-4000-8000-000000000001", "composer_id": "30000000-0000-4000-8000-000000000001", "sequence": 1, "revision": state["revision"], "source_revision": state["source_revision"], "body_revision": state["body_revision"], "kind": "draft", "target": map[string]any{"type": "document"}, "text": "A valid draft"}
	for _, tc := range []struct {
		name, key string
		value     any
		status    int
	}{
		{"forged author", "author", "Somebody else", 400}, {"forged date", "recorded_at", "2020-01-01T00:00:00Z", 400}, {"arbitrary path", "path", "/private/other.org", 400},
		{"empty text", "text", "", 400}, {"NUL", "text", "bad\x00text", 400}, {"unknown operation", "kind", "delete", 400}, {"invalid UUID", "operation_id", "invalid", 400},
		{"too many characters", "text", strings.Repeat("a", 4001), 413}, {"too many bytes", "text", strings.Repeat("😀", 4097), 413}, {"request body limit", "text", strings.Repeat("a", 65537), 413},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event := map[string]any{}
			for key, value := range base {
				event[key] = value
			}
			event[tc.key] = tc.value
			status, response := annotationJSON(t, s, "POST", endpoint, event, headers)
			if status != tc.status {
				t.Fatalf("status=%d expected=%d body=%v", status, tc.status, response)
			}
			// #nosec G304 -- The path is the synthetic source created in this test's root.
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(after, original) {
				t.Fatal("rejected event changed the file")
			}
		})
	}
	status, _ := annotationJSON(t, s, "POST", endpoint, base, headers)
	if status != 201 {
		t.Fatal("authorized control failed", status)
	}
	otherPath := source(t, s.root, "another.org", "* Another\n")
	otherEndpoint := annotationRegistrationURL(t, s, otherPath, "Reviewer")
	annotationJSON(t, s, "GET", otherEndpoint, nil, nil)
	if status, _ := annotationJSON(t, s, "POST", otherEndpoint, base, headers); status != 403 {
		t.Fatal("write token crossed document boundary")
	}
	for _, method := range []string{"PUT", "PATCH", "DELETE"} {
		if status, _ := annotationJSON(t, s, method, endpoint, base, headers); status != 405 {
			t.Fatal("unexpected mutation method", method, status)
		}
	}
}

// ABOUTME: Reproduces extended-attribute annotation saves through the HTTP service.
// ABOUTME: Checks successful native footnote persistence without losing macOS metadata.
package preview

import (
	"bytes"
	"os"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestHTTPAnnotationSaveWithExtendedAttributes(t *testing.T) {
	s := startTestService(t, NativeHost())
	for _, format := range []string{"md", "org"} {
		t.Run(format, func(t *testing.T) {
			path := source(t, s.root, "attributed."+format, "A preserved paragraph.\n")
			name := "user.htmlpreview-test"
			if runtime.GOOS == "darwin" {
				name = "com.apple.lastuseddate#PS"
			}
			value := make([]byte, 16)
			value[0] = 42
			if err := unix.Setxattr(path, name, value, 0); err != nil {
				t.Fatal(err)
			}
			endpoint := annotationRegistrationURL(t, s, path, "Reviewer")
			status, state := annotationJSON(t, s, "GET", endpoint, nil, nil)
			if status != 200 || state["writable"] != true {
				t.Fatalf("state: %d %#v", status, state)
			}
			headers := map[string]string{"Origin": s.origin, "X-HTMLPreview-Annotation-Token": state["write_token"].(string), "X-HTMLPreview-Composer-Token": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}
			event := map[string]any{"operation_id": "10000000-0000-4000-8000-000000000001", "annotation_id": "20000000-0000-4000-8000-000000000001", "composer_id": "30000000-0000-4000-8000-000000000001", "sequence": 1, "revision": state["revision"], "source_revision": state["source_revision"], "body_revision": state["body_revision"], "action": "upsert", "label": "reviewer-001", "target": map[string]any{"type": "point", "position": 1, "run": "A preserved paragraph.", "run_offset": 1}, "text": "Saved with metadata."}
			status, saved := annotationJSON(t, s, "POST", endpoint, event, headers)
			if status != 201 {
				t.Fatalf("save: %d %#v", status, saved)
			}
			// #nosec G304 -- Synthetic document created in this test service's temporary root.
			data, err := os.ReadFile(path)
			if err != nil || !strings.Contains(string(data), "Saved with metadata.") {
				t.Fatalf("saved footnote missing: %v", err)
			}
			got := make([]byte, 32)
			n, err := unix.Getxattr(path, name, got)
			if err != nil || !bytes.Equal(got[:n], value) {
				t.Fatalf("attribute changed: %x %v", got[:n], err)
			}
		})
	}
}

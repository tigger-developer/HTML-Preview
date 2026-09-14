// ABOUTME: Exercises current footnote state through the authorized service boundary.
// ABOUTME: Protects W009 protocol admission and native annotation persistence.
package preview

import (
	"strings"
	"testing"
)

func TestRT009_6_CurrentFootnoteEndpoint(t *testing.T) {
	s := startTestService(t, NativeHost())
	path := source(t, s.root, "notes.org", "* Notes\n\nA sentence to annotate.\n")
	endpoint := strings.Replace(annotationRegistrationURL(t, s, path, "Taḋg"), "/_annotations/v1/", "/_annotations/v2/", 1)
	status, state := annotationJSON(t, s, "GET", endpoint, nil, nil)
	if status != 200 || state["protocol"] != float64(2) || state["comments"] == nil {
		t.Fatalf("current-footnote state unavailable: status=%d state=%#v", status, state)
	}
}

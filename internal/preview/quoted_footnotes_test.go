// ABOUTME: Exercises quoted native footnotes through the served-page boundary.
// ABOUTME: Prevents phantom unplaced copies and preserves unrelated notes on deletion.
package preview

import (
	"os"
	"strings"
	"testing"
)

// RT011.5: A real reference in a rendered quote is already placed, including
// indented quotes inside description lists and references inside words.
func TestHTTPQuotedFootnotes(t *testing.T) {
	fixture, err := os.ReadFile("../../testdata/annotation-targets/quoted-footnotes.org")
	if err != nil {
		t.Fatal(err)
	}
	s := startTestService(t, NativeHost())
	path := source(t, s.root, "quoted.org", string(fixture))
	endpoint := annotationRegistrationURL(t, s, path, "Reviewer")
	pageURL := strings.Replace(endpoint, "/_annotations/v2/", "/", 1)
	check := func(labels ...string) map[string]any {
		t.Helper()
		status, state := annotationJSON(t, s, "GET", endpoint, nil, nil)
		if status != 200 || len(state["footnotes"].([]any)) != len(labels) {
			t.Fatal("native note inventory", status, state)
		}
		status, _, data := responseAsset(t, s, "GET", pageURL)
		if status != 200 {
			t.Fatal("page status", status)
		}
		doc := parseHTTPDocument(t, data)
		if strings.Contains(textOf(documentNode(t, doc, "hp-document")), "Unplaced annotation") {
			t.Error("placed quote reference generated an unplaced duplicate")
		}
		counts := make(map[string]int)
		for _, li := range nodes(doc, "li") {
			if label := attr(li, "data-hp-footnote-label"); label != "" {
				counts[label]++
				backlinks := 0
				for _, a := range nodes(li, "a") {
					if attr(a, "role") == "doc-backlink" {
						backlinks++
						ref := documentNode(t, doc, strings.TrimPrefix(attr(a, "href"), "#"))
						if attr(ref, "role") != "doc-noteref" || attr(ref, "href") != "#"+attr(li, "id") {
							t.Error("backlink does not return to its reference")
						}
					}
				}
				if backlinks != 1 {
					t.Errorf("note %s has %d backlinks", label, backlinks)
				}
			}
		}
		if len(counts) != len(labels) {
			t.Errorf("rendered note labels: %v", counts)
		}
		for _, label := range labels {
			if counts[label] != 1 {
				t.Errorf("note %s rendered %d times; want once", label, counts[label])
			}
		}
		return state
	}
	state := check("first", "second")
	var note map[string]any
	for _, value := range state["footnotes"].([]any) {
		if candidate := value.(map[string]any); candidate["label"] == "second" {
			note = candidate
		}
	}
	if note == nil {
		t.Fatal("second note absent")
	}
	headers := map[string]string{"Origin": s.origin, "X-HTMLPreview-Annotation-Token": state["write_token"].(string), "X-HTMLPreview-Composer-Token": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}
	request := map[string]any{
		"operation_id": "10000000-0000-4000-8000-000000000001", "annotation_id": "20000000-0000-4000-8000-000000000001",
		"composer_id": "30000000-0000-4000-8000-000000000001", "sequence": 1,
		"revision": state["revision"], "source_revision": state["source_revision"], "body_revision": state["body_revision"],
		"action": "delete", "label": "second", "text": "", "target": map[string]any{"type": "footnote", "exact": note["revision"], "run": "embedded"},
	}
	status, response := annotationJSON(t, s, "POST", endpoint, request, headers)
	if status != 201 {
		t.Fatal("delete", status, response)
	}
	check("first")
	// #nosec G304 -- Synthetic source created in the isolated service root.
	data, err := os.ReadFile(path)
	want := strings.Replace(strings.Split(string(fixture), "[fn:second] Second synthetic note.")[0], "[fn:second]", "", 1)
	if err != nil || string(data) != want {
		t.Fatal("deleting the second note changed unrelated source bytes", err)
	}
}

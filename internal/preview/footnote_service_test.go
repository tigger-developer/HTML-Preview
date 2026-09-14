// ABOUTME: Exercises current footnote state through the authorized service boundary.
// ABOUTME: Protects W009 protocol admission and native annotation persistence.
package preview

import (
	"encoding/base64"
	"fmt"
	"github.com/tigger-developer/HTML-Preview/internal/annotation"
	"os"
	"strings"
	"testing"
)

func TestRT009_5_LiteralSourcePayload(t *testing.T) {
	for _, ext := range []string{"org", "md", "txt", "go"} {
		t.Run(ext, func(t *testing.T) {
			root := t.TempDir()
			original := "A\tline.\r\n\r\n</template><script>not executable</script>\r\n"
			path := source(t, root, "source."+ext, original)
			result := run(t, root, nil, path)
			if result.code != 0 {
				t.Fatalf("preview failed: %#v", result)
			}
			var payload string
			found := false
			for _, n := range nodes(result.pages[0], "template") {
				if attr(n, "id") == "hp-source-data" {
					found = true
					payload = textOf(n)
				}
			}
			want := ext == "org" || ext == "md"
			if found != want {
				t.Fatalf("source payload offered=%v want=%v", found, want)
			}
			if want {
				decoded, err := base64.StdEncoding.DecodeString(payload)
				if err != nil || string(decoded) != original {
					t.Fatalf("source bytes changed: %v %q", err, decoded)
				}
			}
		})
	}
}

func TestRT009_8_NativeNotesRemainInRenderedPage(t *testing.T) {
	root := t.TempDir()
	event := annotation.Event{Schema: 2, AnnotationID: "20000000-0000-4000-8000-000000000001", OperationID: "30000000-0000-4000-8000-000000000001", Author: "Taḋg", CreatedAt: "2026-09-14T10:00:00Z", RecordedAt: "2026-09-14T10:00:01Z", Sequence: 1, Kind: "draft", Target: annotation.Target{Type: "point"}, Text: "A saved native comment."}
	header := annotation.Header{Schema: 1, DocumentID: "10000000-0000-4000-8000-000000000001", SourceFormat: "org"}
	data, err := annotation.UpdateFootnote([]byte("* Heading\n\nA sentence.\n"), "org", header, event, "tadg-001", len("* Heading\n\nA sentence"))
	if err != nil {
		t.Fatal(err)
	}
	path := source(t, root, "native.org", string(data))
	result := run(t, root, nil, path)
	if result.code != 0 {
		t.Fatalf("preview failed: %#v", result)
	}
	var endnote bool
	for _, n := range nodes(result.pages[0], "section") {
		if strings.Contains(attr(n, "class"), "footnotes") {
			endnote = strings.Contains(textOf(n), event.Text)
			if attr(n, "role") != "doc-endnotes" {
				t.Fatal("native footnote accessibility role removed")
			}
		}
	}
	if !endnote || strings.Contains(result.raw[0], "HTMLPREVIEW_ANNOTATION") {
		t.Fatal("native footnote missing or metadata exposed")
	}
}

func TestRT009_7_HTTPCurrentFootnotes(t *testing.T) {
	s := startTestService(t, NativeHost())
	for _, format := range []string{"org", "md"} {
		t.Run(format, func(t *testing.T) {
			path := source(t, s.root, "current."+format, "A sentence to annotate.\n")
			endpoint := strings.Replace(annotationRegistrationURL(t, s, path, "Taḋg"), "/_annotations/v1/", "/_annotations/v2/", 1)
			status, state := annotationJSON(t, s, "GET", endpoint, nil, nil)
			if status != 200 {
				t.Fatal(status, state)
			}
			headers := map[string]string{"Origin": s.origin, "X-HTMLPreview-Annotation-Token": state["write_token"].(string), "X-HTMLPreview-Composer-Token": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}
			request := map[string]any{"operation_id": "10000000-0000-4000-8000-000000000001", "annotation_id": "20000000-0000-4000-8000-000000000001", "composer_id": "30000000-0000-4000-8000-000000000001", "sequence": 1, "revision": state["revision"], "source_revision": state["source_revision"], "body_revision": state["body_revision"], "action": "upsert", "label": "tadg-001", "target": map[string]any{"type": "point", "position": 10, "run": "A sentence to annotate.", "run_offset": 10}, "text": "Draft value 1"}
			for i := 1; i <= 20; i++ {
				request["sequence"], request["operation_id"], request["text"] = i, fmt.Sprintf("10000000-0000-4000-8000-%012d", i), fmt.Sprintf("Draft value %d", i)
				status, saved := annotationJSON(t, s, "POST", endpoint, request, headers)
				if status != 201 {
					t.Fatalf("save %d: %d %#v", i, status, saved)
				}
				for _, key := range []string{"revision", "source_revision", "body_revision"} {
					request[key] = saved[key]
				}
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(string(data), "HTMLPREVIEW_ANNOTATION: 2") != 1 || strings.Contains(string(data), "Draft value 19") || !strings.Contains(string(data), "Draft value 20") {
				t.Fatalf("current native value not retained: %s", data)
			}
			status, current := annotationJSON(t, s, "GET", endpoint, nil, nil)
			if status != 200 || len(current["comments"].([]any)) != 1 {
				t.Fatal(status, current)
			}
		})
	}
}

func TestRT009_6_CurrentFootnoteEndpoint(t *testing.T) {
	s := startTestService(t, NativeHost())
	path := source(t, s.root, "notes.org", "* Notes\n\nA sentence to annotate.\n")
	endpoint := strings.Replace(annotationRegistrationURL(t, s, path, "Taḋg"), "/_annotations/v1/", "/_annotations/v2/", 1)
	status, state := annotationJSON(t, s, "GET", endpoint, nil, nil)
	if status != 200 || state["protocol"] != float64(2) || state["comments"] == nil {
		t.Fatalf("current-footnote state unavailable: status=%d state=%#v", status, state)
	}
}

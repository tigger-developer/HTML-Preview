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
				request["sequence"], request["operation_id"], request["text"] = i, fmt.Sprintf("10000000-0000-4000-8000-%012d", i), fmt.Sprintf("Draft value %d with *markup*.\n\nSecond paragraph with `code`.", i)
				status, saved := annotationJSON(t, s, "POST", endpoint, request, headers)
				if status != 201 {
					t.Fatalf("save %d: %d %#v", i, status, saved)
				}
				for _, key := range []string{"revision", "source_revision", "body_revision"} {
					request[key] = saved[key]
				}
			}
			// #nosec G304 -- Reads only the synthetic source allocated in this test's temporary root.
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "HTMLPREVIEW_ANNOTATION") || strings.Contains(string(data), "BEGIN_COMMENT") || !strings.Contains(string(data), "Author: Taḋg; Created: [") || strings.Contains(string(data), "Draft value 19") || !strings.Contains(string(data), "Draft value 20") {
				t.Fatalf("current native value not retained: %s", data)
			}
			status, current := annotationJSON(t, s, "GET", endpoint, nil, nil)
			if status != 200 || len(current["footnotes"].([]any)) != 1 || current["footnotes"].([]any)[0].(map[string]any)["text"] != request["text"] {
				t.Fatal(status, current)
			}
			// A different registered reviewer reopens the saved native definition.
			endpoint = strings.Replace(annotationRegistrationURL(t, s, path, "Another reviewer"), "/_annotations/v1/", "/_annotations/v2/", 1)
			status, current = annotationJSON(t, s, "GET", endpoint, nil, nil)
			if status != 200 {
				t.Fatal(status, current)
			}
			note := current["footnotes"].([]any)[0].(map[string]any)
			headers["X-HTMLPreview-Annotation-Token"] = current["write_token"].(string)
			updatedText := "Edited *native* markup.\n\nAnother paragraph with `code`."
			edit := map[string]any{"operation_id": "10000000-0000-4000-8000-000000000099", "annotation_id": "20000000-0000-4000-8000-000000000099", "composer_id": "30000000-0000-4000-8000-000000000099", "sequence": 1, "action": "edit", "label": note["label"], "text": updatedText, "revision": current["revision"], "source_revision": current["source_revision"], "body_revision": current["body_revision"], "target": map[string]any{"type": "footnote", "exact": note["revision"], "run": "embedded"}}
			status, saved := annotationJSON(t, s, "POST", endpoint, edit, headers)
			if status != 201 {
				t.Fatal(status, saved)
			}
			status, current = annotationJSON(t, s, "GET", endpoint, nil, nil)
			if status != 200 {
				t.Fatal(status, current)
			}
			note = current["footnotes"].([]any)[0].(map[string]any)
			if note["text"] != updatedText || note["author"] != "Another reviewer" || !strings.HasPrefix(note["created_at"].(string), "[") {
				t.Fatal("reopened native note or attribution changed incorrectly", note)
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

func TestRT009_8_ReadOnlySidecarUsesNativeEndnotes(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires unprivileged source permission checks")
	}
	s := startTestService(t, NativeHost())
	for _, ext := range []string{"org", "md"} {
		t.Run(ext, func(t *testing.T) {
			path := source(t, s.root, "sidecar."+ext, "A sentence to annotate.\n")
			if err := os.Chmod(path, 0400); err != nil {
				t.Fatal(err)
			}
			endpoint := strings.Replace(annotationRegistrationURL(t, s, path, "Taḋg"), "/_annotations/v1/", "/_annotations/v2/", 1)
			status, state := annotationJSON(t, s, "GET", endpoint, nil, nil)
			if status != 200 {
				t.Fatal(status, state)
			}
			headers := map[string]string{"Origin": s.origin, "X-HTMLPreview-Annotation-Token": state["write_token"].(string), "X-HTMLPreview-Composer-Token": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}
			request := map[string]any{"operation_id": "10000000-0000-4000-8000-000000000001", "annotation_id": "20000000-0000-4000-8000-000000000001", "composer_id": "30000000-0000-4000-8000-000000000001", "sequence": 1, "revision": state["revision"], "source_revision": state["source_revision"], "body_revision": state["body_revision"], "action": "upsert", "label": "tadg-001", "target": map[string]any{"type": "point", "position": 10, "run": "A sentence to annotate.", "run_offset": 10}, "text": "A sidecar footnote"}
			status, saved := annotationJSON(t, s, "POST", endpoint, request, headers)
			if status != 201 {
				t.Fatal(status, saved)
			}
			pageURL := strings.Replace(endpoint, "/_annotations/v2/", "/", 1)
			pageStatus, _, data := responseAsset(t, s, "GET", pageURL)
			if pageStatus != 200 {
				t.Fatal(pageStatus)
			}
			page := parseHTTPDocument(t, data)
			var count int
			for _, section := range nodes(page, "section") {
				if attr(section, "role") == "doc-endnotes" && strings.Contains(textOf(section), "A sidecar footnote") {
					count++
				}
			}
			if count != 1 {
				t.Fatal("sidecar comment is absent from the single native endnotes section")
			}
			status, current := annotationJSON(t, s, "GET", endpoint, nil, nil)
			if status != 200 || current["body_revision"] != state["body_revision"] {
				t.Fatal("virtual footnote changed authored body", status, current)
			}
			if len(current["footnotes"].([]any)) != 1 || !strings.Contains(string(data), `role="doc-noteref"`) || strings.Contains(string(data), "Unplaced annotation") {
				t.Fatal("sidecar native reference unavailable")
			}
			// #nosec G304 -- Reads only the synthetic source allocated in this test's temporary root.
			original, err := os.ReadFile(path)
			if err != nil || string(original) != "A sentence to annotate.\n" {
				t.Fatal("read-only source changed", err)
			}
			if err := os.Chmod(path, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("Entirely different text.\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, 0400); err != nil {
				t.Fatal(err)
			}
			pageStatus, _, data = responseAsset(t, s, "GET", pageURL)
			if pageStatus != 200 {
				t.Fatalf("unplaced page: %d %s", pageStatus, data)
			}
			page = parseHTTPDocument(t, data)
			unplaced := false
			for _, section := range nodes(page, "section") {
				if attr(section, "role") == "doc-endnotes" {
					unplaced = strings.Contains(textOf(section), "Unplaced annotation") && strings.Contains(textOf(section), "A sidecar footnote")
				}
			}
			if !unplaced {
				t.Fatal("unmatched sidecar note was lost or falsely located")
			}
			for _, anchor := range nodes(page, "a") {
				if attr(anchor, "role") == "doc-noteref" || attr(anchor, "role") == "doc-backlink" {
					t.Fatal("unplaced note retains a false source link")
				}
			}
		})
	}
}

func TestRT009_7_HTTPLegacyReadAndImport(t *testing.T) {
	s := startTestService(t, NativeHost())
	for _, format := range []string{"org", "markdown"} {
		t.Run(format, func(t *testing.T) {
			body := "One unique sentence.\n"
			header := annotation.Header{Schema: 1, DocumentID: "40000000-0000-4000-8000-000000000001", SourceFormat: format}
			event := annotation.Event{Schema: 1, OperationID: "10000000-0000-4000-8000-000000000001", AnnotationID: "20000000-0000-4000-8000-000000000001", ComposerID: "30000000-0000-4000-8000-000000000001", Sequence: 1, Kind: "draft", Author: "Original reviewer", CreatedAt: "2026-09-13T12:00:00Z", RecordedAt: "2026-09-13T12:00:00Z", Target: annotation.Target{Type: "text", BodyRevision: annotation.Digest([]byte(strings.TrimSpace(body))), Exact: "unique", Start: 4, End: 10, Prefix: "One ", Suffix: " sentence."}, Text: "Retain this legacy comment"}
			addition, err := annotation.AppendBytes(annotation.Parse([]byte(body), format), &header, event, format)
			if err != nil {
				t.Fatal(err)
			}
			ext := ".org"
			if format == "markdown" {
				ext = ".md"
			}
			path := source(t, s.root, "legacy"+ext, body+string(addition))
			endpoint := strings.Replace(annotationRegistrationURL(t, s, path, "Taḋg"), "/_annotations/v1/", "/_annotations/v2/", 1)
			pageURL := strings.Replace(endpoint, "/_annotations/v2/", "/", 1)
			status, _, output := responseAsset(t, s, "GET", pageURL)
			if status != 200 || !strings.Contains(string(output), event.Text) {
				t.Fatalf("legacy read: %d", status)
			}
			// #nosec G304 -- Reads only the synthetic source allocated in this test's temporary root.
			unchanged, err := os.ReadFile(path)
			if err != nil || string(unchanged) != body+string(addition) {
				t.Fatal("reading migrated source", err)
			}
			status, state := annotationJSON(t, s, "GET", endpoint, nil, nil)
			if status != 200 {
				t.Fatal(status, state)
			}
			headers := map[string]string{"Origin": s.origin, "X-HTMLPreview-Annotation-Token": state["write_token"].(string), "X-HTMLPreview-Composer-Token": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}
			request := map[string]any{"operation_id": "50000000-0000-4000-8000-000000000001", "annotation_id": "60000000-0000-4000-8000-000000000001", "composer_id": "70000000-0000-4000-8000-000000000001", "sequence": 1, "revision": state["revision"], "source_revision": state["source_revision"], "body_revision": state["body_revision"], "action": "upsert", "label": "tadg-001", "target": map[string]any{"type": "point", "position": 19, "run": "One unique sentence.", "run_offset": 19}, "text": "New footnote"}
			status, saved := annotationJSON(t, s, "POST", endpoint, request, headers)
			if status != 201 {
				t.Fatal(status, saved)
			}
			current, err := annotation.Read(annotation.Location{Root: s.root, Path: path, Format: format})
			if err != nil || current.Reason != "" || len(annotation.EditableFootnotes(current.RawSource, format, "embedded")) != 2 || string(annotation.FootnoteBodySource(current.RawSource, format)) != body {
				t.Fatalf("invalid migrated source: %v %s", err, current.Reason)
			}
			for _, note := range annotation.EditableFootnotes(current.RawSource, format, "embedded") {
				reference := "[fn:" + note.Label + "]"
				if format != "org" {
					reference = "[^" + note.Label + "]"
				}
				if strings.Count(string(current.RawSource), reference) != 2 {
					t.Fatal("verified legacy passage did not receive native reference")
				}
			}
		})
	}
}

func TestRT009_7_PreviousProtocolCannotAppendHistory(t *testing.T) {
	s := startTestService(t, NativeHost())
	path := source(t, s.root, "previous.org", "A sentence.\n")
	endpoint := strings.Replace(annotationRegistrationURL(t, s, path, "Reviewer"), "/_annotations/v2/", "/_annotations/v1/", 1)
	_, state := annotationJSON(t, s, "GET", endpoint, nil, nil)
	headers := map[string]string{"Origin": s.origin, "X-HTMLPreview-Annotation-Token": state["write_token"].(string), "X-HTMLPreview-Composer-Token": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}
	request := map[string]any{"operation_id": "10000000-0000-4000-8000-000000000001", "annotation_id": "20000000-0000-4000-8000-000000000001", "composer_id": "30000000-0000-4000-8000-000000000001", "sequence": 1, "revision": state["revision"], "source_revision": state["source_revision"], "body_revision": state["body_revision"], "kind": "draft", "target": map[string]any{"type": "document"}, "text": "Must not append history"}
	status, response := annotationJSON(t, s, "POST", endpoint, request, headers)
	if status != 409 || response["error"] != "annotation_upgrade_required" {
		t.Fatalf("old protocol remains a writer: %d %#v", status, response)
	}
	// #nosec G304 -- Reads only the synthetic source allocated in this test's temporary root.
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "A sentence.\n" {
		t.Fatal("old request changed source", err)
	}
}

func TestRT009_7_EditOrdinaryFootnotes(t *testing.T) {
	s := startTestService(t, NativeHost())
	for _, format := range []string{"org", "md"} {
		t.Run(format, func(t *testing.T) {
			original := "A[fn:12] and again[fn:12].\n\n[fn:12] *Original* note.\n\n\n* Next\n\nUnrelated text.\n"
			if format == "md" {
				original = "A[^12] and again[^12].\n\n[^12]: **Original** note.\n\n# Next\n\nUnrelated text.\n"
			}
			path := source(t, s.root, "editable."+format, original)
			endpoint := strings.Replace(annotationRegistrationURL(t, s, path, "Reviewer"), "/_annotations/v1/", "/_annotations/v2/", 1)
			status, state := annotationJSON(t, s, "GET", endpoint, nil, nil)
			notes, ok := state["footnotes"].([]any)
			if status != 200 || !ok || len(notes) != 1 {
				t.Fatalf("ordinary footnote unavailable for editing: %d %#v", status, state)
			}
			note := notes[0].(map[string]any)
			if note["label"] != "12" || !strings.Contains(note["text"].(string), "Original") {
				t.Fatal("native ID or text lost", note)
			}
			headers := map[string]string{"Origin": s.origin, "X-HTMLPreview-Annotation-Token": state["write_token"].(string), "X-HTMLPreview-Composer-Token": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}
			request := map[string]any{"operation_id": "10000000-0000-4000-8000-000000000001", "annotation_id": "20000000-0000-4000-8000-000000000001", "composer_id": "30000000-0000-4000-8000-000000000001", "sequence": 1, "revision": state["revision"], "source_revision": state["source_revision"], "body_revision": state["body_revision"], "action": "edit", "label": "12", "target": map[string]any{"type": "footnote", "exact": note["revision"], "run": "embedded"}, "text": strings.Replace(note["text"].(string), "Original", "Updated", 1)}
			status, saved := annotationJSON(t, s, "POST", endpoint, request, headers)
			if status != 201 {
				t.Fatalf("edit refused: %d %#v", status, saved)
			}
			// #nosec G304 -- Synthetic file within this test's isolated root.
			data, err := os.ReadFile(path)
			if err != nil || string(data) != strings.Replace(original, "Original", "Updated", 1) {
				t.Fatalf("edit changed unrelated bytes: %v\n%s", err, data)
			}
			status, retry := annotationJSON(t, s, "POST", endpoint, request, headers)
			if status != 200 {
				t.Fatalf("lost-response retry: %d %#v", status, retry)
			}
			request["text"] = "Stale replacement"
			status, conflict := annotationJSON(t, s, "POST", endpoint, request, headers)
			if status != 409 || conflict["error"] != "footnote_conflict" {
				t.Fatalf("stale edit accepted: %d %#v", status, conflict)
			}
			status, current := annotationJSON(t, s, "GET", endpoint, nil, nil)
			if status != 200 || !strings.Contains(current["footnotes"].([]any)[0].(map[string]any)["text"].(string), "Updated") {
				t.Fatal("reload lost current footnote", status, current)
			}
			pageURL := strings.Replace(endpoint, "/_annotations/v2/", "/", 1)
			pageStatus, _, data := responseAsset(t, s, "GET", pageURL)
			if pageStatus != 200 {
				t.Fatal(pageStatus)
			}
			page := parseHTTPDocument(t, data)
			mapped := 0
			for _, node := range nodes(page, "li") {
				if attr(node, "data-hp-footnote-label") == "12" {
					mapped++
				}
			}
			if mapped != 2 {
				t.Fatalf("repeated rendered notes mapped=%d nodes=%v", mapped, nodes(page, "li")[0].Attr)
			}
		})
	}
}

func TestRT009_8_EditableNotesPreserveRenderedMarkup(t *testing.T) {
	s := startTestService(t, NativeHost())
	for _, tc := range []struct{ format, body string }{
		{"org", "Text[fn:a].\n\n[fn:a] - First item\n  - Second item\n\n#+BEGIN_EXAMPLE\n[fn:fake] literal\n#+END_EXAMPLE\n"},
		{"md", "Text[^a].\n\n[^a]: - First item\n      - Second item\n\n    ```\n    [^fake]: literal\n    ```\n"},
	} {
		t.Run(tc.format, func(t *testing.T) {
			path := source(t, s.root, "markup."+tc.format, tc.body)
			endpoint := strings.Replace(annotationRegistrationURL(t, s, path, "Reviewer"), "/_annotations/v1/", "/_annotations/v2/", 1)
			status, state := annotationJSON(t, s, "GET", endpoint, nil, nil)
			if status != 200 {
				t.Fatal(status, state)
			}
			pageURL := strings.Replace(endpoint, "/_annotations/v2/", "/", 1)
			status, _, data := responseAsset(t, s, "GET", pageURL)
			if status != 200 {
				t.Fatal(status)
			}
			page := parseHTTPDocument(t, data)
			var mapped int
			for _, node := range nodes(page, "li") {
				if attr(node, "data-hp-footnote-label") == "a" {
					mapped++
					if len(nodes(node, "ul")) == 0 || len(nodes(node, "pre")) != 1 || !strings.Contains(textOf(node), "literal") {
						t.Fatal("footnote markup changed", textOf(node))
					}
					if strings.Contains(textOf(node), "HPPREVIEW") {
						t.Fatal("conversion token visible")
					}
				}
			}
			if mapped != 1 {
				t.Fatalf("mapped footnotes=%d body=%s", mapped, textOf(nodes(page, "main")[0]))
			}
		})
	}
}

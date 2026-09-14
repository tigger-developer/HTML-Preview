// ABOUTME: Exercises attributed annotation persistence through real service HTTP.
// ABOUTME: Verifies source preservation, read-only registration and current footnote values.
package preview

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tigger-developer/HTML-Preview/internal/annotation"
	"golang.org/x/net/html"
)

func annotationRegistrationURL(t *testing.T, s *runningTestService, path string, name any) string {
	t.Helper()
	data, err := json.Marshal(map[string]any{"paths": []string{path}, "settings": map[string]any{}, "format_contract": 1, "display_name": name})
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, "http://control/v1/annotation-previews", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	response, status, err := controlResponse(s.control, req)
	if err != nil || status != 200 {
		t.Fatalf("annotation registration status=%d error=%v body=%s", status, err, response)
	}
	var result registrationResponse
	if err := json.Unmarshal(response, &result); err != nil || len(result.Results) != 1 || result.Results[0].URL == "" {
		t.Fatalf("annotation registration returned no page: %s, %v", response, err)
	}
	return s.origin + "/_annotations/v2/" + strings.TrimPrefix(result.Results[0].URL, s.origin+"/")
}

func TestRT007_1_ReaderSelectionSurvivesAnnotationRefresh(t *testing.T) {
	s := startTestService(t, NativeHost())
	path := source(t, s.root, "reader.md", "# Heading\n\nLiteral **emphasis**.\n")
	endpoint := annotationRegistrationURL(t, s, path, "Reviewer")
	pageURL := s.origin + "/" + strings.TrimPrefix(endpoint, s.origin+"/_annotations/v2/") + "?htmlpreview-format=markdown_strict"
	status, data, err := testHTTPBody(s.client, pageURL)
	if err != nil || status != 200 {
		t.Fatal(status, err)
	}
	dom, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	var metadata map[string]any
	for n := range dom.Descendants() {
		if n.Type == html.ElementNode && attribute(n, "id") == "hp-annotation-data" {
			if err := json.Unmarshal([]byte(contentText(n)), &metadata); err != nil {
				t.Fatal(err)
			}
		}
	}
	if metadata == nil {
		t.Fatal("genuine Markdown variant has no annotation metadata")
	}
	for _, key := range []string{"endpoint", "page_url"} {
		u, err := url.Parse(metadata[key].(string))
		if err != nil || u.Query().Get("htmlpreview-format") != "markdown_strict" {
			t.Fatalf("%s lost its reader: %v", key, metadata[key])
		}
	}
	if status, state := annotationJSON(t, s, "GET", metadata["endpoint"].(string), nil, nil); status != 200 || state["writable"] != true {
		t.Fatalf("selected reader state=%d %#v", status, state)
	}
}

func annotationJSON(t *testing.T, s *runningTestService, method, target string, value any, headers map[string]string) (int, map[string]any) {
	t.Helper()
	var data []byte
	var err error
	if value != nil {
		data, err = json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
	}
	req, err := http.NewRequest(method, target, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if value != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(resp.Body)
	closeErr := resp.Body.Close()
	if readErr != nil || closeErr != nil {
		t.Fatalf("read=%v close=%v", readErr, closeErr)
	}
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("status=%d non-JSON annotation response: %s", resp.StatusCode, body)
	}
	return resp.StatusCode, result
}

func TestRT007_1_AnnotationRegistrationAndReadOnlyCompatibility(t *testing.T) {
	s := startTestService(t, NativeHost())
	path := source(t, s.root, "review.org", "* Review\nA passage.\n")
	endpoint := annotationRegistrationURL(t, s, path, "Fixture Reviewer")
	status, state := annotationJSON(t, s, "GET", endpoint, nil, nil)
	if status != 200 || state["write_token"] == "" || state["writable"] != true {
		t.Fatalf("annotation state: %d %#v", status, state)
	}
	readURL := s.register(t, "old.org", "* Old client\n")
	oldEndpoint := s.origin + "/_annotations/v2/" + strings.TrimPrefix(readURL, s.origin+"/")
	status, state = annotationJSON(t, s, "GET", oldEndpoint, nil, nil)
	if status != 200 || state["writable"] != false || state["write_token"] != nil {
		t.Fatalf("old registration acquired write authority: %d %#v", status, state)
	}
	for _, name := range []any{nil, ""} {
		endpoint = annotationRegistrationURL(t, s, path, name)
		status, state = annotationJSON(t, s, "GET", endpoint, nil, nil)
		if status != 200 || state["writable"] != false || state["write_token"] != nil {
			t.Fatalf("missing identity acquired write authority: %d %#v", status, state)
		}
	}
}

func TestRT007_3_CurrentFootnotesPreserveAuthoredSource(t *testing.T) {
	s := startTestService(t, NativeHost())
	for _, format := range []string{"org", "md"} {
		t.Run(format, func(t *testing.T) {
			original := []byte("\xef\xbb\xbfA preserved paragraph.\r\n\r\nNo final newline")
			path := source(t, s.root, "review."+format, string(original))
			endpoint := annotationRegistrationURL(t, s, path, "Taḋg")
			status, state := annotationJSON(t, s, "GET", endpoint, nil, nil)
			if status != 200 {
				t.Fatalf("state: %d %#v", status, state)
			}
			headers := map[string]string{"Origin": s.origin, "X-HTMLPreview-Annotation-Token": state["write_token"].(string), "X-HTMLPreview-Composer-Token": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}
			event := map[string]any{"operation_id": "10000000-0000-4000-8000-000000000001", "annotation_id": "20000000-0000-4000-8000-000000000001", "composer_id": "30000000-0000-4000-8000-000000000001", "sequence": 1, "revision": state["revision"], "source_revision": state["source_revision"], "body_revision": state["body_revision"], "action": "upsert", "label": "tadg-001", "target": map[string]any{"type": "point", "position": 1, "run": "A preserved paragraph.", "run_offset": 1}, "text": "Check <script> and -->, #+end_comment safely."}
			status, saved := annotationJSON(t, s, "POST", endpoint, event, headers)
			if status != 201 {
				t.Fatalf("save: %d %#v", status, saved)
			}
			// #nosec G304 -- Path is allocated by this test inside its temporary root.
			first, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(annotation.Parse(first, map[string]string{"org": "org", "md": "markdown"}[format]).Source, original) || bytes.Equal(first, original) {
				t.Fatalf("existing prefix changed: %v", err)
			}
			status, retry := annotationJSON(t, s, "POST", endpoint, event, headers)
			if status != 200 || retry["sequence"] != float64(1) {
				t.Fatalf("retry: %d %#v", status, retry)
			}
			// #nosec G304 -- Path is allocated by this test inside its temporary root.
			afterRetry, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(first, afterRetry) {
				t.Fatal("identical retry appended again")
			}
			event["operation_id"], event["sequence"], event["text"] = "10000000-0000-4000-8000-000000000002", 2, "Second draft"
			status, saved = annotationJSON(t, s, "POST", endpoint, event, headers)
			if status != 201 {
				t.Fatalf("second draft: %d %#v", status, saved)
			}
			event["operation_id"], event["sequence"], event["action"] = "10000000-0000-4000-8000-000000000003", 3, "close"
			status, saved = annotationJSON(t, s, "POST", endpoint, event, headers)
			if status != 201 || saved["closed"] != true {
				t.Fatalf("close: %d %#v", status, saved)
			}
			event["operation_id"], event["sequence"], event["action"] = "10000000-0000-4000-8000-000000000004", 4, "upsert"
			status, _ = annotationJSON(t, s, "POST", endpoint, event, headers)
			if status != 409 {
				t.Fatalf("closed comment accepted a revision: %d", status)
			}
			status, projected := annotationJSON(t, s, "GET", endpoint, nil, nil)
			if status != 200 || projected["source_revision"] != state["source_revision"] {
				t.Fatalf("annotation changed authored revision: %d %#v", status, projected)
			}
			events := projected["comments"].([]any)
			if len(events) != 1 || events[0].(map[string]any)["author"] != "Taḋg" || events[0].(map[string]any)["text"] != "Second draft" {
				t.Fatalf("projection=%#v", events)
			}
			pageURL := s.origin + "/" + strings.TrimPrefix(endpoint, s.origin+"/_annotations/v2/")
			status, page, err := testHTTPBody(s.client, pageURL)
			if err != nil || status != 200 || bytes.Contains(page, []byte("htmlpreview-annotation-event:v1")) {
				t.Fatal("owned frames leaked into rendered document")
			}
			if _, err := os.Stat(filepath.Join(s.root, "review."+format+"-annotations.org")); !os.IsNotExist(err) {
				t.Fatal("writable source used sidecar")
			}
		})
	}
}

func TestRT007_11_PollsShareSnapshotButWritesReread(t *testing.T) {
	s := startTestService(t, NativeHost())
	path := source(t, s.root, "poll.org", "* Before\nOriginal text.\n")
	endpoint := annotationRegistrationURL(t, s, path, "Reviewer")
	status, state := annotationJSON(t, s, "GET", endpoint, nil, nil)
	if status != 200 {
		t.Fatal(state)
	}
	if err := os.WriteFile(path, []byte("* After\nChanged text.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	status, cached := annotationJSON(t, s, "GET", endpoint, nil, nil)
	if status != 200 || cached["source_revision"] != state["source_revision"] {
		t.Fatal("immediate poll did not share the one-second snapshot")
	}
	headers := map[string]string{"Origin": s.origin, "X-HTMLPreview-Annotation-Token": state["write_token"].(string), "X-HTMLPreview-Composer-Token": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}
	event := map[string]any{"operation_id": "10000000-0000-4000-8000-000000000001", "annotation_id": "20000000-0000-4000-8000-000000000001", "composer_id": "30000000-0000-4000-8000-000000000001", "sequence": 1, "revision": state["revision"], "source_revision": state["source_revision"], "body_revision": state["body_revision"], "action": "upsert", "label": "reviewer-001", "target": map[string]any{"type": "point", "position": 8, "run": "Original text.", "run_offset": 1}, "text": "Must not use cached source"}
	status, response := annotationJSON(t, s, "POST", endpoint, event, headers)
	if status != 409 || response["error"] != "stale_source" {
		t.Fatalf("stale write=%d %#v", status, response)
	}
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline.C:
			t.Fatal("source poll never refreshed")
		case <-tick.C:
			status, current := annotationJSON(t, s, "GET", endpoint, nil, nil)
			if status == 200 && current["source_revision"] != state["source_revision"] {
				return
			}
		}
	}
}

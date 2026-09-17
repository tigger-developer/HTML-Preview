// ABOUTME: Checks push updates and private diagnostics through the real HTTP boundary.
// ABOUTME: Uses filesystem edits and stream responses without browser automation.
package preview

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestDocumentEvents(t *testing.T) {
	s := startTestService(t, NativeHost())
	path := source(t, s.root, "events.md", "# Heading\nOriginal\n")
	endpoint := annotationRegistrationURL(t, s, path, "Reviewer")
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint+"?events=1", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Error(err)
		}
	}()
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("not SSE: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	reader := bufio.NewReader(resp.Body)
	if line, err := reader.ReadString('\n'); err != nil || line != "retry: 1000\n" {
		t.Fatalf("reconnect must fit within the browser grace period: %q, %v", line, err)
	}
	event := func() {
		t.Helper()
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				t.Fatal(err)
			}
			if line == "data: {}\n" {
				return
			}
		}
	}
	event()
	if err := os.WriteFile(path, []byte("# Heading\nChanged\n"), 0600); err != nil {
		t.Fatal(err)
	}
	event()
	replacement := source(t, s.root, "replacement", "# Heading\nReplaced\n")
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	event()
	source(t, s.root, "events.md-annotations.org", "sidecar change")
	event()
	cancel()
	if _, err := io.Copy(io.Discard, resp.Body); err == nil {
		t.Fatal("cancelled stream unexpectedly remained readable")
	}
}

func TestAnnotationFailureLogging(t *testing.T) {
	var logs bytes.Buffer
	s := startObservedTestService(t, NativeHost(), &logs)
	path := source(t, s.root, "private-name.md", "# Secret text\n")
	endpoint := annotationRegistrationURL(t, s, path, "Private Author")
	status, _ := annotationJSON(t, s, "POST", endpoint, map[string]string{"secret": "Private Comment"}, nil)
	if status != 403 {
		t.Fatalf("status=%d", status)
	}
	if code := s.stop(); code != 0 {
		t.Fatal(code)
	}
	got := logs.String()
	if !strings.Contains(got, "status=403") || !strings.Contains(got, "write_refused") {
		t.Fatalf("missing failure diagnostic: %s", got)
	}
	for _, secret := range []string{endpoint, "private-name.md", "Secret text", "Private Comment", "Private Author"} {
		if strings.Contains(got, secret) {
			t.Fatalf("log disclosed %q", secret)
		}
	}
}

func TestDocumentEventsRefuseUnauthorizedRequests(t *testing.T) {
	s := startTestService(t, NativeHost())
	path := source(t, s.root, "events.md", "# Heading\n")
	endpoint := annotationRegistrationURL(t, s, path, "Reviewer")
	for _, tc := range []struct {
		target, origin string
		status         int
	}{
		{endpoint + "?events=1", "https://foreign.example", 403},
		{endpoint + "?events=1&events=1", "", 400},
		{endpoint + "?events=invalid", "", 400},
		{s.origin + "/_annotations/v2/unknown/events.md?events=1", "", 404},
	} {
		req, err := http.NewRequestWithContext(t.Context(), "GET", tc.target, nil)
		if err != nil {
			t.Fatal(err)
		}
		if tc.origin != "" {
			req.Header.Set("Origin", tc.origin)
		}
		resp, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if err := resp.Body.Close(); err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != tc.status || resp.Header.Get("Content-Type") == "text/event-stream" {
			t.Fatalf("refusal=%d want=%d", resp.StatusCode, tc.status)
		}
	}
}

// ABOUTME: Exercises strict private requests and public response limits over HTTP.
// ABOUTME: Asserts documented boundary outcomes using synthetic documents.
package preview

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestRT006_3_MultipleAuthorityHeaders(t *testing.T) {
	s := startTestService(t, NativeHost())
	url := s.register(t, "entry.html", "<p>PRIVATE HEADER FIXTURE</p>")
	for _, tc := range []struct {
		name   string
		values []string
	}{
		{"Origin", []string{s.origin, "https://foreign.example"}},
		{"Origin", []string{"", "null"}},
		{"Sec-Fetch-Site", []string{"same-origin", "cross-site"}},
	} {
		t.Run(tc.name+strings.Join(tc.values, ","), func(t *testing.T) {
			req, err := http.NewRequestWithContext(t.Context(), "GET", url, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header[tc.name] = tc.values
			resp, err := s.client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := resp.Body.Close(); err != nil {
					t.Error(err)
				}
			}()
			body, err := io.ReadAll(resp.Body)
			if err != nil || resp.StatusCode != 403 || bytes.Contains(body, []byte("PRIVATE HEADER FIXTURE")) {
				t.Fatalf("ambiguous authority status=%d err=%v", resp.StatusCode, err)
			}
		})
	}
}

func TestRT006_2_StrictControlRequests(t *testing.T) {
	s := startTestService(t, NativeHost())
	file := source(t, s.root, "entry.html", "<p>Valid source</p>")
	path, err := json.Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	valid := `{"paths":[` + string(path) + `],"settings":{},"format_contract":1}`
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"valid", valid, 200},
		{"at body bound", valid + strings.Repeat(" ", 65536-len(valid)), 200},
		{"over body bound", valid + strings.Repeat(" ", 65537-len(valid)), 413},
		{"duplicate", strings.Replace(valid, `"settings":{}`, `"settings":{},"settings":{}`, 1), 400},
		{"unknown", strings.Replace(valid, `"settings":{}`, `"settings":{"arguments":"--unsafe"}`, 1), 400},
		{"null", strings.Replace(valid, `"settings":{}`, `"settings":null`, 1), 400},
		{"trailing", valid + `{}`, 400},
		{"version", strings.Replace(valid, `"format_contract":1`, `"format_contract":2`, 1), 409},
		{"depth zero", strings.Replace(valid, `"settings":{}`, `"settings":{"toc_depth":0}`, 1), 400},
		{"depth seven", strings.Replace(valid, `"settings":{}`, `"settings":{"toc_depth":7}`, 1), 400},
		{"wrong type", strings.Replace(valid, `"settings":{}`, `"settings":{"toc":"true"}`, 1), 400},
		{"source ceiling", strings.Replace(valid, `"settings":{}`, `"settings":{"source_bytes":10485761}`, 1), 400},
		{"output ceiling", strings.Replace(valid, `"settings":{}`, `"settings":{"output_bytes":52428801}`, 1), 400},
		{"deadline ceiling", strings.Replace(valid, `"settings":{}`, `"settings":{"deadline":"61s"}`, 1), 400},
		{"narrow relative", strings.Replace(valid, `"settings":{}`, `"settings":{"root":"relative"}`, 1), 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(t.Context(), "POST", "http://control/v1/previews", bytes.NewBufferString(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			body, status, err := controlResponse(s.control, req)
			if err != nil || status != tc.status || !json.Valid(body) {
				t.Fatalf("control status=%d want=%d err=%v", status, tc.status, err)
			}
		})
	}
}

func TestRT006_8_SourceAndOutputLimits(t *testing.T) {
	s := startTestService(t, NativeHost())
	for _, tc := range []struct {
		name, body string
		settings   map[string]any
		status     int
	}{
		{"at-source.html", strings.Repeat("x", 64), map[string]any{"source_bytes": 64}, 200},
		{"over-source.html", strings.Repeat("x", 65), map[string]any{"source_bytes": 64}, 413},
		{"tiny-output.html", "literal", map[string]any{"output_bytes": 1}, 413},
		{"invalid-utf8.html", string([]byte{255}), map[string]any{}, 422},
	} {
		t.Run(tc.name, func(t *testing.T) {
			url := s.registerSettings(t, tc.name, tc.body, tc.settings)
			code, headers, body := responseAsset(t, s, "GET", url)
			if code != tc.status {
				t.Fatalf("status=%d want=%d", code, tc.status)
			}
			headCode, headHeaders, headBody := responseAsset(t, s, "HEAD", url)
			if headCode != code || len(headBody) != 0 || headHeaders.Get("Content-Length") != headers.Get("Content-Length") || headHeaders.Get("Content-Type") != headers.Get("Content-Type") {
				t.Fatal("HEAD does not match the GET representation")
			}
			if tc.status != 200 && bytes.Contains(body, []byte(tc.body)) {
				t.Fatal("error disclosed denied source bytes")
			}
		})
	}
}

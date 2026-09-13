// ABOUTME: Tests new-client compatibility with a service lacking annotation registration.
// ABOUTME: Exercises real HTTP and strict old-schema decoding without browser activation.
package preview

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRT007_1_OnlyMissingExtensionDowngrades(t *testing.T) {
	for _, code := range []int{404, 400, 503, 200} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			paths := []string{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				paths = append(paths, r.URL.Path)
				if r.URL.Path == "/v1/annotation-previews" {
					writeControl(w, code, map[string]string{"error": "fixture"})
					return
				}
				if r.URL.Path != "/v1/previews" {
					http.NotFound(w, r)
					return
				}
				data, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
					return
				}
				var request previewRegistration
				if decodeControl(data, &request) != nil || len(request.Paths) != 1 {
					t.Error("fallback sent fields outside the old request schema")
					writeControl(w, 400, map[string]string{"error": "bad_schema"})
					return
				}
				writeControl(w, 200, registrationResponse{Protocol: 1, Results: []registrationResult{{Path: request.Paths[0], Error: "outside_root"}}})
			}))
			defer server.Close()
			transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
			}}
			defer transport.CloseIdleConnections()
			client := &http.Client{Transport: transport, Timeout: time.Second}
			var messages bytes.Buffer
			log := &console{out: io.Discard, diagnostics: &messages}
			cfg, err := settings([]string{"USER=Reviewer"})
			if err != nil {
				t.Fatal(err)
			}
			_, fallback, err := prepareHTTP(t.Context(), []sourceContext{{logical: "/synthetic.org"}}, cfg, &serviceConnection{client: client}, log)
			if code == 404 {
				if err != nil || len(paths) != 2 || len(fallback) != 1 || strings.Count(messages.String(), "annotations unavailable") != 1 {
					t.Fatalf("fallback=%v err=%v paths=%v messages=%s", fallback, err, paths, messages.String())
				}
			} else if err == nil || len(paths) != 1 || len(fallback) != 0 {
				t.Fatalf("non-404 downgraded: err=%v paths=%v", err, paths)
			}
		})
	}
}

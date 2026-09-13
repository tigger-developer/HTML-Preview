// ABOUTME: Exercises interrupted entry preflight against bounded loopback HTTP fixtures.
// ABOUTME: Distinguishes confirmed service absence from conversion and protocol failures.
package preview

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRT006_1_InterruptedPreflightRechecksAvailability(t *testing.T) {
	for _, outcome := range []string{"unavailable", "healthy", "foreign-instance", "malformed", "conversion-error"} {
		t.Run(outcome, func(t *testing.T) {
			var probes atomic.Int64
			var listener *httptest.Server
			listener = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/_health" {
					probes.Add(1)
					w.Header().Set("Content-Type", "application/json")
					body := `{"protocol":1,"instance":"fixture","ready":true}`
					if outcome == "foreign-instance" {
						body = `{"protocol":1,"instance":"other","ready":true}`
					} else if outcome == "malformed" {
						body = "invalid JSON"
					}
					if _, err := io.WriteString(w, body); err != nil {
						t.Error(err)
					}
					return
				}
				if outcome == "conversion-error" {
					w.WriteHeader(http.StatusUnprocessableEntity)
					return
				}
				conn, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				if outcome == "unavailable" {
					if err := listener.Listener.Close(); err != nil {
						t.Error(err)
					}
				}
				if err := conn.Close(); err != nil {
					t.Error(err)
				}
			}))
			defer listener.Close()
			file := source(t, t.TempDir(), "selected.data", "# Explicit reader")
			src, err := identify(file, "")
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := settings(nil)
			if err != nil {
				t.Fatal(err)
			}
			cfg.from = "markdown"
			status := serviceStatus{Protocol: 1, Instance: "fixture", Origin: listener.URL, Ready: true, FormatContract: 1}
			reply := registrationResponse{Protocol: 1, Results: []registrationResult{{Path: file, URL: listener.URL + "/token/selected.data?htmlpreview-format=markdown"}}}
			prepared := httpPreviews{urls: make(map[string]string)}
			var diagnostics strings.Builder
			got, fallback, err := preflightHTTP(context.Background(), []sourceContext{src}, cfg, status, reply, prepared, &console{out: io.Discard, diagnostics: &diagnostics})
			if outcome == "unavailable" {
				if err != nil || len(fallback) != 1 || fallback[0].logical != file || len(got.urls) != 0 || got.invalid {
					t.Fatalf("confirmed absence must preserve input for file conversion: fallback=%d error=%v", len(fallback), err)
				}
			} else if outcome == "conversion-error" {
				if err != nil || !got.invalid || len(fallback) != 0 || probes.Load() != 0 {
					t.Fatal("conversion failure must stay a per-input failure without availability retry")
				}
			} else if err == nil || len(fallback) != 0 || probes.Load() != 1 {
				t.Fatalf("%s must fail after exactly one recheck: probes=%d error=%v", outcome, probes.Load(), err)
			}
		})
	}
}

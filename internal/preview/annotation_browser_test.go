//go:build browser

// ABOUTME: Runs the packaged browser modules in the native default browser.
// ABOUTME: Collects bounded deterministic assertions through a one-run loopback capability.
package preview

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"golang.org/x/net/html"
)

//go:embed testdata/annotations-browser.js
var annotationBrowserTests []byte

//go:embed testdata/annotations-browser.html
var annotationBrowserPage []byte

//go:embed testdata/annotations-browser-bootstrap.js
var annotationBrowserBootstrap []byte

func TestRT007_Browser(t *testing.T) {
	s := startTestService(t, NativeHost())
	target := s.register(t, "browser.org", "* Browser fixture\nA *formatted* passage with ~code~.\n")
	status, rendered, err := testHTTPBody(s.client, target)
	if err != nil || status != 200 {
		t.Fatal("rendered browser fixture unavailable")
	}
	dom, err := html.Parse(bytes.NewReader(rendered))
	if err != nil {
		t.Fatal(err)
	}
	var module []byte
	for n := range dom.Descendants() {
		if n.Type == html.ElementNode && n.Data == "script" && attribute(n, "type") == "module" {
			module = append(module, []byte(contentText(n))...)
		}
	}
	if len(module) == 0 {
		t.Fatal("rendered page contains no packaged browser module")
	}
	token, err := randomCapability()
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	origin := "http://" + listener.Addr().String()
	prefix := "/" + token + "/"
	type result struct {
		Browser  string   `json:"browser"`
		Passed   int      `json:"passed"`
		Failures []string `json:"failures"`
	}
	results := make(chan result, 1)
	textFixtures, err := os.ReadFile("../../testdata/annotation-text.json")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != listener.Addr().String() {
			http.Error(w, "Refused", 403)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		switch r.URL.Path {
		case prefix + "text-fixtures.json":
			w.Header().Set("Content-Type", "application/json")
			if _, err := w.Write(textFixtures); err != nil {
				return
			}
		case prefix:
			t.Logf("native browser fetched test page: %s", r.UserAgent())
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if _, err := w.Write(annotationBrowserPage); err != nil {
				return
			}
		case prefix + "bootstrap.js":
			t.Log("browser requested test bootstrap")
			w.Header().Set("Content-Type", "text/javascript")
			if _, err := w.Write(annotationBrowserBootstrap); err != nil {
				return
			}
		case prefix + "module.js":
			t.Log("browser requested packaged module")
			w.Header().Set("Content-Type", "text/javascript")
			if _, err := w.Write(module); err != nil {
				return
			}
		case prefix + "tests.js":
			t.Log("browser requested test assertions")
			w.Header().Set("Content-Type", "text/javascript")
			if _, err := w.Write(annotationBrowserTests); err != nil {
				return
			}
		case prefix + "results":
			t.Log("browser returned test results")
			if r.Method != "POST" || r.Header.Get("Origin") != origin {
				http.Error(w, "Refused", 403)
				return
			}
			data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 65536))
			if err != nil {
				http.Error(w, "Limit", 413)
				return
			}
			var response result
			if json.Unmarshal(data, &response) != nil {
				http.Error(w, "Invalid", 400)
				return
			}
			select {
			case results <- response:
				w.WriteHeader(204)
			default:
				http.Error(w, "Already received", 409)
			}
		default:
			http.NotFound(w, r)
		}
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second}
	stopped := make(chan error, 1)
	go func() { stopped <- server.Serve(listener) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			t.Error(err)
		}
		if err := <-stopped; err != http.ErrServerClosed {
			t.Error(err)
		}
	})
	desktop, err := selectDesktop(NativeHost(), os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()
	if err := desktop.open(ctx, origin+prefix); err != nil {
		t.Fatalf("native browser handoff: %v", err)
	}
	select {
	case report := <-results:
		t.Logf("native browser: %s; assertions passed: %d", report.Browser, report.Passed)
		if len(report.Failures) > 0 || report.Passed == 0 {
			t.Fatalf("browser assertions failed: %v", report.Failures)
		}
	case <-ctx.Done():
		t.Fatal("browser assertions did not return within 120 seconds; browser qualification unavailable")
	}
}

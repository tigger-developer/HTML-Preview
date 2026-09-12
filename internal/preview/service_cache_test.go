// ABOUTME: Verifies cache reuse and bounded conversion work through real service HTTP.
// ABOUTME: Counts and stalls only the existing Pandoc process boundary for deterministic concurrency.
package preview

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type runningTestService struct {
	client, control *http.Client
	root, origin    string
	stop            func() int
}

func startTestService(t *testing.T, host Host, extraRoots ...string) *runningTestService {
	t.Helper()
	root := t.TempDir()
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := os.MkdirTemp("", "hp-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(runtime); err != nil {
			t.Error(err)
		}
	})
	cfg, err := settings(nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan int, 1)
	go func() {
		done <- runService(ctx, cfg, serviceConfig{roots: append([]string{canonical}, extraRoots...), runtime: runtime}, host, &console{out: io.Discard, diagnostics: io.Discard})
	}()
	var stopOnce sync.Once
	stopCode := 0
	stop := func() int {
		stopOnce.Do(func() {
			cancel()
			select {
			case code := <-done:
				stopCode = code
			case <-time.After(7 * time.Second):
				stopCode = -1
			}
		})
		return stopCode
	}
	t.Cleanup(func() {
		if code := stop(); code != 0 {
			t.Errorf("service shutdown status=%d", code)
		}
	})
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	deadline := time.NewTimer(8 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case <-deadline.C:
			t.Fatal("service did not become ready")
		case <-tick.C:
			client, err := controlClient(runtime)
			if err != nil {
				continue
			}
			status, err := serviceDiscovery(t.Context(), client)
			if err != nil {
				client.CloseIdleConnections()
				continue
			}
			public := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 10 * time.Second}
			t.Cleanup(client.CloseIdleConnections)
			t.Cleanup(public.CloseIdleConnections)
			return &runningTestService{client: public, control: client, root: root, origin: status.Origin, stop: stop}
		}
	}
}

func TestRT006_8_EvictionKeepsCapability(t *testing.T) {
	host := NativeHost()
	actual := host.Temp
	var count atomic.Int64
	host.Temp = func() (string, error) { count.Add(1); return actual() }
	s := startTestService(t, host)
	first := s.register(t, "first.html", "<h1>First</h1>")
	if status, _, err := testHTTPBody(s.client, first); err != nil || status != 200 {
		t.Fatal("first page unavailable")
	}
	for i := range 50 {
		target := s.register(t, fmt.Sprintf("page-%d.html", i), "<h1>Other</h1>")
		if status, _, err := testHTTPBody(s.client, target); err != nil || status != 200 {
			t.Fatal("cache filler unavailable")
		}
	}
	if count.Load() != 51 {
		t.Fatalf("unexpected conversion count %d", count.Load())
	}
	status, body, err := testHTTPBody(s.client, first)
	if err != nil || status != 200 || !bytes.Contains(body, []byte("First")) || count.Load() != 52 {
		t.Fatal("eviction did not rerender through the original capability")
	}
}

func TestRT006_9_ShutdownWaitsForConversionCleanup(t *testing.T) {
	host := NativeHost()
	actual := host.Execute
	remove := host.Remove
	entered := make(chan struct{}, 1)
	cleaning := make(chan struct{}, 1)
	release := make(chan struct{})
	host.Execute = func(ctx context.Context, cmd Command) ([]byte, error) {
		if len(cmd.Args) > 0 && strings.HasPrefix(cmd.Args[0], "--defaults=") {
			entered <- struct{}{}
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return actual(ctx, cmd)
	}
	host.Remove = func(path string) error { cleaning <- struct{}{}; <-release; return remove(path) }
	s := startTestService(t, host)
	target := s.register(t, "entry.md", "# Entry")
	response := make(chan struct{})
	go func() { _, _, _ = testHTTPBody(s.client, target); close(response) }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("conversion did not start")
	}
	stopped := make(chan int, 1)
	go func() { stopped <- s.stop() }()
	select {
	case <-cleaning:
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled conversion did not clean up")
	}
	select {
	case code := <-stopped:
		close(release)
		t.Fatalf("service returned %d before conversion cleanup", code)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if code := <-stopped; code != 0 {
		t.Fatalf("shutdown failed: %d", code)
	}
	<-response
}

func (s *runningTestService) register(t *testing.T, name, body string) string {
	t.Helper()
	return s.registerSettings(t, name, body, map[string]any{})
}

func (s *runningTestService) registerSettings(t *testing.T, name, body string, settings map[string]any) string {
	t.Helper()
	file := filepath.Join(s.root, name)
	if err := os.WriteFile(file, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]any{"paths": []string{file}, "settings": settings, "format_contract": 1})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(t.Context(), "POST", "http://control/v1/previews", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	data, status, err := controlResponse(s.control, request)
	var reply struct{ Results []struct{ URL string } }
	if err != nil || status != 200 || json.Unmarshal(data, &reply) != nil || len(reply.Results) != 1 || reply.Results[0].URL == "" {
		t.Fatalf("register: %v %d %s", err, status, data)
	}
	return reply.Results[0].URL
}

func testHTTPBody(client *http.Client, target string) (int, []byte, error) {
	resp, err := client.Get(target)
	if err != nil {
		return 0, nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, 50*1024*1024+1))
	closeErr := resp.Body.Close()
	if readErr != nil {
		return resp.StatusCode, nil, readErr
	}
	return resp.StatusCode, data, closeErr
}

func TestRT006_8_CacheRehashesSourceAndStylesheet(t *testing.T) {
	host := NativeHost()
	actual := host.Execute
	var conversions atomic.Int64
	host.Execute = func(ctx context.Context, cmd Command) ([]byte, error) {
		if len(cmd.Args) > 0 && strings.HasPrefix(cmd.Args[0], "--defaults=") {
			conversions.Add(1)
		}
		return actual(ctx, cmd)
	}
	s := startTestService(t, host)
	target := s.register(t, "entry.md", "# First value\n")
	for range 2 {
		status, body, err := testHTTPBody(s.client, target)
		if err != nil || status != 200 || !bytes.Contains(body, []byte("First value")) {
			t.Fatalf("read preview: %d %v", status, err)
		}
	}
	if got := conversions.Load(); got != 1 {
		t.Fatalf("unchanged page converted %d times; expected one", got)
	}
	file := filepath.Join(s.root, "entry.md")
	st, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(file, []byte("# Other value\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Chtimes(file, st.ModTime(), st.ModTime()); err != nil {
		t.Fatal(err)
	}
	status, body, err := testHTTPBody(s.client, target)
	if err != nil || status != 200 || !bytes.Contains(body, []byte("Other value")) || conversions.Load() != 2 {
		t.Fatal("same-size, same-mtime edit did not invalidate cached source")
	}
	css := filepath.Join(s.root, "style.css")
	if err = os.WriteFile(css, []byte("h1{color:red}"), 0600); err != nil {
		t.Fatal(err)
	}
	native := s.register(t, "native.html", `<link rel="stylesheet" href="style.css"><h1>Styled source</h1>`)
	_, body, err = testHTTPBody(s.client, native)
	if err != nil || !bytes.Contains(body, []byte("color:red")) {
		t.Fatal("native stylesheet absent")
	}
	st, err = os.Stat(css)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(css, []byte("h1{color:tan}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Chtimes(css, st.ModTime(), st.ModTime()); err != nil {
		t.Fatal(err)
	}
	_, body, err = testHTTPBody(s.client, native)
	if err != nil || !bytes.Contains(body, []byte("color:tan")) || bytes.Contains(body, []byte("color:red")) {
		t.Fatal("same-size stylesheet edit did not invalidate cached output")
	}
}

func TestRT006_8_BoundsDistinctWorkersAndSharesDuplicates(t *testing.T) {
	host := NativeHost()
	actual := host.Execute
	entered := make(chan struct{}, 8)
	release := make(chan struct{})
	var count atomic.Int64
	host.Execute = func(ctx context.Context, cmd Command) ([]byte, error) {
		if len(cmd.Args) > 0 && strings.HasPrefix(cmd.Args[0], "--defaults=") {
			count.Add(1)
			entered <- struct{}{}
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return actual(ctx, cmd)
	}
	s := startTestService(t, host)
	first := s.register(t, "one.md", "# One")
	second := s.register(t, "two.md", "# Two")
	third := s.register(t, "three.md", "# Three")
	type result struct {
		status int
		err    error
	}
	results := make(chan result, 3)
	get := func(target string) { status, _, err := testHTTPBody(s.client, target); results <- result{status, err} }
	go get(first)
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("first converter did not start")
	}
	go get(second)
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("second converter did not start")
	}
	go get(first)
	status, _, err := testHTTPBody(s.client, third)
	close(release)
	if err != nil || status != 503 {
		t.Errorf("third distinct request should be rejected with 503: status=%d err=%v", status, err)
	}
	for range 3 {
		r := <-results
		if r.err != nil || r.status != 200 {
			t.Errorf("shared or admitted request: %d %v", r.status, r.err)
		}
	}
	if count.Load() != 2 {
		t.Fatalf("expected two converters shared by three clients, got %d", count.Load())
	}
}

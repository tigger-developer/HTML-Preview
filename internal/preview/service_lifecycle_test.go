// ABOUTME: Verifies ownership and cleanup failures through service requests and startup.
// ABOUTME: Uses only synthetic sources and the existing filesystem/process boundaries.
package preview

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRT006_9_CleanupFailureReportsOwnedPath(t *testing.T) {
	host := NativeHost()
	remove := host.Remove
	var remaining string
	host.Remove = func(path string) error {
		remaining = path
		return errors.New("synthetic cleanup refusal")
	}
	var diagnostics bytes.Buffer
	s := startObservedTestService(t, host, &diagnostics)
	target := s.register(t, "entry.html", "<h1>Cleanup fixture</h1>")
	status, _, _ := responseAsset(t, s, "GET", target)
	if remaining == "" {
		t.Fatal("conversion did not attempt owned cleanup")
	}
	t.Cleanup(func() {
		if err := remove(remaining); err != nil {
			t.Error(err)
		}
	})
	if status != 503 || !strings.Contains(diagnostics.String(), remaining) || !strings.Contains(diagnostics.String(), "cleanup") {
		t.Fatalf("cleanup failure must refuse publication and identify remaining owned path: status=%d diagnostic=%q", status, diagnostics.String())
	}
	if strings.Contains(diagnostics.String(), target) || strings.Contains(diagnostics.String(), "Cleanup fixture") {
		t.Fatal("cleanup diagnostic disclosed token URL or source content")
	}
}

func TestRT006_9_SecondInstancePreservesOwner(t *testing.T) {
	s := startTestService(t, NativeHost())
	target := s.register(t, "owner.html", "<h1>Original owner</h1>")
	lock, err := os.Stat(filepath.Join(s.runtime, "instance.lock"))
	if err != nil {
		t.Fatal(err)
	}
	sentinel := source(t, s.runtime, "unrelated", "retained")
	cfg, err := settings(nil)
	if err != nil {
		t.Fatal(err)
	}
	var diagnostics bytes.Buffer
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	code := runService(ctx, cfg, serviceConfig{runtime: s.runtime}, NativeHost(), &console{out: io.Discard, diagnostics: &diagnostics})
	current, err := os.Stat(filepath.Join(s.runtime, "instance.lock"))
	if code != 1 || err != nil || !os.SameFile(lock, current) {
		t.Fatal("second instance replaced the active lock")
	}
	status, _, body := responseAsset(t, s, "GET", target)
	if status != 200 || !bytes.Contains(body, []byte("Original owner")) {
		t.Fatal("second instance disrupted original service")
	}
	if code := s.stop(); code != 0 {
		t.Fatalf("owner shutdown status=%d", code)
	}
	// #nosec G304 -- Reads the exact synthetic sentinel created in this test's private runtime.
	if data, err := os.ReadFile(sentinel); err != nil || string(data) != "retained" {
		t.Fatal("shutdown changed unrelated runtime state")
	}
}

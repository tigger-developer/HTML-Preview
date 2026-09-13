// ABOUTME: Verifies ownership and cleanup failures through service requests and startup.
// ABOUTME: Uses only synthetic sources and the existing filesystem/process boundaries.
package preview

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRT006_9_UnsafeRuntimeAndStaleSocket(t *testing.T) {
	for _, kind := range []string{"public directory", "symlink directory", "symlink lock", "unrelated socket path", "stale socket"} {
		t.Run(kind, func(t *testing.T) {
			path, err := os.MkdirTemp("", "hp-runtime-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := os.RemoveAll(path); err != nil {
					t.Error(err)
				}
			})
			runtimePath := path
			sentinel := source(t, path, "sentinel", "retain this content")
			switch kind {
			case "public directory":
				// #nosec G302 -- Deliberately invalid permissions on this test-owned runtime.
				if err := os.Chmod(path, 0755); err != nil {
					t.Fatal(err)
				}
			case "symlink directory":
				runtimePath = filepath.Join(path, "alias")
				if err := os.Symlink(path, runtimePath); err != nil {
					t.Fatal(err)
				}
			case "symlink lock":
				if err := os.Symlink(sentinel, filepath.Join(path, "instance.lock")); err != nil {
					t.Fatal(err)
				}
			case "unrelated socket path":
				source(t, path, "control.sock", "not a socket")
			case "stale socket":
				listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: filepath.Join(path, "control.sock"), Net: "unix"})
				if err != nil {
					t.Fatal(err)
				}
				listener.SetUnlinkOnClose(false)
				if err := listener.Close(); err != nil {
					t.Fatal(err)
				}
			}
			runtime, err := openServiceRuntime(runtimePath)
			if kind == "stale socket" {
				if err != nil {
					t.Fatal(err)
				}
				if err := runtime.close(); err != nil {
					t.Fatal(err)
				}
				if _, err := os.Lstat(filepath.Join(path, "control.sock")); !os.IsNotExist(err) {
					t.Fatal("owned socket retained after clean shutdown")
				}
			} else {
				if runtime != nil {
					if err := runtime.close(); err != nil {
						t.Error(err)
					}
				}
				if err == nil {
					t.Fatal("unsafe runtime accepted")
				}
			}
			// #nosec G304 -- Reads the exact test-owned sentinel to check it survived startup.
			data, err := os.ReadFile(sentinel)
			if err != nil || string(data) != "retain this content" {
				t.Fatal("startup altered unrelated state")
			}
		})
	}
}

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

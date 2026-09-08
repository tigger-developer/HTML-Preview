// ABOUTME: Tests cancellation, retention, publication, and cleanup failures.
// ABOUTME: Child-process doubles force actual bounded execution failures.
package preview

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRT001_8_PublicationTransaction(t *testing.T) {
	root := t.TempDir()
	a := source(t, root, "a.md", "A")
	b := source(t, root, "b.md", "B")
	r := run(t, root, []string{"PREVIEW_TEST_FAULT=publication"}, a, b)
	if r.code != 1 || len(r.opens) != 0 || !strings.Contains(r.stderr, "publication") {
		t.Fatalf("publication status=%d opens=%d diagnostic=%s", r.code, len(r.opens), r.stderr)
	}
}

func TestRT001_8_PublicationExpiryThroughCommand(t *testing.T) {
	root := t.TempDir()
	p := source(t, root, "doc.md", "text")
	r := run(t, root, []string{"PREVIEW_TEST_FAULT=publication-timeout"}, p)
	if r.code != 1 || len(r.opens) != 0 || !strings.Contains(r.stderr, "publication failed: context deadline exceeded") || r.cleanedAt == 0 {
		t.Fatal("publication expiry did not abort and clean the command")
	}
}

func TestRT001_15_PreflightBoundsBeforeAllocation(t *testing.T) {
	root := t.TempDir()
	p := source(t, root, "doc.md", "text")
	for _, fault := range []string{"preflight-stall", "preflight-flood"} {
		tmp := t.TempDir()
		start := time.Now()
		r := run(t, root, []string{"PREVIEW_TEST_FAULT=" + fault, "TMPDIR=" + tmp}, p)
		entries, err := os.ReadDir(tmp)
		if err != nil {
			t.Fatal(err)
		}
		if r.code != 1 || len(r.opens) != 0 || len(entries) != 0 || time.Since(start) > 8*time.Second {
			t.Fatalf("preflight %s escaped its bounds", fault)
		}
	}
}

func TestRT001_12_SignalDuringConversion(t *testing.T) {
	root := t.TempDir()
	p := source(t, root, "doc.md", "text")
	r := run(t, root, []string{"PREVIEW_TEST_FAULT=converter-signal"}, p)
	if r.code != 1 || len(r.opens) != 0 || r.cleanedAt == 0 {
		t.Fatal("conversion cancellation did not fail and clean")
	}
}

func TestRT001_11_ExactByteBudgets(t *testing.T) {
	root := t.TempDir()
	a := source(t, root, "a.md", "four")
	b := source(t, root, "b.md", "four")
	for _, tc := range []struct {
		setting     string
		files       []string
		code, pages int
	}{
		{"HTMLPREVIEW_MAX_SOURCE_BYTES=5", []string{a}, 0, 1},
		{"HTMLPREVIEW_MAX_SOURCE_BYTES=4", []string{a}, 0, 1},
		{"HTMLPREVIEW_MAX_SOURCE_BYTES=3", []string{a}, 1, 0},
		{"HTMLPREVIEW_MAX_TOTAL_SOURCE_BYTES=9", []string{a, b}, 0, 2},
		{"HTMLPREVIEW_MAX_TOTAL_SOURCE_BYTES=8", []string{a, b}, 0, 2},
		{"HTMLPREVIEW_MAX_TOTAL_SOURCE_BYTES=7", []string{a, b}, 1, 1},
	} {
		r := run(t, root, []string{tc.setting}, tc.files...)
		if r.code != tc.code || len(r.pages) != tc.pages {
			t.Fatalf("%s: status %d pages %d", tc.setting, r.code, len(r.pages))
		}
	}
	s := &session{path: t.TempDir(), host: NativeHost(), cfg: config{outputBytes: 4}}
	if err := s.write("exact", []byte("four")); err != nil {
		t.Fatal(err)
	}
	if err := s.write("excess", []byte("x")); err == nil {
		t.Fatal("write exceeded exact output allowance")
	}
	if _, err := os.Stat(filepath.Join(s.path, "excess")); !os.IsNotExist(err) {
		t.Fatal("excess output reached disk")
	}
}

func TestRT001_11_ConverterBounds(t *testing.T) {
	root := t.TempDir()
	p := source(t, root, "doc.md", "text")
	for _, fault := range []string{"converter-stall", "converter-flood"} {
		start := time.Now()
		r := run(t, root, []string{"PREVIEW_TEST_FAULT=" + fault, "HTMLPREVIEW_DEADLINE=100ms", "HTMLPREVIEW_MAX_OUTPUT_BYTES=100000"}, p)
		if r.code != 1 || len(r.opens) != 0 || time.Since(start) > 3*time.Second {
			t.Fatalf("unbounded converter %s", fault)
		}
	}
}

func TestRT001_12_RetentionAndCleanup(t *testing.T) {
	root := t.TempDir()
	p := source(t, root, "doc.md", "text")
	r := run(t, root, []string{"HTMLPREVIEW_GRACE=250ms"}, p)
	success(t, r, 1)
	grace := time.Duration(r.cleanedAt - r.opens[len(r.opens)-1].CompletedAt)
	if grace < 225*time.Millisecond || grace > 2*time.Second {
		t.Fatalf("handoff-to-cleanup grace %s outside 250ms with scheduling tolerance", grace)
	}
	r = run(t, root, []string{"HTMLPREVIEW_MODE=read"}, p)
	success(t, r, 1)
	if !strings.Contains(r.stderr, "reading session") || !r.liveRead {
		t.Fatal("retention not observed")
	}
	r = run(t, root, []string{"PREVIEW_TEST_FAULT=cleanup"}, p)
	if r.code != 1 || !strings.Contains(r.stderr, "remaining directory") {
		t.Fatal("cleanup failure missing")
	}
}

func TestRT001_2_ConcurrentSessions(t *testing.T) {
	sharedTemp := t.TempDir()
	for _, name := range []string{"one", "two"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			p := source(t, root, "doc.md", name)
			r := run(t, root, []string{"TMPDIR=" + sharedTemp}, p)
			success(t, r, 1)
			if !strings.Contains(textOf(nodes(r.pages[0], "main")[0]), name) {
				t.Fatal("sessions mixed")
			}
		})
	}
}

func TestRT001_15_PandocPreflight(t *testing.T) {
	root := t.TempDir()
	p := source(t, root, "doc.md", "test")
	for _, version := range []string{"3.8.3", "3.9.0.1", "3.10.0", "4.0"} {
		r := run(t, root, []string{"PREVIEW_TEST_PANDOC_VERSION=" + version}, p)
		if r.code != 1 || len(r.opens) != 0 || !strings.Contains(r.stderr, "compatibility") {
			t.Errorf("version %s not rejected", version)
		}
	}
	for _, version := range []string{"3.9.0.2", "3.9.1"} {
		r := run(t, root, []string{"PREVIEW_TEST_PANDOC_VERSION=" + version}, p)
		success(t, r, 1)
	}
	for _, args := range [][]string{{"--help"}, {"--version"}} {
		r := run(t, root, []string{"PREVIEW_TEST_PANDOC_VERSION=invalid"}, args...)
		success(t, r, 0)
	}
}

func TestRT001_17_PrivateWSLWorkspace(t *testing.T) {
	for _, mapped := range []string{`C:\Temp\htmlpreview-test`, `\\wsl.localhost\Other\tmp\preview`} {
		host := NativeHost()
		host.Execute = func(context.Context, Command) ([]byte, error) { return []byte(mapped), nil }
		d := &desktop{host: host, wsl: true, distro: "Ubuntu", translator: "wslpath", urls: make(map[string]string)}
		if err := d.prepare(context.Background(), "/tmp/preview"); err == nil {
			t.Fatalf("unsuitable output accepted %s", mapped)
		}
	}
}

func TestRT001_11_PartialOutputBudget(t *testing.T) {
	root := t.TempDir()
	a := source(t, root, "a.md", "first")
	b := source(t, root, "b.md", "second")
	r := run(t, root, []string{"HTMLPREVIEW_MAX_OUTPUT_BYTES=3000000"}, a, b)
	if r.code != 1 || len(r.opens) != 1 || len(r.pages) != 1 {
		t.Fatalf("completed page should survive output exhaustion: code=%d opens=%d pages=%d stderr=%s", r.code, len(r.opens), len(r.pages), r.stderr)
	}
}

func TestRT001_12_PublicationDeadline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := &session{path: t.TempDir(), host: NativeHost(), cfg: config{outputBytes: 10000000}, log: &console{out: &bytes.Buffer{}, diagnostics: &bytes.Buffer{}}}
	s.desktop = &desktop{host: s.host}
	s.pages = []*page{{name: "0001.html", ready: true}}
	if err := s.publish(ctx); err == nil {
		t.Fatal("expired publication allowed")
	}
	if _, err := os.Stat(filepath.Join(s.path, "0001.html")); !os.IsNotExist(err) {
		t.Fatal("expired publication wrote output")
	}
}

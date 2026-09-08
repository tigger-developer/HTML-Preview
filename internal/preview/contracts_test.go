// ABOUTME: Covers contract boundaries across configuration, references, and hosts.
// ABOUTME: Uses synthetic local data and explicit adapters for unavailable platforms.
package preview

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"golang.org/x/net/html"
)

func TestRT001_17_WSLCommandGraph(t *testing.T) {
	root := t.TempDir()
	a := source(t, root, "a.md", "[next](b.org#target) [forged](file://wsl.localhost/Ubuntu/secret.md)\n\n![bad](bad%3Aname.png)\n")
	source(t, root, "b.org", "* Target\n:PROPERTIES:\n:CUSTOM_ID: target\n:END:\n")
	r := run(t, root, []string{"PREVIEW_TEST_PLATFORM=wsl", "WSL_DISTRO_NAME=Ubuntu", "HTMLPREVIEW_LINKS=1"}, a)
	success(t, r, 2)
	if len(r.opens) != 1 || filepath.Base(r.opens[0].Tool) != "powershell.exe" || !strings.HasPrefix(r.stdout, "file://wsl.localhost/Ubuntu/") {
		t.Fatal("WSL handoff was not the Windows bridge")
	}
	for _, link := range nodes(r.pages[0], "a") {
		switch textOf(link) {
		case "next":
			if !strings.Contains(attr(link, "href"), "0002.html#target") {
				t.Fatal("linked Windows preview destination")
			}
		case "forged":
			if attr(link, "href") != "" {
				t.Fatal("source claimed the generated WSL authority")
			}
		}
	}
	if len(nodes(r.pages[0], "img")) != 0 || !strings.Contains(r.stderr, "cannot be represented") {
		t.Fatal("unrepresentable original image was not inactive")
	}
	if attr(nodes(r.pages[0], "h1")[0], "data-hp-source") != a {
		t.Fatal("WSL copy value ceased to be the Linux source path")
	}
	r = run(t, root, []string{"PREVIEW_TEST_PLATFORM=wsl", "WSL_DISTRO_NAME=Ubuntu", "HTMLPREVIEW_LINKS=1", "PREVIEW_TEST_FAULT=page-translation"}, a)
	if r.code != 1 || len(r.opens) != 0 {
		t.Fatal("failed generated translation opened partial output")
	}
}

func TestRT001_9_AllSettingBoundaries(t *testing.T) {
	root := t.TempDir()
	p := source(t, root, "doc.md", "text")
	valid := []string{"HTMLPREVIEW_LINKS=0", "HTMLPREVIEW_LINKS=1", "HTMLPREVIEW_MODE=quick", "HTMLPREVIEW_MODE=read", "HTMLPREVIEW_GRACE=100ms", "HTMLPREVIEW_GRACE=1h", "HTMLPREVIEW_DEADLINE=100ms", "HTMLPREVIEW_DEADLINE=10m", "HTMLPREVIEW_MAX_FILES=1", "HTMLPREVIEW_MAX_FILES=500", "HTMLPREVIEW_MAX_DEPTH=0", "HTMLPREVIEW_MAX_DEPTH=10", "HTMLPREVIEW_MAX_SOURCE_BYTES=1", "HTMLPREVIEW_MAX_SOURCE_BYTES=10485760", "HTMLPREVIEW_MAX_TOTAL_SOURCE_BYTES=1", "HTMLPREVIEW_MAX_TOTAL_SOURCE_BYTES=52428800", "HTMLPREVIEW_MAX_OUTPUT_BYTES=1", "HTMLPREVIEW_MAX_OUTPUT_BYTES=104857600", "HTMLPREVIEW_ROOT=" + root}
	for _, setting := range valid {
		r := run(t, root, []string{setting, "PATH="}, p)
		if r.code != 1 || !strings.Contains(r.stderr, "Pandoc") {
			t.Errorf("valid boundary %s rejected as %d: %s", setting, r.code, r.stderr)
		}
	}
	invalid := []string{"HTMLPREVIEW_MAX_FILES=0", "HTMLPREVIEW_MAX_DEPTH=11", "HTMLPREVIEW_MAX_SOURCE_BYTES=10485761", "HTMLPREVIEW_MAX_TOTAL_SOURCE_BYTES=52428801", "HTMLPREVIEW_MAX_OUTPUT_BYTES=104857601", "HTMLPREVIEW_GRACE=1h1ms", "HTMLPREVIEW_DEADLINE=99ms", "HTMLPREVIEW_MAX_FILES=1.5", "HTMLPREVIEW_ROOT=missing"}
	for _, setting := range invalid {
		r := run(t, root, []string{setting}, p)
		if r.code != 2 {
			t.Errorf("invalid boundary %s status=%d", setting, r.code)
		}
	}
	r := run(t, root, []string{"HTMLPREVIEW_LINKS=1", "HTMLPREVIEW_MODE=quick"}, p)
	if r.code != 2 {
		t.Fatal("linked quick mode accepted")
	}
}

func TestRT001_10_SymlinkOriginsAndEscape(t *testing.T) {
	root := t.TempDir()
	a := source(t, root, "a.md", "[one](one/alias.md) [two](two/alias.md) [escape](escape.md)\n")
	target := source(t, root, "shared/doc.md", "![relative](image.png)\n")
	for _, dir := range []string{"one", "two"} {
		source(t, root, dir+"/image.png", "image")
		if err := os.Symlink(target, filepath.Join(root, dir, "alias.md")); err != nil {
			t.Fatal(err)
		}
	}
	outside := source(t, t.TempDir(), "outside.md", "outside")
	if err := os.Symlink(outside, filepath.Join(root, "escape.md")); err != nil {
		t.Fatal(err)
	}
	r := run(t, root, []string{"HTMLPREVIEW_LINKS=1"}, a)
	success(t, r, 3)
	for i, dir := range []string{"one", "two"} {
		if !strings.Contains(attr(nodes(r.pages[i+1], "img")[0], "src"), "/"+dir+"/image.png") {
			t.Fatal("symlink contexts merged or used the canonical parent")
		}
	}
	if !strings.Contains(r.stderr, "outside") {
		t.Fatal("escape not diagnosed")
	}
}

func TestRT001_15_FIFOIsRejected(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "input.org")
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	r := run(t, root, nil, path)
	if r.code != 1 || len(r.opens) != 0 || !strings.Contains(r.stderr, "regular file") || time.Since(start) > 3*time.Second {
		t.Fatal("FIFO input blocked or was accepted")
	}
}

func TestRT001_13_NoSourceURLFetch(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(http.StatusNoContent) }))
	defer server.Close()
	root := t.TempDir()
	p := source(t, root, "doc.md", "---\ncss: "+server.URL+"/style.css\nheader-includes: '<script src=\""+server.URL+"/script.js\"></script>'\n---\n\n![remote]("+server.URL+"/image.png)\n\n[ordinary]("+server.URL+"/doc.md)\n")
	r := run(t, root, nil, p)
	success(t, r, 1)
	if requests.Load() != 0 {
		t.Fatalf("conversion fetched %d source URLs", requests.Load())
	}
}

func TestRT001_11_DeviceBoundary(t *testing.T) {
	root := t.TempDir()
	a := source(t, root, "a.md", "[b](b.md)")
	source(t, root, "b.md", "B")
	cfg, err := settings(nil)
	if err != nil {
		t.Fatal(err)
	}
	host := NativeHost()
	src, err := identify(a, "")
	if err != nil {
		t.Fatal(err)
	}
	src.device = "deliberately-different-device"
	s := &session{cfg: cfg, host: host, log: &console{out: os.Stdout, diagnostics: os.Stderr}, byKey: make(map[string]*page)}
	p := s.admit(src)
	p.dom, err = html.Parse(strings.NewReader(`<a href="b.md">B</a>`))
	if err != nil {
		t.Fatal(err)
	}
	s.discover(p)
	if len(s.pages) != 1 {
		t.Fatal("different device admitted")
	}
}

func TestRT001_15_HelperOutputCap(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	host := NativeHost()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := helper(ctx, host, exe, []string{"--process-flood"}, nil); err == nil {
		t.Fatal("unbounded helper output accepted")
	}
}

// ABOUTME: Exercises the command and generated pages across process boundaries.
// ABOUTME: Normal conversion uses real Pandoc; desktop handoff is controlled.
package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"golang.org/x/net/html"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// RT001.1 begins at the executable boundary, before renderer implementation.
func TestPublicCommandBootstrap(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "htmlpreview")
	// #nosec G204 -- Fixed Go build arguments target this test-owned temporary binary.
	cmd := exec.Command("go", "build", "-o", binary, "../cmd/htmlpreview")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("htmlpreview executable is unavailable: %v\n%s", err, out)
	}
	// #nosec G204 -- Executes the binary this test has just built.
	cmd = exec.Command(binary, "--help")
	cmd.Env = append(os.Environ(), "HTMLPREVIEW_UNKNOWN=ignored-for-help")
	if out, err := cmd.CombinedOutput(); err != nil || len(out) == 0 {
		t.Fatalf("expected command information before preview side effects: %v\n%s", err, out)
	}
}

func TestRT001_14_PrefixInstall(t *testing.T) {
	stage := t.TempDir()
	for i := 0; i < 2; i++ {
		// #nosec G204 -- The fixed install target receives a test-owned temporary DESTDIR as one argument.
		cmd := exec.Command("make", "install", "PREFIX=/htmlpreview-test", "DESTDIR="+stage)
		cmd.Dir = ".."
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("staged installation: %v\n%s", err, output)
		}
	}
	binary := filepath.Join(stage, "htmlpreview-test/bin/htmlpreview")
	symlink := filepath.Join(t.TempDir(), "htmlpreview")
	if err := os.Symlink(binary, symlink); err != nil {
		t.Fatal(err)
	}
	// #nosec G204 -- This symlink points to the test-installed executable.
	cmd := exec.Command(symlink, "--version")
	cmd.Dir = t.TempDir()
	if output, err := cmd.CombinedOutput(); err != nil || len(output) == 0 {
		t.Fatalf("installed symlink invocation: %v %s", err, output)
	}
	for _, path := range []string{"LICENSE", "THIRD_PARTY_NOTICES.md", "asap-OFL.txt", "iosevka-custom-OFL.md"} {
		// #nosec G304 -- Reads a fixed licence name within the test-owned installation prefix.
		data, err := os.ReadFile(filepath.Join(stage, "htmlpreview-test/share/licenses/htmlpreview", path))
		if err != nil || len(data) == 0 {
			t.Fatalf("installed licence %s: %v", path, err)
		}
	}
	t.Run("render from installed entry point", func(t *testing.T) {
		overlay := desktopOverlay(t)
		// #nosec G204 -- Installs under the same test-owned prefix with only the desktop boundary overlaid.
		cmd := exec.Command("make", "install", "PREFIX=/htmlpreview-test", "DESTDIR="+stage)
		cmd.Dir = ".."
		cmd.Env = append(os.Environ(), "GOFLAGS=-overlay="+overlay)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("instrumented install: %v %s", err, out)
		}
		root := t.TempDir()
		capture := t.TempDir()
		for _, name := range []string{"Taḋg  & <notes>.md", "notes.org"} {
			if err := os.WriteFile(filepath.Join(root, name), []byte("Hello Taḋg"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		// #nosec G204 -- Executes the installed command through its test-owned symlink.
		cmd = exec.Command(symlink, filepath.Join(root, "Taḋg  & <notes>.md"), filepath.Join(root, "notes.org"))
		cmd.Dir = t.TempDir()
		cmd.Env = append(os.Environ(), "HTMLPREVIEW_GRACE=100ms", "HTMLPREVIEW_LINKS=0", "HTMLPREVIEW_MODE=quick", "PREVIEW_TEST_CAPTURE="+capture)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("installed rendering: %v %s", err, out)
		}
		for i, name := range []string{"Taḋg  & <notes>.md", "notes.org"} {
			// #nosec G304 -- Reads numbered captures written by the controlled desktop into t.TempDir.
			data, err := os.ReadFile(filepath.Join(capture, fmt.Sprintf("%04d.html", i+1)))
			if err != nil {
				t.Fatal(err)
			}
			doc, err := html.Parse(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for n := range doc.Descendants() {
				for _, a := range n.Attr {
					if a.Key == "data-hp-source" && a.Val == filepath.Join(root, name) {
						found = true
					}
				}
			}
			if !found || strings.Count(string(data), "data:font/woff2;base64,") != 6 || !strings.Contains(string(data), "SIL OPEN FONT LICENSE") {
				t.Fatal("installed renderer lost its source or embedded assets")
			}
		}
	})
}

// Go's file overlay replaces only NativeHost at the public entry point. All
// argument handling, rendering, packaging, and asset reads remain production code.
func desktopOverlay(t *testing.T) string {
	t.Helper()
	mainPath, err := filepath.Abs("../cmd/htmlpreview/main.go")
	if err != nil {
		t.Fatal(err)
	}
	// #nosec G304 -- This exact repository entry point is the subject of the integration test.
	data, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), "preview.NativeHost()") != 1 {
		t.Fatal("entry-point seam changed; review the overlay")
	}
	fixture, err := os.ReadFile("testdata/desktop.go.txt")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	replacement := filepath.Join(root, "main.go")
	adapter := filepath.Join(root, "desktop.go")
	if err := os.WriteFile(replacement, []byte(strings.Replace(string(data), "preview.NativeHost()", "testDesktopHost()", 1)), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(adapter, fixture, 0600); err != nil {
		t.Fatal(err)
	}
	mapping, err := json.Marshal(map[string]any{"Replace": map[string]string{mainPath: replacement, filepath.Join(filepath.Dir(mainPath), "desktop_integration.go"): adapter}})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "overlay.json")
	if err := os.WriteFile(path, mapping, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

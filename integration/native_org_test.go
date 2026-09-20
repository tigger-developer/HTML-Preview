// ABOUTME: Verifies that the packaged executable supplies its native Org worker through a symlink.
// ABOUTME: Runs outside the checkout with no compiler or helper on the executable search path.
package integration

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tigger-developer/HTML-Preview/internal/orgconvert"
)

func TestRT011_1_PackagedNativeWorker(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "htmlpreview")
	// #nosec G204 -- Fixed build target writes only this test's temporary executable.
	build := exec.Command("go", "build", "-o", binary, "../cmd/htmlpreview")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	link := filepath.Join(t.TempDir(), "preview-link")
	if err := os.Symlink(binary, link); err != nil {
		t.Fatal(err)
	}
	input, err := json.Marshal(orgconvert.Request{Version: 1, Text: "* Native title\n\nA paragraph.\n", Token: strings.Repeat("a", 32), TOC: true, TOCDepth: 3, InputBytes: 100 << 20, OutputBytes: 50 << 20, MemoryBytes: 512 << 20})
	if err != nil {
		t.Fatal(err)
	}
	// #nosec G204 -- Invokes the test-built executable through its test-owned symlink.
	cmd := exec.Command(link, "--internal-org-convert")
	cmd.Dir = t.TempDir()
	cmd.Env = []string{"PATH="}
	cmd.Stdin = bytes.NewReader(input)
	out, err := cmd.CombinedOutput()
	if err != nil || !bytes.Contains(out, []byte("Native title")) || !bytes.Contains(out, []byte("A paragraph.")) {
		t.Fatalf("packaged native worker unavailable: %v %s", err, out)
	}
}

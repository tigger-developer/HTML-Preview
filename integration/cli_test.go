// ABOUTME: Exercises the command and generated pages across process boundaries.
// ABOUTME: Normal conversion uses real Pandoc; desktop handoff is controlled.
package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// RT001.1 begins at the executable boundary, before renderer implementation.
func TestRT001_1_ExplicitInputs(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "htmlpreview")
	cmd := exec.Command("go", "build", "-o", binary, "../cmd/htmlpreview")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("htmlpreview executable is unavailable: %v\n%s", err, out)
	}
	cmd = exec.Command(binary, "--help")
	cmd.Env = append(os.Environ(), "HTMLPREVIEW_UNKNOWN=ignored-for-help")
	if out, err := cmd.CombinedOutput(); err != nil || len(out) == 0 {
		t.Fatalf("expected command information before preview side effects: %v\n%s", err, out)
	}
}

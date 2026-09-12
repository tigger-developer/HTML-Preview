// ABOUTME: Verifies service startup rejects malformed or unsafe root configuration.
// ABOUTME: Asserts failures before any listener can be published.
package integration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRT006_2_RejectInvalidConfiguration(t *testing.T) {
	binary := serviceBinary(t)
	for _, tc := range []struct {
		name, value string
		mode        os.FileMode
	}{
		{"unknown key", "version: 1\nserve: {roots: [], bind: '0.0.0.0'}\n", 0600},
		{"duplicate key", "version: 1\nversion: 1\nserve: {roots: []}\n", 0600},
		{"alias", "version: 1\nserve: {roots: &r []}\nextra: *r\n", 0600},
		{"explicit tag", "version: !!int 1\nserve: {roots: []}\n", 0600},
		{"extra document", "version: 1\nserve: {roots: []}\n---\nversion: 1\n", 0600},
		{"relative root", "version: 1\nserve: {roots: ['relative']}\n", 0600},
		{"wrong version type", "version: '1'\nserve: {roots: []}\n", 0600},
		{"group writable", "version: 1\nserve: {roots: []}\n", 0620},
		{"oversize", strings.Repeat("#", 65537), 0600},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "config.yaml")
			if err := os.WriteFile(path, []byte(tc.value), 0600); err != nil {
				t.Fatal(err)
			}
			// #nosec G302 -- This owned fixture deliberately exercises rejected permissions.
			if err := os.Chmod(path, tc.mode); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			// #nosec G204 -- Test-built executable consumes a synthetic invalid configuration.
			cmd := exec.CommandContext(ctx, binary, "--serve")
			cmd.Env = append(os.Environ(), "HTMLPREVIEW_CONFIG="+path, "HTMLPREVIEW_RUNTIME_DIR="+filepath.Join(root, "runtime"))
			output, err := cmd.CombinedOutput()
			if err == nil || cmd.ProcessState.ExitCode() != 2 || !strings.Contains(string(output), "configuration") {
				t.Fatalf("configuration failure must exit 2: %v\n%s", err, output)
			}
			if _, err := os.Lstat(filepath.Join(root, "runtime", "control.sock")); !os.IsNotExist(err) {
				t.Fatal("invalid config published a control socket")
			}
		})
	}
}

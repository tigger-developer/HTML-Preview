// ABOUTME: Checks service root configuration through its bounded file-reading boundary.
// ABOUTME: Covers empty grants, canonical aliases, root count and malformed structures.
package preview

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRT006_2_RootConfigurationBoundaries(t *testing.T) {
	root := t.TempDir()
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	file := source(t, root, "plain.txt", "a file is not a directory")
	rootList := func(count int) string {
		return "version: 1\nserve:\n  roots: [" + strings.TrimSuffix(strings.Repeat(fmt.Sprintf("%q,", root), count), ",") + "]\n"
	}
	for _, tc := range []struct {
		name, body string
		valid      bool
		grants     int
	}{
		{"no serve", "version: 1\n", true, 0},
		{"no roots", "version: 1\nserve: {}\n", true, 0},
		{"empty roots", rootList(0), true, 0},
		{"32 roots", rootList(32), true, 1},
		{"33 roots", rootList(33), false, 0},
		{"canonical aliases", fmt.Sprintf("version: 1\nserve: {roots: [%q, %q]}\n", root, alias), true, 1},
		{"missing root", fmt.Sprintf("version: 1\nserve: {roots: [%q]}\n", filepath.Join(root, "missing")), false, 0},
		{"file root", fmt.Sprintf("version: 1\nserve: {roots: [%q]}\n", file), false, 0},
		{"null serve", "version: 1\nserve: null\n", false, 0},
		{"null roots", "version: 1\nserve: {roots: null}\n", false, 0},
		{"non-string root", "version: 1\nserve: {roots: [12]}\n", false, 0},
		{"syntax error", "version: [\n", false, 0},
		{"non-mapping document", "[]\n", false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config{configPath: source(t, t.TempDir(), "config.yaml", tc.body), runtimePath: filepath.Join(root, "runtime")}
			settings, err := serviceSettings(cfg)
			if (err == nil) != tc.valid {
				t.Fatalf("configuration valid=%t err=%v", tc.valid, err)
			}
			if tc.valid && (len(settings.roots) != tc.grants || (tc.grants == 1 && settings.roots[0] != canonical)) {
				t.Fatalf("unexpected canonical grants: %v", settings.roots)
			}
		})
	}
}

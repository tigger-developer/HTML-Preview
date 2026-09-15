// ABOUTME: Checks TMPDIR root expansion through the configuration file boundary.
// ABOUTME: Rejects unset, relative and unsupported substitutions without widening grants.
package preview

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestTMPDIRConfiguration(t *testing.T) {
	dir := t.TempDir()
	child := filepath.Join(dir, "docs")
	if err := os.Mkdir(child, 0700); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, root, env, want string }{
		{"variable", "$TMPDIR", dir, dir},
		{"braces", "${TMPDIR}", dir, dir},
		{"child", "$TMPDIR/docs", dir, child},
		{"trailing slash", "${TMPDIR}/docs", dir + "/", child},
		{"unset", "$TMPDIR", "", ""},
		{"relative value", "$TMPDIR", "relative", ""},
		{"unknown", "$OTHER", dir, ""},
		{"different name", "$TMPDIRECTORY", dir, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TMPDIR", tc.env)
			path := filepath.Join(dir, "config.yaml")
			if err := os.WriteFile(path, []byte(fmt.Sprintf("version: 1\nserve:\n  roots: [%q]\n", tc.root)), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := readServiceConfiguration(path)
			if tc.want == "" {
				if err == nil {
					t.Fatal("invalid expansion accepted")
				}
				return
			}
			want, e := filepath.EvalSymlinks(tc.want)
			if e != nil {
				t.Fatal(e)
			}
			if err != nil || len(cfg.roots) != 1 || cfg.roots[0] != want {
				t.Fatalf("roots=%v want=%q error=%v", cfg.roots, want, err)
			}
		})
	}
}

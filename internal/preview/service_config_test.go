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

func TestRT006_2_CurrentDirectoryConfiguration(t *testing.T) {
	cwd := t.TempDir()
	root := t.TempDir()
	t.Chdir(cwd)
	source(t, cwd, "config.yaml", fmt.Sprintf("version: 1\nserve: {roots: [%q]}\n", root))
	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := serviceSettings(config{runtimePath: filepath.Join(cwd, "runtime")})
	if err != nil || len(cfg.roots) != 1 || cfg.roots[0] != want {
		t.Fatalf("local configuration roots=%v want=%q err=%v", cfg.roots, want, err)
	}
}

func TestRT006_2_ConfigurationDiscovery(t *testing.T) {
	for _, name := range []string{"absent", "user", "local", "empty local", "invalid local", "dangling local", "unreadable local"} {
		t.Run(name, func(t *testing.T) {
			cwd, home, localRoot, userRoot := t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir()
			localConfig := filepath.Join(cwd, "config.yaml")
			userConfig := filepath.Join(home, ".config/htmlpreview/config.yaml")
			if name != "absent" {
				source(t, home, ".config/htmlpreview/config.yaml", fmt.Sprintf("version: 1\nserve: {roots: [%q]}\n", userRoot))
			}
			want, invalid := cwd, false
			switch name {
			case "user":
				want = userRoot
			case "local":
				want = localRoot
				source(t, cwd, "config.yaml", fmt.Sprintf("version: 1\nserve: {roots: [%q]}\n", localRoot))
			case "empty local":
				want = ""
				source(t, cwd, "config.yaml", "version: 1\nserve: {roots: []}\n")
				// A selected local file must not even parse a lower-priority file.
				if err := os.WriteFile(userConfig, []byte("invalid: ["), 0600); err != nil {
					t.Fatal(err)
				}
			case "invalid local":
				invalid = true
				source(t, cwd, "config.yaml", "version: [")
			case "dangling local":
				invalid = true
				if err := os.Symlink(filepath.Join(cwd, "missing"), localConfig); err != nil {
					t.Fatal(err)
				}
			case "unreadable local":
				if os.Geteuid() == 0 {
					t.Skip("root bypasses ordinary file read permissions")
				}
				invalid = true
				source(t, cwd, "config.yaml", "version: 1\nserve: {roots: []}\n")
				if err := os.Chmod(localConfig, 0000); err != nil {
					t.Fatal(err)
				}
			}
			roots, err := discoverServiceRoots(cwd, home)
			if invalid {
				if err == nil {
					t.Fatal("invalid selected config fell through")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if want == "" {
				if len(roots) != 0 {
					t.Fatalf("empty config granted %v", roots)
				}
				return
			}
			canonical, err := filepath.EvalSymlinks(want)
			if err != nil || len(roots) != 1 || roots[0] != canonical {
				t.Fatalf("selected roots=%v want=%q err=%v", roots, canonical, err)
			}
		})
	}
}

func TestRT006_2_ExplicitConfigurationPrecedence(t *testing.T) {
	cwd, root := t.TempDir(), t.TempDir()
	t.Chdir(cwd)
	source(t, cwd, "config.yaml", "invalid: [")
	explicit := source(t, t.TempDir(), "explicit.yaml", fmt.Sprintf("version: 1\nserve: {roots: [%q]}\n", root))
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := serviceSettings(config{configPath: explicit, runtimePath: filepath.Join(cwd, "runtime")})
	if err != nil || len(cfg.roots) != 1 || cfg.roots[0] != canonical {
		t.Fatalf("explicit override roots=%v err=%v", cfg.roots, err)
	}
	if _, err := serviceSettings(config{configPath: filepath.Join(cwd, "missing.yaml")}); err == nil {
		t.Fatal("missing explicit config fell through")
	}
}

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

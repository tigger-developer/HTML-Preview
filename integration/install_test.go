// ABOUTME: Checks source installation through the public make install target.
// ABOUTME: Stages all destinations and preserves conflicting user-owned paths.
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

// RT001.14 - Installation outside the checkout, default-link amendment.
func TestRT001_14_DefaultInstall(t *testing.T) {
	stage := t.TempDir()
	link := stagedDefaultLink(t, stage)
	target, err := filepath.Abs("../bin/htmlpreview")
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{nil, {"PREFIX="}} {
		out, err := stagedInstall(t, stage, args...)
		if err != nil {
			t.Fatalf("default installation: %v\n%s", err, out)
		}
		resolved, err := filepath.EvalSymlinks(target)
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.Readlink(link)
		if err != nil || got != resolved {
			t.Fatalf("expected default link %s -> %s; got %q, %v", link, resolved, got, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(stage, "usr/local/bin/htmlpreview")); !os.IsNotExist(err) {
		t.Fatalf("default installation must not create /usr/local binary: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// #nosec G204 -- Executes the link just installed beneath t.TempDir.
	cmd := exec.CommandContext(ctx, link, "--version")
	cmd.Dir = t.TempDir()
	if out, err := cmd.CombinedOutput(); err != nil || !strings.Contains(string(out), "htmlpreview ") {
		t.Fatalf("default installed link from unrelated cwd: %v\n%s", err, out)
	}
}

func TestRT001_14_DefaultInstallPreservesConflicts(t *testing.T) {
	for _, kind := range []string{"file", "directory", "live link", "dangling link"} {
		t.Run(kind, func(t *testing.T) {
			stage := t.TempDir()
			link := stagedDefaultLink(t, stage)
			if err := os.MkdirAll(filepath.Dir(link), 0700); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(t.TempDir(), "personal preview")
			sentinel := []byte("personal installation must survive\n")
			if kind == "file" {
				target = link
			}
			if kind == "directory" {
				if err := os.Mkdir(link, 0700); err != nil {
					t.Fatal(err)
				}
				target = filepath.Join(link, "sentinel")
			}
			if kind != "dangling link" {
				if err := os.WriteFile(target, sentinel, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if strings.HasSuffix(kind, "link") {
				if err := os.Symlink(target, link); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.Lstat(link)
			if err != nil {
				t.Fatal(err)
			}
			out, err := stagedInstall(t, stage)
			if err == nil || !strings.Contains(string(out), link) || !strings.Contains(string(out), "move") {
				t.Fatalf("expected actionable conflict failure for %s: %v\n%s", link, err, out)
			}
			after, err := os.Lstat(link)
			if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() {
				t.Fatalf("conflicting destination changed: %v", err)
			}
			if strings.HasSuffix(kind, "link") {
				got, err := os.Readlink(link)
				if err != nil || got != target {
					t.Fatalf("conflicting symlink changed to %q: %v", got, err)
				}
			}
			if kind == "dangling link" {
				if _, err := os.Lstat(target); !os.IsNotExist(err) {
					t.Fatalf("dangling target was created: %v", err)
				}
			} else if got, err := os.ReadFile(target); err != nil || string(got) != string(sentinel) {
				t.Fatalf("conflicting content changed: %q, %v", got, err)
			}
		})
	}
}

func TestRT001_14_InstallRejectsRelativePaths(t *testing.T) {
	for _, arg := range []string{"PREFIX=relative", "DESTDIR=relative"} {
		out, err := stagedInstall(t, t.TempDir(), arg)
		key, _, _ := strings.Cut(arg, "=")
		if err == nil || !strings.Contains(string(out), key+" must be absolute") {
			t.Fatalf("expected absolute-path validation for %s: %v\n%s", arg, err, out)
		}
	}
}

func TestRT001_14_PrefixInstallPreservesSymlinkTarget(t *testing.T) {
	stage := t.TempDir()
	link := filepath.Join(stage, "test-prefix/bin/htmlpreview")
	target := filepath.Join(t.TempDir(), "personal preview")
	sentinel := "personal executable\n"
	if err := os.MkdirAll(filepath.Dir(link), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(sentinel), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	out, err := stagedInstall(t, stage, "PREFIX=/test-prefix")
	if err == nil || !strings.Contains(string(out), link) {
		t.Fatalf("expected refusal to copy through %s: %v\n%s", link, err, out)
	}
	if got, err := os.Readlink(link); err != nil || got != target {
		t.Fatalf("prefix symlink changed: %q, %v", got, err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != sentinel {
		t.Fatalf("prefix copy changed personal executable: %v", err)
	}
}

func stagedDefaultLink(t *testing.T, stage string) string {
	t.Helper()
	userHome, err := os.UserHomeDir()
	if err != nil || !filepath.IsAbs(userHome) {
		t.Fatalf("resolve absolute home directory for staging: %q, %v", userHome, err)
	}
	return filepath.Join(stage, userHome, ".local/bin/htmlpreview")
}

func stagedInstall(t *testing.T, stage string, args ...string) ([]byte, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	// #nosec G204 -- Fixed make target; destinations are test-owned or deliberately rejected relative paths.
	cmd := exec.CommandContext(ctx, "make", append([]string{"install", "DESTDIR=" + stage}, args...)...)
	cmd.Dir = ".."
	for _, setting := range os.Environ() {
		key, _, _ := strings.Cut(setting, "=")
		switch key {
		case "PREFIX", "DESTDIR", "MAKEFLAGS", "MFLAGS", "MAKEOVERRIDES":
			continue
		}
		cmd.Env = append(cmd.Env, setting)
	}
	return cmd.CombinedOutput()
}

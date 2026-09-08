// ABOUTME: Implements reproducible builds, user-local linking and prefix installation.
// ABOUTME: Keeps operator values out of generated shell command strings.
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func main() {
	if err := task(os.Args[1:]); err != nil {
		if _, writeErr := fmt.Fprintln(os.Stderr, err); writeErr != nil {
			os.Exit(1)
		}
		os.Exit(1)
	}
}
func task(args []string) error {
	if len(args) != 1 {
		return errors.New("expected build, install, release, lint, or sync")
	}
	switch args[0] {
	case "build":
		return build("bin/htmlpreview", runtime.GOOS, runtime.GOARCH, version())
	case "install":
		return install()
	case "release":
		return release()
	case "sync":
		return synchronize()
	case "lint":
		return lint()
	default:
		return fmt.Errorf("unknown build task %q", args[0])
	}
}

func command(env []string, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}
func version() string {
	if v := os.Getenv("VERSION"); v != "" {
		return v
	}
	return "dev"
}
func build(path, goos, arch, version string) error {
	if goos != "darwin" && goos != "linux" {
		return fmt.Errorf("unsupported build platform %s", goos)
	}
	if arch != "amd64" && arch != "arm64" {
		return fmt.Errorf("unsupported build architecture %s", arch)
	}
	revision, err := exec.Command("git", "rev-parse", "--short=12", "HEAD").Output()
	if err != nil {
		return fmt.Errorf("build revision: %w", err)
	}
	// #nosec G301 -- Build directories contain distributable binaries, not preview documents.
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return command([]string{"CGO_ENABLED=0", "GOOS=" + goos, "GOARCH=" + arch}, "go", "build", "-trimpath", "-ldflags", "-X main.version="+version+" -X main.revision="+strings.TrimSpace(string(revision)), "-o", path, "./cmd/htmlpreview")
}

func licenceFiles() map[string]string {
	return map[string]string{"LICENSE": "LICENSE", "THIRD_PARTY_NOTICES.md": "THIRD_PARTY_NOTICES.md", "asap-OFL.txt": "assets/fonts/asap/OFL.txt", "iosevka-custom-OFL.md": "assets/fonts/iosevka-custom/OFL.md", "golang-x-net-LICENSE": "assets/licenses/golang-x-net-LICENSE", "bluemonday-LICENSE.md": "assets/licenses/bluemonday-LICENSE.md", "douceur-LICENSE": "assets/licenses/douceur-LICENSE", "gorilla-css-LICENSE": "assets/licenses/gorilla-css-LICENSE"}
}
func copyFile(from, to string, mode fs.FileMode) error {
	if info, err := os.Lstat(to); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to copy through symlink %q; move it aside and retry", to)
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("inspect copy destination %q: %w", to, err)
	}
	// #nosec G304 -- Callers select repository licence files or the binary just built.
	data, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	// #nosec G301 -- Prefix installation deliberately creates traversable public package directories.
	if err := os.MkdirAll(filepath.Dir(to), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(to, data, mode); err != nil {
		return err
	}
	return os.Chmod(to, mode)
}
func install() error {
	prefix := os.Getenv("PREFIX")
	if prefix != "" && !filepath.IsAbs(prefix) {
		return errors.New("PREFIX must be absolute")
	}
	stage := os.Getenv("DESTDIR")
	if stage != "" && !filepath.IsAbs(stage) {
		return errors.New("DESTDIR must be absolute")
	}
	if prefix == "" {
		return installLink(stage)
	}
	if err := build("bin/htmlpreview", runtime.GOOS, runtime.GOARCH, version()); err != nil {
		return err
	}
	root := filepath.Join(stage, prefix)
	if err := copyFile("bin/htmlpreview", filepath.Join(root, "bin/htmlpreview"), 0755); err != nil {
		return err
	}
	for name, from := range licenceFiles() {
		if err := copyFile(from, filepath.Join(root, "share/licenses/htmlpreview", name), 0644); err != nil {
			return err
		}
	}
	return nil
}

func installLink(stage string) error {
	userHome, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home for user-local installation: %w", err)
	}
	if !filepath.IsAbs(userHome) {
		return errors.New("home directory must be absolute for user-local installation")
	}
	if err := build("bin/htmlpreview", runtime.GOOS, runtime.GOARCH, version()); err != nil {
		return err
	}
	target, err := filepath.Abs("bin/htmlpreview")
	if err != nil {
		return err
	}
	target, err = filepath.EvalSymlinks(target)
	if err != nil {
		return fmt.Errorf("resolve built executable: %w", err)
	}
	link := filepath.Join(stage, userHome, ".local/bin/htmlpreview")
	// #nosec G301 -- The directory holds a public executable link, not document data.
	if err := os.MkdirAll(filepath.Dir(link), 0755); err != nil {
		return fmt.Errorf("create installation directory: %w", err)
	}
	if err := os.Symlink(target, link); err != nil {
		if !errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("install symlink %q: %w", link, err)
		}
		existing, readErr := os.Readlink(link)
		if readErr != nil || existing != target {
			return fmt.Errorf("installation destination %q already exists; move it aside and retry: %w", link, err)
		}
	}
	_, err = fmt.Fprintf(os.Stdout, "%s -> %s\n", link, target)
	return err
}

func synchronize() error {
	if err := command(nil, "git", "add", "-A"); err != nil {
		return err
	}
	err := exec.Command("git", "diff", "--cached", "--quiet").Run()
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 1 {
			return fmt.Errorf("inspect staged changes: %w", err)
		}
		message := os.Getenv("COMMIT_MESSAGE")
		if message == "" {
			message = "chore: sync"
		}
		if err := command(nil, "git", "commit", "-m", message); err != nil {
			return err
		}
	}
	if err := command(nil, "git", "pull"); err != nil {
		return err
	}
	return command(nil, "git", "push")
}

func lint() error {
	output, err := exec.Command("gofmt", "-l", "cmd", "internal", "integration", "bundle.go").Output()
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(output)) > 0 {
		return fmt.Errorf("gofmt required:\n%s", output)
	}
	if err := command(nil, "go", "vet", "./..."); err != nil {
		return err
	}
	if err := command(nil, "golangci-lint", "run", "./..."); err != nil {
		return err
	}
	if err := command(nil, "stylua", "--check", "assets/pandoc"); err != nil {
		return err
	}
	return command(nil, "pandoc", "--from=markdown", "--to=html5", "--sandbox", "--metadata=htmlpreview-code-token:0123456789abcdef0123456789abcdef", "--lua-filter=assets/pandoc/fidelity.lua", "--output="+os.DevNull, "assets/pandoc/empty.md")
}

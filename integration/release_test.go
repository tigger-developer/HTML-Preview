// ABOUTME: Inspects the four release archives and generated local Homebrew recipe.
// ABOUTME: Cross-compilation evidence is separate from native execution evidence.
package integration

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"debug/elf"
	"debug/macho"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRT001_14_ReleaseArchives(t *testing.T) {
	cmd := exec.Command("make", "release", "VERSION=0.1.0")
	cmd.Dir = ".."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("release generation: %v\n%s", err, out)
	}
	manifest, err := os.ReadFile("../dist/SHA256SUMS")
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"darwin-amd64", "darwin-arm64", "linux-amd64", "linux-arm64"} {
		name := "htmlpreview-0.1.0-" + target + ".tar.gz"
		// #nosec G304 -- Archive names are assembled from a fixed test version and target table.
		data, err := os.ReadFile(filepath.Join("../dist", name))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(manifest), fmt.Sprintf("%x  %s", sha256.Sum256(data), name)) {
			t.Fatalf("archive checksum %s", name)
		}
		gz, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		tr := tar.NewReader(gz)
		seen := make(map[string][]byte)
		for {
			header, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			content, err := io.ReadAll(tr)
			if err != nil {
				t.Fatal(err)
			}
			seen[header.Name] = content
		}
		if err := gz.Close(); err != nil {
			t.Fatal(err)
		}
		for _, file := range []string{"htmlpreview", "LICENSE", "THIRD_PARTY_NOTICES.md", "asap-OFL.txt", "iosevka-custom-OFL.md", "golang-x-net-LICENSE", "bluemonday-LICENSE.md", "douceur-LICENSE", "gorilla-css-LICENSE"} {
			if len(seen[file]) == 0 {
				t.Errorf("%s lacks %s", name, file)
			}
		}
		if strings.HasPrefix(target, "darwin") {
			f, err := macho.NewFile(bytes.NewReader(seen["htmlpreview"]))
			if err != nil {
				t.Fatal(err)
			}
			want := macho.CpuAmd64
			if strings.HasSuffix(target, "arm64") {
				want = macho.CpuArm64
			}
			if f.Cpu != want {
				t.Fatalf("wrong Mach-O architecture %s", target)
			}
		} else {
			f, err := elf.NewFile(bytes.NewReader(seen["htmlpreview"]))
			if err != nil {
				t.Fatal(err)
			}
			want := elf.EM_X86_64
			if strings.HasSuffix(target, "arm64") {
				want = elf.EM_AARCH64
			}
			if f.Machine != want {
				t.Fatalf("wrong ELF architecture %s", target)
			}
		}
	}
	formula, err := os.ReadFile("../dist/Formula/htmlpreview.rb")
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{"class Htmlpreview < Formula", "depends_on \"pandoc\"", "depends_on :macos", "Hardware::CPU.arm?", "darwin-arm64.tar.gz", "darwin-amd64.tar.gz", "file://", "bin.install \"htmlpreview\""} {
		if !strings.Contains(string(formula), part) {
			t.Errorf("formula lacks %s", part)
		}
	}
}

func TestRT001_14_InvalidReleaseVersion(t *testing.T) {
	for _, version := range []string{"", "v1.0.0", "1.0", "1.0.0-beta", "1.-1.0"} {
		// #nosec G204 -- Fixed invalid-version fixtures are passed as separate argv values.
		cmd := exec.Command("make", "release", "VERSION="+version)
		cmd.Dir = ".."
		if out, err := cmd.CombinedOutput(); err == nil {
			t.Fatalf("invalid VERSION %q accepted: %s", version, out)
		}
	}
}

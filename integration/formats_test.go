// ABOUTME: Verifies broader formats through a staged installation and a PATH symlink.
// ABOUTME: Compares source identity and immutable input bytes from a different working directory.
package integration

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestRT008_9_InstalledFormats(t *testing.T) {
	stage := t.TempDir()
	// #nosec G204 -- Install only into this test's private staging directory.
	cmd := exec.Command("make", "install", "PREFIX=/htmlpreview-formats", "DESTDIR="+stage)
	cmd.Dir = ".."
	cmd.Env = append(os.Environ(), "GOFLAGS=-tags=htmlpreview_test_desktop")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("staged format installation: %v\n%s", err, output)
	}
	binDir := t.TempDir()
	if err := os.Symlink(filepath.Join(stage, "htmlpreview-formats/bin/htmlpreview"), filepath.Join(binDir, "htmlpreview")); err != nil {
		t.Fatal(err)
	}
	root, capture := t.TempDir(), t.TempDir()
	names := []string{"notes.txt", "source.go", "data.json", "authored.html", "office.docx", "office.odt"}
	bodies := [][]byte{[]byte("Literal *text*\n"), []byte("package main\n"), []byte(`{"n":42}`), []byte("<title>Own title</title><h1>Native reading</h1>")}
	for _, reader := range []string{"docx", "odt"} {
		// #nosec G204 -- Fixed built-in readers generate synthetic local office fixtures.
		converter := exec.Command("pandoc", "--sandbox", "--data-dir="+root, "--from=markdown", "--to="+reader, "--standalone")
		converter.Stdin = strings.NewReader("# Office reading\n")
		data, err := converter.Output()
		if err != nil {
			t.Fatalf("generate %s fixture: %v", reader, err)
		}
		bodies = append(bodies, data)
	}
	args := []string{"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"), "htmlpreview"}
	for i, name := range names {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, bodies[i], 0600); err != nil {
			t.Fatal(err)
		}
		args = append(args, path)
	}
	// #nosec G204 -- env resolves the test-owned PATH symlink; all arguments are synthetic paths.
	cmd = exec.Command("env", args...)
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), "HTMLPREVIEW_GRACE=100ms", "HTMLPREVIEW_LINKS=0", "HTMLPREVIEW_MODE=quick", "PREVIEW_TEST_CAPTURE="+capture)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("installed broad-format preview: %v\n%s", err, output)
	}
	for i, name := range names {
		// #nosec G304 -- Read fixed names within the owned source and output directories.
		original, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || sha256.Sum256(original) != sha256.Sum256(bodies[i]) {
			t.Fatalf("installed preview changed %s", name)
		}
		// #nosec G304 -- Numbered captures are produced by the controlled desktop adapter.
		data, err := os.ReadFile(filepath.Join(capture, fmt.Sprintf("%04d.html", i+1)))
		if err != nil {
			t.Fatal(err)
		}
		doc, err := html.Parse(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		identity := false
		var content strings.Builder
		for n := range doc.Descendants() {
			if n.Type == html.TextNode {
				content.WriteString(n.Data)
			}
			for _, a := range n.Attr {
				if a.Key == "data-hp-source" && a.Val == filepath.Join(root, name) {
					identity = true
				}
			}
		}
		if name == "authored.html" {
			if identity || !strings.Contains(content.String(), "Native reading") || !strings.Contains(content.String(), "Own title") {
				t.Fatal("installed native HTML lost authored presentation")
			}
		} else if !identity || strings.Count(string(data), "data:font/woff2;base64,") != 6 {
			t.Fatalf("installed %s lost source identity or packaged fonts", name)
		}
	}
}

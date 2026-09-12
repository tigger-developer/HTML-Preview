// ABOUTME: Tests document-container and resource confinement with synthetic owned canaries.
// ABOUTME: Exercises the public preview boundary and retains no external documents or network state.
package preview

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func appendArchiveFixture(t *testing.T, original []byte, total int, extra *zip.FileHeader, contents []byte) []byte {
	t.Helper()
	r, err := zip.NewReader(bytes.NewReader(original), int64(len(original)))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	for _, file := range r.File {
		if err := w.Copy(file); err != nil {
			t.Fatal(err)
		}
	}
	if extra != nil {
		entry, err := w.CreateHeader(extra)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(contents); err != nil {
			t.Fatal(err)
		}
	}
	for i := len(r.File); i < total; i++ {
		if _, err := w.Create(fmt.Sprintf("padding/empty-%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestRT008_8_ArchiveKindsAndLimits(t *testing.T) {
	root := t.TempDir()
	valid := readerFixture(t, root, "docx", false)
	for _, count := range []int{4096, 4097} {
		r := run(t, root, nil, source(t, root, "count.docx", string(appendArchiveFixture(t, valid, count, nil, nil))))
		if count == 4096 {
			success(t, r, 1)
		} else if r.code != 1 || len(r.opens) != 0 || !strings.Contains(r.stderr, "4096 entries") {
			t.Fatalf("archive count not bounded: %s", r.stderr)
		}
	}
	for _, mode := range []os.FileMode{os.ModeSymlink | 0600, os.ModeNamedPipe | 0600} {
		header := &zip.FileHeader{Name: "special", Method: zip.Deflate}
		header.SetMode(mode)
		r := run(t, root, nil, source(t, root, "special.docx", string(appendArchiveFixture(t, valid, 0, header, []byte("target")))))
		if r.code != 1 || len(r.opens) != 0 || !strings.Contains(r.stderr, "non-regular") {
			t.Fatalf("special archive entry admitted: %s", r.stderr)
		}
	}
	for _, name := range []string{"media/huge.png", "huge.png"} {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetMode(0600)
		body := append(rasterFixture(t), bytes.Repeat([]byte{0}, 10<<20)...)
		r := run(t, root, nil, source(t, root, "oversized.docx", string(appendArchiveFixture(t, valid, 0, header, body))))
		if r.code != 1 || len(r.opens) != 0 || !strings.Contains(r.stderr, "archive") {
			t.Errorf("oversized archive raster accepted: %s", r.stderr)
		}
	}
	for _, name := range []string{"folder/../file", "a/./../b"} {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetMode(0600)
		r := run(t, root, nil, source(t, root, "traversal.docx", string(appendArchiveFixture(t, valid, 0, header, []byte("bad")))))
		if r.code != 1 || len(r.opens) != 0 || !strings.Contains(r.stderr, "archive") {
			t.Errorf("traversal archive member admitted: %s", r.stderr)
		}
	}
}

func TestRT008_8_ExternalReferences(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { requests.Add(1); w.WriteHeader(204) }))
	defer server.Close()
	root := t.TempDir()
	secret := source(t, t.TempDir(), "secret.txt", "OUTSIDE_CONTENT_MUST_NOT_APPEAR")
	for _, c := range []struct{ name, body string }{
		{"include.tex", `\documentclass{article}\begin{document}\input{` + secret + `}\end{document}`},
		{"entity.dbk", `<!DOCTYPE article [<!ENTITY escaped SYSTEM "file://` + secret + `">]><article><title>Reader</title><para>&escaped;</para></article>`},
		{"remote.dbk", `<!DOCTYPE article [<!ENTITY escaped SYSTEM "` + server.URL + `/secret">]><article><title>Reader</title><para>&escaped;</para></article>`},
	} {
		r := run(t, root, nil, source(t, root, c.name, c.body))
		if r.code != 0 && r.code != 1 {
			t.Fatalf("unexpected exit status %d", r.code)
		}
		for _, output := range r.raw {
			if strings.Contains(output, "OUTSIDE_CONTENT_MUST_NOT_APPEAR") {
				t.Fatal("external source content leaked")
			}
		}
		if r.code != 0 && len(r.opens) != 0 {
			t.Fatal("failed conversion published partial page")
		}
	}
	if requests.Load() != 0 {
		t.Fatal("converter fetched external entities")
	}
}

func TestRT008_8_NativeResourcesStayRooted(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	image := source(t, outside, "pixel.png", string(rasterFixture(t)))
	style := source(t, outside, "style.css", ".secret{color:hotpink}")
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	input := `<link rel="stylesheet" href="` + fileReference(style) + `"><img alt="outside" src="` + fileReference(image) + `"><img alt="symlink" src="escape/pixel.png"><img alt="forged" src="data:image/png;base64,` + base64.StdEncoding.EncodeToString([]byte("not an image")) + `">`
	r := run(t, root, nil, source(t, root, "root.html", input))
	success(t, r, 1)
	if len(nodes(r.pages[0], "img")) != 0 || strings.Contains(r.raw[0], "hotpink") || r.stderr == "" {
		t.Fatal("denied root/data resources retained")
	}
	for _, label := range []string{"outside", "symlink", "forged"} {
		if !strings.Contains(textOf(nodes(r.pages[0], "body")[0]), label) {
			t.Fatal("denied image alternative lost")
		}
	}
}

func TestRT008_8_InflationChecksum(t *testing.T) {
	root := t.TempDir()
	valid := readerFixture(t, root, "docx", false)
	header := &zip.FileHeader{Name: "checksum", Method: zip.Store}
	header.SetMode(0600)
	data := appendArchiveFixture(t, valid, 0, header, []byte("checksum-canary"))
	index := bytes.Index(data, []byte("checksum-canary"))
	if index < 0 {
		t.Fatal("checksum fixture not constructed")
	}
	data[index] = 'X'
	r := run(t, root, nil, source(t, root, "crc.docx", string(data)))
	if r.code != 1 || len(r.opens) != 0 || !strings.Contains(r.stderr, "archive") || !strings.Contains(r.stderr, "checksum") {
		t.Fatalf("actual inflation checksum not enforced: %s", r.stderr)
	}
}

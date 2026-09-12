// ABOUTME: Verifies HTTP asset registration and parent-owned container media.
// ABOUTME: Uses real Pandoc office fixtures and exact response-byte comparisons.
package preview

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func parseHTTPDocument(t *testing.T, data []byte) *html.Node {
	t.Helper()
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func responseAsset(t *testing.T, s *runningTestService, method, target string) (int, http.Header, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, target, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, 50*1024*1024+1))
	closeErr := resp.Body.Close()
	if readErr != nil || closeErr != nil {
		t.Fatalf("read response: %v %v", readErr, closeErr)
	}
	return resp.StatusCode, resp.Header, data
}

func TestRT006_7_RegisteredRasterAndDownloads(t *testing.T) {
	s := startTestService(t, NativeHost())
	image := rasterFixture(t)
	if err := os.WriteFile(filepath.Join(s.root, "pixel.png"), image, 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"drawing.svg", "paper.pdf"} {
		source(t, s.root, name, "passive download fixture")
	}
	target := s.register(t, "entry.md", "# Assets\n![pixel](pixel.png) [drawing](drawing.svg) [paper](paper.pdf)\n")
	base := target[:strings.LastIndex(target, "/")+1]
	status, _, _ := responseAsset(t, s, "GET", base+"pixel.png")
	if status != 404 {
		t.Fatalf("unregistered raster status=%d, expected 404", status)
	}
	_, _, data := responseAsset(t, s, "GET", target)
	doc := parseHTTPDocument(t, data)
	images := nodes(doc, "img")
	if len(images) != 1 || !strings.HasPrefix(attr(images[0], "src"), s.origin+"/") {
		t.Fatal("local raster must receive an authorized HTTP route")
	}
	imageURL := attr(images[0], "src")
	status, headers, body := responseAsset(t, s, "GET", imageURL)
	if status != 200 || headers.Get("Content-Type") != "image/png" || !bytes.Equal(body, image) {
		t.Fatal("registered raster bytes or type changed")
	}
	status, headHeaders, body := responseAsset(t, s, "HEAD", imageURL)
	if status != 200 || len(body) != 0 || headHeaders.Get("Content-Length") != headers.Get("Content-Length") {
		t.Fatal("raster HEAD differs from GET")
	}
	for _, a := range nodes(doc, "a") {
		if textOf(a) != "drawing" && textOf(a) != "paper" {
			continue
		}
		status, headers, body := responseAsset(t, s, "GET", attr(a, "href"))
		if status != 200 || headers.Get("Content-Type") != "application/octet-stream" || !strings.HasPrefix(headers.Get("Content-Disposition"), "attachment;") || string(body) != "passive download fixture" {
			t.Fatal("download policy failed")
		}
	}
	if err := os.WriteFile(filepath.Join(s.root, "pixel.png"), []byte("<svg>untrusted</svg>"), 0600); err != nil {
		t.Fatal(err)
	}
	status, _, body = responseAsset(t, s, "GET", imageURL)
	if status != 415 || bytes.Contains(body, []byte("untrusted")) {
		t.Fatal("changed raster bypassed signature admission")
	}
}

func TestRT006_12_ContainerMediaOwnership(t *testing.T) {
	s := startTestService(t, NativeHost())
	for _, format := range []string{"docx", "odt", "epub"} {
		t.Run(format, func(t *testing.T) {
			body := readerFixture(t, t.TempDir(), format, true)
			target := s.register(t, "office."+format, string(body))
			status, _, data := responseAsset(t, s, "GET", target)
			if status != 200 {
				t.Fatalf("office HTTP response=%d", status)
			}
			doc := parseHTTPDocument(t, data)
			images := nodes(doc, "img")
			if len(images) != 1 {
				t.Fatal("container lost its raster")
			}
			media := attr(images[0], "src")
			if !strings.Contains(media, "/_media/") {
				t.Fatal("container media lacks a parent-scoped HTTP route")
			}
			status, _, image := responseAsset(t, s, "GET", media)
			if status != 200 || !bytes.Equal(image, rasterFixture(t)) {
				t.Fatal("container media bytes changed")
			}
			status, _, image = responseAsset(t, s, "HEAD", media)
			if status != 200 || len(image) != 0 {
				t.Fatal("media HEAD returned body or failed")
			}
			changed := writeReaderFixture(t, t.TempDir(), format, "# Changed office source\n")
			if err := os.WriteFile(filepath.Join(s.root, "office."+format), changed, 0600); err != nil {
				t.Fatal(err)
			}
			status, _, image = responseAsset(t, s, "GET", media)
			if status != 404 || bytes.Equal(image, rasterFixture(t)) {
				t.Fatal("old media survived parent revision change")
			}
		})
	}
}

func TestRT006_12_NativeHTMLAssetsAndLinks(t *testing.T) {
	s := startTestService(t, NativeHost())
	source(t, s.root, "next.txt", "literal text")
	image := rasterFixture(t)
	source(t, s.root, "image.png", string(image))
	source(t, s.root, "style.css", "h1{background-image:url(image.png)}")
	entry := s.register(t, "native.html", `<title>Native</title><link rel="stylesheet" href="style.css"><h1>Native</h1><img src="image.png"><a href="next.txt">next</a><script>fetch('/secret')</script>`)
	status, _, data := responseAsset(t, s, "GET", entry)
	if status != 200 || bytes.Contains(data, []byte("fetch(")) {
		t.Fatal("native service HTML lost passive policy")
	}
	doc := parseHTTPDocument(t, data)
	for _, a := range nodes(doc, "a") {
		if textOf(a) == "next" {
			status, _, child := responseAsset(t, s, "GET", attr(a, "href"))
			if status != 200 || !bytes.Contains(child, []byte("literal text")) {
				t.Fatal("native HTML link did not render target text")
			}
		}
	}
	for _, img := range nodes(doc, "img") {
		status, _, body := responseAsset(t, s, "GET", attr(img, "src"))
		if status != 200 || !bytes.Equal(body, image) {
			t.Fatal("native image was not an authorized service asset")
		}
	}
}

func TestRT006_7_FailedPublicationDoesNotGrantAssets(t *testing.T) {
	s := startTestService(t, NativeHost())
	source(t, s.root, "pixel.png", string(rasterFixture(t)))
	target := s.registerSettings(t, "small.html", `<img src="pixel.png"><p>`+strings.Repeat("content ", 15000)+`</p>`, map[string]any{"output_bytes": 220000})
	status, _, _ := responseAsset(t, s, "GET", target)
	if status != 413 {
		t.Fatalf("expected output limit, got %d", status)
	}
	base := target[:strings.LastIndex(target, "/")+1]
	status, _, data := responseAsset(t, s, "GET", base+"pixel.png")
	if status != 404 || bytes.Equal(data, rasterFixture(t)) {
		t.Fatal("failed publication granted an asset")
	}
}

func TestRT006_7_ExplicitCrossRootAssets(t *testing.T) {
	other, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := startTestService(t, NativeHost(), other)
	file := source(t, other, "pixel.png", string(rasterFixture(t)))
	source(t, other, "style.css", "h1{color:tan}")
	target := s.register(t, "cross.html", `<link rel="stylesheet" href="`+fileReference(filepath.Join(other, "style.css"))+`"><h1>Cross root</h1><img src="`+fileReference(file)+`">`)
	status, _, data := responseAsset(t, s, "GET", target)
	if status != 200 || !bytes.Contains(data, []byte("color:tan")) {
		t.Fatal("explicitly authorized cross-root stylesheet was omitted")
	}
	images := nodes(parseHTTPDocument(t, data), "img")
	if len(images) != 1 || attr(images[0], "src") == "" {
		t.Fatal("explicitly authorized cross-root image was omitted")
	}
	status, _, data = responseAsset(t, s, "GET", attr(images[0], "src"))
	if status != 200 || !bytes.Equal(data, rasterFixture(t)) {
		t.Fatal("cross-root image not served")
	}
}

// ABOUTME: Verifies local SVG image references become bounded PNG presentation.
// ABOUTME: Keeps file and service transports aligned without serving source SVG inline.
package preview

import (
	"bytes"
	"context"
	"encoding/base64"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

const svgRasterFixture = `<svg xmlns="http://www.w3.org/2000/svg" width="12" height="7" viewBox="0 0 12 7"><rect width="12" height="7" fill="#12635e"/></svg>`

func assertPNGRaster(t *testing.T, data []byte) {
	t.Helper()
	image, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("SVG image did not produce PNG: %v", err)
	}
	if image.Width != 12 || image.Height != 7 {
		t.Fatalf("SVG raster dimensions=%dx%d, expected 12x7", image.Width, image.Height)
	}
}

func TestRT018_1_LocalSVGImagesBecomePNG(t *testing.T) {
	t.Run("file preview", func(t *testing.T) {
		root := t.TempDir()
		source(t, root, "keyboard.svg", svgRasterFixture)
		entry := source(t, root, "entry.md", "![Keyboard](keyboard.svg)\n")
		result := run(t, root, nil, entry)
		success(t, result, 1)
		images := nodes(documentNode(t, result.pages[0], "hp-document"), "img")
		if len(images) != 1 {
			t.Fatalf("rendered SVG image count=%d, expected 1", len(images))
		}
		encoded, ok := strings.CutPrefix(attr(images[0], "src"), "data:image/png;base64,")
		if !ok {
			t.Fatal("file preview did not embed the rasterized SVG as PNG")
		}
		data, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			t.Fatal(err)
		}
		assertPNGRaster(t, data)
	})

	t.Run("service preview", func(t *testing.T) {
		service := startTestService(t, NativeHost())
		source(t, service.root, "keyboard.svg", svgRasterFixture)
		target := service.register(t, "entry.md", "![Keyboard](keyboard.svg)\n")
		status, _, page := responseAsset(t, service, http.MethodGet, target)
		if status != http.StatusOK {
			t.Fatalf("document status=%d, expected 200", status)
		}
		images := nodes(parseHTTPDocument(t, page), "img")
		if len(images) != 1 || !strings.Contains(attr(images[0], "src"), "/_media/") {
			t.Fatal("service did not publish rasterized SVG through a parent-owned media route")
		}
		status, headers, data := responseAsset(t, service, http.MethodGet, attr(images[0], "src"))
		if status != http.StatusOK || headers.Get("Content-Type") != "image/png" {
			t.Fatalf("PNG media response status=%d type=%q", status, headers.Get("Content-Type"))
		}
		assertPNGRaster(t, data)
	})

	t.Run("automatic external resources stay blocked", func(t *testing.T) {
		var requests atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			requests.Add(1)
		}))
		defer server.Close()
		root := t.TempDir()
		svg := `<svg xmlns="http://www.w3.org/2000/svg" width="12" height="7"><rect width="12" height="7" fill="#12635e"/><image href="` + server.URL + `/tracking.png" width="12" height="7"/></svg>`
		source(t, root, "external.svg", svg)
		entry := source(t, root, "entry.md", "![External](external.svg)\n")
		result := run(t, root, nil, entry)
		success(t, result, 1)
		if requests.Load() != 0 {
			t.Fatal("SVG rasterization fetched a source-directed network resource")
		}
	})

	t.Run("missing converter leaves inactive alternative", func(t *testing.T) {
		host := NativeHost()
		lookPath := host.LookPath
		host.LookPath = func(name string) (string, error) {
			if name == "rsvg-convert" {
				return "", os.ErrNotExist
			}
			return lookPath(name)
		}
		service := startTestService(t, host)
		source(t, service.root, "keyboard.svg", svgRasterFixture)
		target := service.register(t, "entry.md", "![Keyboard](keyboard.svg)\n")
		status, _, page := responseAsset(t, service, http.MethodGet, target)
		if status != http.StatusOK || len(nodes(parseHTTPDocument(t, page), "img")) != 0 || !bytes.Contains(page, []byte("Keyboard")) {
			t.Fatal("missing SVG converter did not preserve a readable document and image alternative")
		}
	})
}

func TestRT018_1_SVGRasterBounds(t *testing.T) {
	for _, test := range []struct {
		name          string
		source        string
		width, height int
		wantError     bool
	}{
		{"explicit pixels", svgRasterFixture, 12, 7, false},
		{"viewBox dimensions", `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 9"/>`, 16, 9, false},
		{"oversized", `<svg xmlns="http://www.w3.org/2000/svg" width="4097" height="1"/>`, 0, 0, true},
		{"percentage", `<svg xmlns="http://www.w3.org/2000/svg" width="100%" height="100%"/>`, 0, 0, true},
		{"document type", `<!DOCTYPE svg><svg xmlns="http://www.w3.org/2000/svg" width="1" height="1"/>`, 0, 0, true},
		{"wrong namespace", `<svg width="12" height="7"/>`, 0, 0, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			width, height, err := svgRasterDimensions([]byte(test.source))
			if test.wantError && err == nil {
				t.Fatal("unsafe or unbounded SVG dimensions were accepted")
			}
			if !test.wantError && (err != nil || width != test.width || height != test.height) {
				t.Fatalf("dimensions=%dx%d error=%v", width, height, err)
			}
		})
	}

	host := NativeHost()
	host.Execute = func(context.Context, Command) ([]byte, error) {
		return []byte("rsvg-convert version 2.56.9\n"), nil
	}
	if err := checkSVGConverter(t.Context(), host, "/converter"); err == nil || !strings.Contains(err.Error(), "2.57.0") {
		t.Fatalf("unsafe converter version was accepted: %v", err)
	}
}

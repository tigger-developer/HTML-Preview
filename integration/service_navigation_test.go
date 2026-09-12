// ABOUTME: Follows generated HTTP links through real local document conversions.
// ABOUTME: Covers lazy targets, Org searches, reader selectors and filesystem denials.
package integration

import (
	"bytes"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func serviceSource(t *testing.T, root, name, body string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func documentAnchor(t *testing.T, body []byte, label string) string {
	t.Helper()
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for n := range doc.Descendants() {
		if n.Data != "a" {
			continue
		}
		var text strings.Builder
		for child := range n.Descendants() {
			if child.Type == html.TextNode {
				text.WriteString(child.Data)
			}
		}
		if text.String() == label {
			for _, attr := range n.Attr {
				if attr.Key == "href" {
					return attr.Val
				}
			}
			return ""
		}
	}
	t.Fatalf("missing anchor label %q", label)
	return ""
}

func TestRT006_5_LazyDocumentNavigation(t *testing.T) {
	s := startService(t, serviceBinary(t))
	entry := serviceSource(t, s.root, "entry.md", "# Entry\n[late](child.org) [bad](broken.docx) [external](https://example.com/path?q=1#part)\n")
	serviceSource(t, s.root, "broken.docx", "not a zip document")
	entryURL := s.register(t, entry)
	status, _, body := serviceResponse(t, s.public, "GET", entryURL, nil)
	if status != 200 {
		t.Fatalf("entry eagerly failed on unvisited targets: %d", status)
	}
	late := documentAnchor(t, body, "late")
	if !strings.HasPrefix(late, s.origin+"/") {
		t.Fatal("relative target did not receive an HTTP route")
	}
	if documentAnchor(t, body, "external") != "https://example.com/path?q=1#part" {
		t.Fatal("external link changed")
	}
	serviceSource(t, s.root, "child.org", "* Created after entry\n[[file:entry.md][back]]\n")
	status, _, child := serviceResponse(t, s.public, "GET", late, nil)
	if status != 200 || !bytes.Contains(child, []byte("Created after entry")) {
		t.Fatal("linked target was not rendered when requested")
	}
	if documentAnchor(t, child, "back") != entryURL {
		t.Fatal("cyclic link lost equivalent capability context")
	}
	status, _, _ = serviceResponse(t, s.public, "GET", documentAnchor(t, body, "bad"), nil)
	if status != 422 {
		t.Fatalf("bad visited document status=%d, expected 422", status)
	}
}

func TestRT006_6_OrgSearchRedirect(t *testing.T) {
	s := startService(t, serviceBinary(t))
	entry := serviceSource(t, s.root, "entry.org", "* Start\n[[file:target.org::#stable-section][custom]]\n[[file:target.org::*Target heading][heading]]\n[[file:target.org::*Missing heading][missing]]\n")
	serviceSource(t, s.root, "target.org", "* Target heading\n:PROPERTIES:\n:CUSTOM_ID: stable-section\n:END:\nBody.\n")
	status, _, body := serviceResponse(t, s.public, "GET", s.register(t, entry), nil)
	if status != 200 {
		t.Fatal("Org entry failed")
	}
	for _, label := range []string{"custom", "heading"} {
		target := documentAnchor(t, body, label)
		for _, method := range []string{"GET", "HEAD"} {
			status, headers, response := serviceResponse(t, s.public, method, target, nil)
			if status != 303 || !strings.HasSuffix(headers.Get("Location"), "/target.org#stable-section") {
				t.Fatalf("%s lookup did not redirect to actual anchor: status=%d location=%s", label, status, headers.Get("Location"))
			}
			if method == "HEAD" && len(response) != 0 {
				t.Fatal("HEAD lookup returned a body")
			}
		}
	}
	status, _, response := serviceResponse(t, s.public, "GET", documentAnchor(t, body, "missing"), nil)
	if status != 200 || !bytes.Contains(response, []byte("Body.")) || !bytes.Contains(response, []byte("Org search not resolved")) {
		t.Fatal("unresolved search must show its target and explanation")
	}
}

func TestRT006_4_RootedRequestDenials(t *testing.T) {
	s := startService(t, serviceBinary(t))
	entry := serviceSource(t, s.root, "entry.md", "# Entry")
	outside := serviceSource(t, t.TempDir(), "outside.md", "DO NOT SERVE THIS SENTINEL")
	if err := os.Symlink(outside, filepath.Join(s.root, "escape.md")); err != nil {
		t.Fatal(err)
	}
	entryURL := s.register(t, entry)
	base := entryURL[:strings.LastIndex(entryURL, "/")+1]
	for _, tc := range []struct {
		path   string
		status int
	}{
		{"escape.md", 403}, {"missing.md", 404}, {"%2e%2e/outside.md", 400}, {"%2fetc%2fpasswd", 400}, {"a%5cb.md", 400}, {"%00.md", 400}, {"%FF.md", 400},
	} {
		t.Run(tc.path, func(t *testing.T) {
			status, _, body := serviceResponse(t, s.public, "GET", base+tc.path, nil)
			if status != tc.status || bytes.Contains(body, []byte("DO NOT SERVE")) {
				t.Fatalf("denial status=%d expected=%d or outside data disclosed", status, tc.status)
			}
		})
	}
}

func TestRT006_11_ReaderSelectorBoundary(t *testing.T) {
	s := startService(t, serviceBinary(t))
	file := serviceSource(t, s.root, "data.json", `{"answer":42}`)
	target := s.register(t, file)
	for _, tc := range []struct {
		query  string
		status int
	}{
		{"", 200}, {"htmlpreview-format=", 400}, {"htmlpreview-format=json&htmlpreview-format=markdown", 400}, {"htmlpreview-format=missing_reader", 415}, {"htmlpreview-format=markdown+smart", 400}, {"htmlpreview-format=markdown%2Bsmart", 200}, {"htmlpreview-format=markdown%2Bno_such_extension", 415}, {"htmlpreview-format=json", 422},
	} {
		t.Run(tc.query, func(t *testing.T) {
			status, _, _ := serviceResponse(t, s.public, "GET", target+"?"+tc.query, nil)
			if status != tc.status {
				t.Fatalf("selector status=%d expected=%d", status, tc.status)
			}
		})
	}
	entry := serviceSource(t, s.root, "selected.md", "[json](data.json?htmlpreview-format=markdown%2Bsmart#piece)")
	_, _, body := serviceResponse(t, s.public, "GET", s.register(t, entry), nil)
	u, err := url.Parse(documentAnchor(t, body, "json"))
	if err != nil || u.Query().Get("htmlpreview-format") != "markdown+smart" || u.Fragment != "piece" {
		t.Fatal("linked selector or fragment changed")
	}
}

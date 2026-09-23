// ABOUTME: Checks theme-control placement on published file and service previews.
// ABOUTME: Leaves actual colour switching and keyboard interaction to browser user tests.
package preview

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestRT015_1_ThemeControl(t *testing.T) {
	check := func(t *testing.T, doc *html.Node) {
		t.Helper()
		row := documentNode(t, doc, "hp-header-row")
		var children []*html.Node
		for child := row.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == html.ElementNode {
				children = append(children, child)
			}
		}
		if len(children) < 2 || children[0].Data != "button" || attr(children[0], "id") != "hp-theme-toggle" || attr(children[1], "id") != "hp-source" {
			t.Fatal("theme button must immediately precede the original filename")
		}
		button := children[0]
		if attr(button, "type") != "button" || attr(button, "aria-label") != "Dark mode" {
			t.Fatal("theme control needs native button semantics and an accessible name")
		}
		if attr(button, "data-hp-source") != "" || attr(children[1], "data-hp-source") == "" {
			t.Fatal("only the filename must own source copying")
		}
		for _, root := range nodes(doc, "html") {
			if attr(root, "data-hp-theme") != "" {
				t.Fatal("a new preview must leave theme selection to the system")
			}
		}
	}
	for _, ext := range []string{"org", "md"} {
		t.Run(ext, func(t *testing.T) {
			s := startTestService(t, NativeHost())
			path := source(t, s.root, "theme."+ext, "Readable text.\n")
			result := run(t, s.root, nil, path)
			success(t, result, 1)
			check(t, result.pages[0])
			endpoint := annotationRegistrationURL(t, s, path, "Reviewer")
			status, _, data := responseAsset(t, s, "GET", strings.Replace(endpoint, "/_annotations/v2/", "/", 1))
			if status != 200 {
				t.Fatalf("service page returned %d", status)
			}
			check(t, parseHTTPDocument(t, data))
		})
	}
}

// ABOUTME: Checks the service format contract against every installed Pandoc reader.
// ABOUTME: Follows returned native-HTML links and reuses the independent reader corpus.
package preview

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
)

func TestRT006_11_EveryInstalledReaderThroughServedLinks(t *testing.T) {
	s := startTestService(t, NativeHost())
	status, err := serviceDiscovery(t.Context(), s.control)
	if err != nil {
		t.Fatal(err)
	}
	var entry strings.Builder
	for _, reader := range status.InputFormats {
		body := readerFixture(t, t.TempDir(), reader, false)
		name := reader + ".data"
		source(t, s.root, name, string(body))
		fmt.Fprintf(&entry, `<a href="%s?htmlpreview-format=%s">%s</a>`, name, url.QueryEscape(reader), reader)
	}
	target := s.register(t, "readers.html", entry.String())
	code, _, data := responseAsset(t, s, "GET", target)
	if code != 200 {
		t.Fatalf("reader index returned %d", code)
	}
	anchors := nodes(parseHTTPDocument(t, data), "a")
	if len(anchors) != len(status.InputFormats) {
		t.Fatal("reader index lost a fixture link")
	}
	for _, a := range anchors {
		t.Run(textOf(a), func(t *testing.T) {
			link := attr(a, "href")
			if !strings.HasPrefix(link, s.origin+"/") {
				t.Fatal("reader link did not receive an authorized service URL")
			}
			code, _, body := responseAsset(t, s, "GET", link)
			if code != 200 || !strings.Contains(textOf(nodes(parseHTTPDocument(t, body), "body")[0]), "Reader fixture") {
				t.Fatalf("installed reader %s: status=%d, expected readable fixture", textOf(a), code)
			}
		})
	}
}

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
	// The service status intentionally lists only its native startup catalogue.
	// Enumerate the public CLI's full installed catalogue independently instead.
	listing := run(t, t.TempDir(), nil, "--list-input-formats")
	success(t, listing, 0)
	readers := strings.Fields(listing.stdout)
	var entry strings.Builder
	for _, reader := range readers {
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
	if len(anchors) != len(readers) {
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

func TestRT014_6_OptionalReaderSuffixLinks(t *testing.T) {
	s := startTestService(t, NativeHost())
	// DOCX uses a mapped suffix; markdown_strict is admitted by the optional
	// installed-reader catalogue. Neither link supplies an explicit selector.
	for _, reader := range []string{"docx", "markdown_strict"} {
		source(t, s.root, "linked."+reader, string(readerFixture(t, t.TempDir(), reader, false)))
	}
	source(t, s.root, "linked.unsupported", "UNSUPPORTED_RAW_PAYLOAD")
	target := s.register(t, "index.md", "[Office](linked.docx)\n\n[Specialist](linked.markdown_strict)\n\n[Unsupported](linked.unsupported)\n")
	status, _, data := responseAsset(t, s, "GET", target)
	if status != 200 {
		t.Fatal(status)
	}
	main := documentNode(t, parseHTTPDocument(t, data), "hp-document")
	links := nodes(main, "a")
	if len(links) != 3 {
		t.Fatalf("linked reader count=%d", len(links))
	}
	for _, link := range links {
		address := attr(link, "href")
		if !strings.HasPrefix(address, s.origin+"/") {
			t.Errorf("%s has no authorized preview URL", textOf(link))
			continue
		}
		status, _, body := responseAsset(t, s, "GET", address)
		if textOf(link) == "Unsupported" {
			if status < 400 || strings.Contains(string(body), "UNSUPPORTED_RAW_PAYLOAD") {
				t.Errorf("unsupported input was published: status=%d", status)
			}
			continue
		}
		if status != 200 || !strings.Contains(textOf(nodes(parseHTTPDocument(t, body), "body")[0]), "Reader fixture") {
			t.Errorf("%s: status=%d, expected converted content", textOf(link), status)
		}
	}
}

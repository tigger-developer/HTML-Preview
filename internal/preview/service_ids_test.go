// ABOUTME: Exercises fileless Org IDs through the bounded rendered HTTP catalogue.
// ABOUTME: Checks discovery order, duplicate IDs and stale source rejection without directory scans.
package preview

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func orgIDLink(t *testing.T, s *runningTestService, target string) string {
	t.Helper()
	status, _, data := responseAsset(t, s, "GET", target)
	if status != 200 {
		t.Fatalf("Org link source status=%d", status)
	}
	doc := parseHTTPDocument(t, data)
	for _, node := range nodes(doc, "a") {
		if textOf(node) == "Find target" {
			return attr(node, "href")
		}
	}
	if !bytes.Contains(data, []byte("Find target")) {
		t.Fatal("unresolved ID lost its label")
	}
	return ""
}

func TestRT006_6_FilelessIDsRequireFreshUniqueRenderedTarget(t *testing.T) {
	s := startTestService(t, NativeHost())
	entry := s.register(t, "entry.org", "* Entry\n[[id:target-id][Find target]]\n")
	target := s.register(t, "target.org", "* Destination\n:PROPERTIES:\n:ID: target-id\n:END:\n")
	if link := orgIDLink(t, s, entry); link != "" {
		t.Fatal("unrendered file acquired an ID link")
	}
	status, _, data := responseAsset(t, s, "GET", target)
	if status != 200 {
		t.Fatal("target did not render")
	}
	link := orgIDLink(t, s, entry)
	if !strings.HasPrefix(link, target+"#") {
		t.Fatal("known unique ID did not resolve to its rendered target")
	}
	_, fragment, _ := strings.Cut(link, "#")
	if !bytes.Contains(data, []byte(`id="`+fragment+`"`)) {
		t.Fatal("ID link does not name an actual rendered anchor")
	}
	duplicate := s.register(t, "duplicate.org", "* Duplicate\n:PROPERTIES:\n:ID: target-id\n:END:\n")
	if status, _, _ := responseAsset(t, s, "GET", duplicate); status != 200 {
		t.Fatal("duplicate fixture did not render")
	}
	if link := orgIDLink(t, s, entry); link != "" {
		t.Fatal("ambiguous ID remained active")
	}
	if err := os.WriteFile(filepath.Join(s.root, "duplicate.org"), []byte("* Changed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if link := orgIDLink(t, s, entry); !strings.HasPrefix(link, target+"#") {
		t.Fatal("stale duplicate catalogue prevented the fresh unique target")
	}
	if err := os.WriteFile(filepath.Join(s.root, "target.org"), []byte("* Changed target\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if link := orgIDLink(t, s, entry); link != "" {
		t.Fatal("stale target ID remained active")
	}
}

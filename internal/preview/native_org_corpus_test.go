// ABOUTME: Reproduces native Org conversion differences found in the frozen corpus.
// ABOUTME: Uses synthetic documents to protect private sources while checking final HTML.
package preview

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestRT011_2_CorpusListSemantics(t *testing.T) {
	root := t.TempDir()
	path := source(t, root, "lists.org", "* Lists\n- Named :: A description.\n- Ordinary item.\n\n* Continuation\n- Wrapped prose on\n  p. 32 and more prose.\n\n1. Numbered item.\n2. Another numbered item.\n\na. Alphabet text.\n")
	r := run(t, root, nil, path)
	success(t, r, 1)
	body := documentNode(t, r.pages[0], "hp-document")
	terms := nodes(body, "dt")
	if len(terms) != 1 || strings.TrimSpace(textOf(terms[0])) != "Named" {
		t.Error("ordinary bullet acquired a definition term")
	}
	found := false
	for _, li := range nodes(body, "li") {
		if strings.TrimSpace(textOf(li)) == "Ordinary item." && li.Parent.Data == "ul" {
			found = true
		}
	}
	if !found {
		t.Error("ordinary item is not an unordered list item")
	}
	text := strings.Join(strings.Fields(textOf(body)), " ")
	if !strings.Contains(text, "Wrapped prose on p. 32 and more prose.") || !strings.Contains(text, "a. Alphabet text.") {
		t.Error("alphabetic prose was consumed as a list marker")
	}
	if ordered := nodes(body, "ol"); len(ordered) != 1 || len(nodes(ordered[0], "li")) != 2 {
		t.Error("numeric list structure changed")
	}
}

func TestRT011_2_LocalHeadingSearches(t *testing.T) {
	text := "* Start\n[[*Target: formatted][local]] [[*Target: /formatted/][styled]] [[*Target: /formatted/]] [[*Repeated][ambiguous]] [[*Absent][missing]]\n\n* Target: /formatted/\n:PROPERTIES:\n:CUSTOM_ID: chosen-target\n:END:\nBody.\n\n* Repeated\nOne.\n* Repeated\nTwo.\n"
	check := func(t *testing.T, doc *html.Node) {
		t.Helper()
		found := 0
		body := documentNode(t, doc, "hp-document")
		for _, link := range nodes(documentNode(t, doc, "hp-document"), "a") {
			switch textOf(link) {
			case "local", "styled", "*Target: /formatted/":
				found++
				if attr(link, "href") != "#chosen-target" {
					t.Errorf("local heading destination: %q", attr(link, "href"))
				}
			default:
				if !strings.HasPrefix(textOf(link), "ambiguous") && !strings.HasPrefix(textOf(link), "missing") {
					continue
				}
				if attr(link, "href") != "" {
					t.Error("unresolved heading received a destination")
				}
			}
		}
		if found != 3 || !strings.Contains(textOf(body), "ambiguous") || !strings.Contains(textOf(body), "missing") {
			t.Errorf("heading link labels lost: %d", found)
		}
	}
	t.Run("file", func(t *testing.T) {
		root := t.TempDir()
		path := source(t, root, "links.org", text)
		r := run(t, root, nil, path)
		success(t, r, 1)
		check(t, r.pages[0])
	})
	t.Run("http", func(t *testing.T) {
		s := startTestService(t, NativeHost())
		path := source(t, s.root, "links.org", text)
		endpoint := annotationRegistrationURL(t, s, path, "Reviewer")
		status, _, data := responseAsset(t, s, "GET", strings.Replace(endpoint, "/_annotations/v2/", "/", 1))
		if status != 200 {
			t.Fatal("page unavailable", status)
		}
		check(t, parseHTTPDocument(t, data))
	})
}

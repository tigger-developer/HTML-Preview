// ABOUTME: Checks served annotation targets from the text a reader can click.
// ABOUTME: Protects the HTML contract consumed by the existing browser selectors.
package preview

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// RT011.4 - Whole verified blocks must be reachable through the browser's
// nearest-block selectors, not merely present somewhere in the response.
func TestHTTPAnnotationTargetContract(t *testing.T) {
	s := startTestService(t, NativeHost())
	for _, format := range []string{"org", "md"} {
		t.Run(format, func(t *testing.T) {
			// #nosec G304 -- Both filenames come from this test's fixed format list.
			fixture, err := os.ReadFile(filepath.Join("..", "..", "testdata", "annotation-targets", "blocks."+format))
			if err != nil {
				t.Fatal(err)
			}
			path := source(t, s.root, "blocks."+format, string(fixture))
			endpoint := annotationRegistrationURL(t, s, path, "Reviewer")
			pageURL := strings.Replace(endpoint, "/_annotations/v2/", "/", 1)
			status, _, data := responseAsset(t, s, "GET", pageURL)
			if status != 200 {
				t.Fatalf("page status %d", status)
			}
			doc := parseHTTPDocument(t, data)
			main := documentNode(t, doc, "hp-document")
			var metadata map[string]any
			if err := json.Unmarshal([]byte(textOf(documentNode(t, doc, "hp-annotation-data"))), &metadata); err != nil {
				t.Fatal("browser bootstrap metadata:", err)
			}
			status, state := annotationJSON(t, s, "GET", metadata["endpoint"].(string), nil, nil)
			if status != 200 || state["writable"] != true {
				t.Fatal("annotation creation unavailable", status, state["reason"])
			}
			for _, key := range []string{"revision", "source_revision", "body_revision"} {
				if metadata[key] == "" || metadata[key] != state[key] {
					t.Errorf("page and write state disagree on %s", key)
				}
			}
			for _, text := range []string{
				"Heading", "Ordinary paragraph", "strong text", "emphasized text",
				"inline code", "a local link", "second source line", "Tight item", "item emphasis",
				"Parent item", "Nested child", "child emphasis", "Checked item", "Unchecked item",
				"Partial item", "source block", "Column", "Cell",
			} {
				t.Run(text, func(t *testing.T) { assertClickableAnnotationText(t, main, text) })
			}
			if format == "org" {
				for _, text := range []string{"literal text", "Definition body", "definition emphasis", "Quoted paragraph", "quote emphasis", "First verse line", "Second verse line", "Description group", "A description before", "Nested literal item", "Nested ordinary item", "A description after", "Paragraph after the description list"} {
					t.Run(text, func(t *testing.T) { assertClickableAnnotationText(t, main, text) })
				}
				for _, text := range []string{"Continued descriptions", "Description starting", "Description following", "Star-nested child", "Numbered-nested child", "Ordinary prose ending"} {
					t.Run(text, func(t *testing.T) { assertClickableAnnotationText(t, main, text) })
				}
			}
			// The browser refreshes through the advertised page URL, which can
			// select an explicit reader even when the original request did not.
			status, _, refreshed := responseAsset(t, s, "GET", metadata["page_url"].(string))
			if status != 200 {
				t.Fatalf("refresh status %d", status)
			}
			refreshedMain := documentNode(t, parseHTTPDocument(t, refreshed), "hp-document")
			for _, text := range []string{"Heading", "Ordinary paragraph", "item emphasis", "child emphasis", "source block", "Cell"} {
				assertClickableAnnotationText(t, refreshedMain, text)
			}
			block := assertClickableAnnotationText(t, refreshedMain, "Ordinary paragraph")
			headers := map[string]string{"Origin": s.origin, "X-HTMLPreview-Annotation-Token": state["write_token"].(string), "X-HTMLPreview-Composer-Token": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}
			request := map[string]any{
				"operation_id": "10000000-0000-4000-8000-000000000001", "annotation_id": "20000000-0000-4000-8000-000000000001",
				"composer_id": "30000000-0000-4000-8000-000000000001", "sequence": 1,
				"revision": state["revision"], "source_revision": state["source_revision"], "body_revision": state["body_revision"],
				"action": "upsert", "label": "reviewer-001", "target": map[string]any{"type": "point", "block_id": block}, "text": "Synthetic review note.",
			}
			status, result := annotationJSON(t, s, "POST", metadata["endpoint"].(string), request, headers)
			if status != 201 {
				t.Fatalf("creation from browser-selectable target: %d %v", status, result)
			}
			status, _, refreshed = responseAsset(t, s, "GET", metadata["page_url"].(string))
			if status != 200 {
				t.Fatalf("post-save refresh status %d", status)
			}
			refreshedMain = documentNode(t, parseHTTPDocument(t, refreshed), "hp-document")
			assertClickableAnnotationText(t, refreshedMain, "Ordinary paragraph")
			assertClickableAnnotationText(t, refreshedMain, "Tight item")
			if !strings.Contains(textOf(refreshedMain), "Synthetic review note.") {
				t.Fatal("created footnote absent from refreshed page")
			}
		})
	}
}

func assertClickableAnnotationText(t *testing.T, main *html.Node, text string) string {
	t.Helper()
	found := false
	selected := ""
	for n := range main.Descendants() {
		if n.Type != html.TextNode || !strings.Contains(n.Data, text) || insideEndnotes(n) {
			continue
		}
		found = true
		for p := n.Parent; p != main; p = p.Parent {
			if p.Data == "button" || p.Data == "summary" || hasClass(p, "todo") || hasClass(p, "done") || hasClass(p, "tag") || hasClass(p, "priority") || hasClass(p, "cookie") {
				t.Errorf("%q is inside an annotation-excluded element <%s>", text, p.Data)
			}
			if p.Data == "a" && attr(p, "href") == "" {
				t.Errorf("%q is inside an anchor that neither click handler accepts", text)
			}
		}
		// The browser gives enclosing table/quote/verse containers precedence;
		// otherwise the closest semantic block must carry the verified ID itself.
		var block *html.Node
		for p := n.Parent; p != main; p = p.Parent {
			if (p.Data == "table" || p.Data == "blockquote" || p.Data == "div") && attr(p, "data-hp-annotation-block") != "" {
				block = p
				break
			}
		}
		if block == nil {
			for p := n.Parent; p != main; p = p.Parent {
				switch p.Data {
				case "p", "li", "dt", "dd", "td", "th", "pre", "details", "summary", "h1", "h2", "h3", "h4", "h5", "h6":
					block = p
				}
				if block != nil {
					break
				}
			}
		}
		if block == nil {
			t.Errorf("%q has no browser-selectable block", text)
		} else if attr(block, "data-hp-annotation-block") == "" {
			t.Errorf("%q selects <%s> without an annotation target", text, block.Data)
		} else {
			selected = attr(block, "data-hp-annotation-block")
		}
	}
	if !found {
		t.Fatalf("visible text %q is missing", text)
	}
	return selected
}

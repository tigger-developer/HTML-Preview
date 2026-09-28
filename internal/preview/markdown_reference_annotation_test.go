// ABOUTME: Verifies annotation entry and saves beside hidden Markdown references.
// ABOUTME: Uses synthetic documents and the same served block contract as the browser.
package preview

import (
	"fmt"
	"strings"
	"testing"
)

func TestRT016_1_AnnotationsWithReferenceDefinitions(t *testing.T) {
	s := startTestService(t, NativeHost())
	for i, definition := range []string{
		"[style-setup]: # (category: synthetic; purpose: regression)",
		"[example]: <https://example.invalid/reference> \"Reference title\"",
		"[example]:\n  <https://example.invalid/reference>\n  \"Reference title\"",
		"   [example]: <https://example.invalid/reference>\r\n   \"Reference title\"",
		"[example]: <https://example.invalid/reference> \"Reference title\"\n[unused]: # (Another hidden definition)",
	} {
		t.Run(fmt.Sprintf("definition-%d", i), func(t *testing.T) {
			body := definition + "\n\nFirst paragraph.\n\n## Heading\n\n- List item.\n\nLast paragraph.\n\n[Not a definition] remains visible.\n\n[Reference link][example]\n\n```text\n[literal]: # (Code stays literal)\n```\n"
			path := source(t, s.root, fmt.Sprintf("references-%d.md", i), body)
			endpoint := annotationRegistrationURL(t, s, path, "Reviewer")
			pageURL := strings.Replace(endpoint, "/_annotations/v2/", "/", 1)
			main, state := markdownBrowserPage(t, s, pageURL)
			for _, text := range []string{"First paragraph.", "Heading", "List item.", "Last paragraph.", "[Not a definition] remains visible.", "Code stays literal"} {
				assertClickableAnnotationText(t, main, text)
			}
			if i > 0 {
				found := false
				for node := range main.Descendants() {
					if node.Data == "a" && textOf(node) == "Reference link" && attr(node, "href") == "https://example.invalid/reference" && attr(node, "title") == "Reference title" {
						found = true
					}
				}
				if !found {
					t.Fatal("reference link lost its destination or title")
				}
			}
			if t.Failed() {
				return
			}
			block := assertClickableAnnotationText(t, main, "First paragraph.")
			markdownCreateAtBlock(t, s, state, block, "reviewer-001", 1)
			data := readMarkdownSource(t, path)
			if !strings.HasPrefix(data, definition+"\n\nFirst paragraph.[^reviewer-001]\n") {
				t.Fatal("save changed the reference definition or missed the paragraph boundary")
			}
			main, state = markdownBrowserPage(t, s, pageURL)
			if strings.Contains(textOf(main), "[style-setup]:") || strings.Contains(textOf(main), "[example]:") {
				t.Fatal("hidden definition became visible")
			}
			block = assertClickableAnnotationText(t, main, "Last paragraph.")
			markdownCreateAtBlock(t, s, state, block, "reviewer-002", 2)
			if !strings.Contains(readMarkdownSource(t, path), "Last paragraph.[^reviewer-002]") {
				t.Fatal("subsequent annotation missed its paragraph boundary")
			}
		})
	}
}

// ABOUTME: Checks navigation ownership and document order in final preview HTML.
// ABOUTME: Responsive positioning is verified separately in a browser.
package preview

import (
	"testing"

	"golang.org/x/net/html"
)

func TestRT005_2_NavigationStructure(t *testing.T) {
	for _, format := range []string{"org", "md"} {
		t.Run(format, func(t *testing.T) {
			root := t.TempDir()
			input := "# First\n\nBody.\n\n## Second\n\nMore.\n"
			if format == "org" {
				input = "#+TITLE: Title\n* First\nBody.\n** Second\nMore.\n"
			}
			path := source(t, root, "doc."+format, input)
			r := run(t, root, nil, path)
			success(t, r, 1)
			nav := documentNode(t, r.pages[0], "hp-toc")
			body := nodes(r.pages[0], "body")[0]
			wantFormat := "markdown"
			if format == "org" {
				wantFormat = "org"
			}
			if attr(body, "data-hp-format") != wantFormat || attr(nav, "aria-label") != "Document navigation" {
				t.Fatal("navigation lacks format or accessible identity")
			}
			checkContents(t, r.pages[0], []string{"First", "Second"})
			previous := nav.PrevSibling
			for previous != nil && previous.Type != html.ElementNode {
				previous = previous.PrevSibling
			}
			if previous != documentNode(t, r.pages[0], "hp-header") {
				t.Fatal("navigation must immediately follow frontmatter/header in reading order")
			}
			for _, disabled := range []bool{false, true} {
				file := path
				var env []string
				if disabled {
					env = []string{"HTMLPREVIEW_TOC=0"}
				} else {
					file = source(t, root, "empty."+format, "A paragraph without headings.\n")
				}
				r = run(t, root, env, file)
				success(t, r, 1)
				checkContents(t, r.pages[0], nil)
				if attr(nodes(r.pages[0], "body")[0], "class") != "" {
					t.Fatal("disabled/empty navigation reserved a sidebar column")
				}
			}
		})
	}
}

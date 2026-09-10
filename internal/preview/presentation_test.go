// ABOUTME: Verifies source metadata placement in the generated preview.
// ABOUTME: Browser gestures and visual appearance have separate one-off evidence.
package preview

import (
	"strings"
	"testing"
)

func TestRT004_3_ConsolidatedFrontmatter(t *testing.T) {
	root := t.TempDir()
	input := "#+TITLE: A document\n#+SUBTITLE: Its subtitle\n#+AUTHOR: A Writer\n#+DATE: 2026-09-10\n#+STARTUP: overview\n#+TODO: WAIT | FINISHED\n#+OPTIONS: toc:nil\n#+TAGS: example\n#+CUSTOM: <safe>\n\n* WAIT Heading :example:\n:PROPERTIES:\n:OWNER: Reader\n:END:\nA ~code~ fragment.\n#+BEGIN_SRC text\n#+TITLE: literal source\n#+END_SRC\n"
	r := run(t, root, nil, source(t, root, "metadata.org", input))
	success(t, r, 1)
	front := documentNode(t, r.pages[0], "hp-frontmatter")
	labels, values := nodes(front, "dt"), nodes(front, "dd")
	want := []string{"A document", "Its subtitle", "A Writer", "2026-09-10", "overview", "WAIT | FINISHED", "toc:nil", "example", "<safe>"}
	names := []string{"TITLE", "SUBTITLE", "AUTHOR", "DATE", "STARTUP", "TODO", "OPTIONS", "TAGS", "CUSTOM"}
	if len(labels) != len(want) || len(values) != len(want) {
		t.Fatalf("frontmatter fields: labels=%d values=%d, want %d", len(labels), len(values), len(want))
	}
	for i, value := range want {
		if textOf(labels[i]) != "⚙ "+names[i] {
			t.Errorf("field %d name: %q, want %q", i, textOf(labels[i]), names[i])
		}
		if textOf(values[i]) != value {
			t.Errorf("field %d: %q, want %q", i, textOf(values[i]), value)
		}
	}
	body := documentNode(t, r.pages[0], "hp-document")
	if strings.Contains(textOf(body), "#+STARTUP:") || strings.Contains(textOf(body), "#+TODO:") {
		t.Fatal("frontmatter repeated in document body")
	}
	if !strings.Contains(textOf(body), "#+TITLE: literal source") {
		t.Fatal("literal code was extracted as metadata")
	}
	header := documentNode(t, r.pages[0], "hp-header")
	if front.Parent != header {
		t.Fatal("frontmatter must span the header independently of the filename row")
	}
	sourceHeader := documentNode(t, r.pages[0], "hp-source")
	if sourceHeader.Parent == front || textOf(sourceHeader) != attr(sourceHeader, "data-hp-source") {
		t.Fatal("filename copy target includes metadata")
	}
}

func TestRT004_3_FrontmatterBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		fields      int
	}{
		{"empty", "* Body\n", 0},
		{"settings-only", "#+STARTUP: overview\n#+OPTIONS: num:nil\n* Body\n", 2},
		{"repeat", "#+TITLE: First\n#+TITLE: Last\n* Body\n", 2},
		{"literal-first", "#+BEGIN_EXAMPLE\n#+TITLE: literal\n#+END_EXAMPLE\n* Body\n", 0},
		{"drawer-first", ":LOGBOOK:\n#+TITLE: literal\n:END:\n* Body\n", 0},
		{"body-keywords", "Body first.\n#+TITLE: Too late\n#+SUBTITLE: Too late\n#+AUTHOR: Too late\n#+DATE: Too late\n", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			r := run(t, root, nil, source(t, root, "doc.org", tc.input))
			success(t, r, 1)
			count := 0
			for _, details := range nodes(r.pages[0], "details") {
				if attr(details, "id") == "hp-frontmatter" {
					count++
					if len(nodes(details, "dt")) != tc.fields {
						t.Fatal("frontmatter field count")
					}
					if tc.name == "repeat" {
						for i, value := range []string{"First", "Last"} {
							if textOf(nodes(details, "dt")[i]) != "⚙ TITLE" || textOf(nodes(details, "dd")[i]) != value {
								t.Fatal("repeated title label/value pair")
							}
						}
					}
				}
			}
			if (tc.fields == 0 && count != 0) || (tc.fields > 0 && count != 1) {
				t.Fatalf("frontmatter panels=%d fields=%d", count, tc.fields)
			}
			if tc.name == "repeat" && textOf(nodes(r.pages[0], "title")[0]) != "Last" {
				t.Fatal("last title did not win")
			}
			if tc.fields == 0 {
				if textOf(nodes(r.pages[0], "title")[0]) != "doc.org" {
					t.Fatal("non-frontmatter keyword promoted to browser title")
				}
				for _, section := range nodes(r.pages[0], "section") {
					if attr(section, "id") == "hp-document-title" {
						t.Fatal("non-frontmatter keyword promoted to title block")
					}
				}
			}
		})
	}
}

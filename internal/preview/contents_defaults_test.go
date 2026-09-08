// ABOUTME: Checks format-specific contents defaults through real CLI conversion.
// ABOUTME: Covers mixed inputs, linked pages, explicit settings and empty values.
package preview

import (
	"fmt"
	"strings"
	"testing"
)

func TestRT003_1_FormatContentsDefaults(t *testing.T) {
	root := t.TempDir()
	var markdown, org strings.Builder
	markdown.WriteString("---\ntoc: false\ntoc-depth: 1\n---\n\n")
	org.WriteString("#+OPTIONS: toc:6\n")
	for level := 1; level <= 4; level++ {
		fmt.Fprintf(&markdown, "%s Level %d\nBody.\n\n", strings.Repeat("#", level), level)
		fmt.Fprintf(&org, "%s Level %d\nBody.\n\n", strings.Repeat("*", level), level)
	}
	a := source(t, root, "a.MD", markdown.String()+"[Org](b.ORG)\n")
	b := source(t, root, "b.ORG", org.String()+"[[file:c.markdown][Markdown]]\n")
	c := source(t, root, "c.markdown", markdown.String()+"[Markdown](a.MD)\n")
	for _, setting := range []struct {
		name    string
		env     []string
		md, org int
	}{
		{"unset", nil, 3, 0},
		{"empty", []string{"HTMLPREVIEW_TOC=", "HTMLPREVIEW_TOC_DEPTH="}, 3, 0},
		{"enabled", []string{"HTMLPREVIEW_TOC=1"}, 3, 3},
		{"disabled", []string{"HTMLPREVIEW_TOC=0"}, 0, 0},
		{"depth only", []string{"HTMLPREVIEW_TOC_DEPTH=2"}, 2, 0},
	} {
		for i, entries := range [][]string{{a, b, c}, {b, c, a}, {a}, {b}} {
			t.Run(fmt.Sprintf("%s/entries-%d", setting.name, i), func(t *testing.T) {
				env := append([]string{"HTMLPREVIEW_LINKS=1", "HTMLPREVIEW_ROOT=" + root}, setting.env...)
				r := run(t, t.TempDir(), env, entries...)
				success(t, r, 3)
				seen := make(map[string]bool)
				for _, doc := range r.pages {
					path := attr(nodes(doc, "h1")[0], "data-hp-source")
					if path != a && path != b && path != c {
						t.Fatalf("unexpected source %q", path)
					}
					if seen[path] {
						t.Fatalf("duplicate source %q", path)
					}
					seen[path] = true
					depth := setting.md
					if path == b {
						depth = setting.org
					}
					var want []string
					for level := 1; level <= depth; level++ {
						want = append(want, fmt.Sprintf("Level %d", level))
					}
					checkContents(t, doc, want)
				}
			})
		}
	}
	r := run(t, root, []string{"HTMLPREVIEW_TOC_DEPTH=7"}, b)
	if r.code != 2 || len(r.opens) != 0 || len(r.pages) != 0 || r.cleanedAt != 0 || !strings.Contains(r.stderr, "HTMLPREVIEW_TOC_DEPTH") {
		t.Fatal("Org's inactive default depth was not validated before allocation")
	}
}

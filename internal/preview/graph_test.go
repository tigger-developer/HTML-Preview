// ABOUTME: Exercises bounded graph admission, aliases, and outline metadata.
// ABOUTME: Reading sessions end only after the retention state is observable.
package preview

import (
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestRT001_10_LinkedBreadthFirst(t *testing.T) {
	root := t.TempDir()
	a := source(t, root, "a.md", "# A\n\n[b](b.org) [c](c.md) [missing](missing.md)\n")
	source(t, root, "b.org", "* B\n[[file:a.md][cycle]] [[file:d.md][d]]\n")
	source(t, root, "c.md", "# C\n")
	source(t, root, "d.md", "# D\n")
	r := run(t, root, nil, a)
	success(t, r, 1)
	r = run(t, root, []string{"HTMLPREVIEW_LINKS=1"}, a)
	success(t, r, 4)
	if len(r.opens) != 1 {
		t.Fatalf("opened linked pages: %d", len(r.opens))
	}
	for i, title := range []string{"a.md", "b.org", "c.md", "d.md"} {
		if got := textOf(nodes(r.pages[i], "title")[0]); got != title {
			t.Errorf("BFS page %d=%s", i, got)
		}
	}
	for _, link := range nodes(r.pages[0], "a") {
		if textOf(link) == "b" {
			u, err := url.Parse(attr(link, "href"))
			if err != nil || !strings.HasSuffix(u.Path, "0002.html") {
				t.Fatalf("linked preview URL: %s", attr(link, "href"))
			}
		}
	}
}

func TestRT001_11_GraphCountDepthRoot(t *testing.T) {
	root := t.TempDir()
	a := source(t, root, "docs/a.md", "[b](b.md) [outside](../outside.md)\n")
	source(t, root, "docs/b.md", "[c](c.md)\n")
	source(t, root, "docs/c.md", "leaf")
	source(t, root, "outside.md", "outside")
	for _, tc := range []struct {
		env   []string
		pages int
	}{
		{[]string{"HTMLPREVIEW_LINKS=1", "HTMLPREVIEW_MAX_FILES=1"}, 1},
		{[]string{"HTMLPREVIEW_LINKS=1", "HTMLPREVIEW_MAX_DEPTH=0"}, 1},
		{[]string{"HTMLPREVIEW_LINKS=1", "HTMLPREVIEW_MAX_DEPTH=1"}, 2},
		{[]string{"HTMLPREVIEW_LINKS=1"}, 3},
		{[]string{"HTMLPREVIEW_LINKS=1", "HTMLPREVIEW_ROOT=" + root}, 4},
	} {
		r := run(t, root, tc.env, a)
		success(t, r, tc.pages)
	}
	outside := filepath.Join(root, "outside.md")
	r := run(t, root, []string{"HTMLPREVIEW_LINKS=1", "HTMLPREVIEW_MAX_FILES=1"}, a, outside)
	if r.code != 2 || len(r.opens) != 0 {
		t.Fatal("explicit reservation count")
	}
}

func TestRT001_16_OutlineMetadata(t *testing.T) {
	for _, startup := range []string{"overview", "content", "showall", "unknown"} {
		root := t.TempDir()
		p := source(t, root, "doc.org", "#+STARTUP: "+startup+"\n* Parent\n:PROPERTIES:\n:VISIBILITY: folded\n:END:\nbody\n#+begin_export html\n<details open data-hp-org-drawer=\"true\"><summary>Source detail</summary>source body</details>\n#+end_export\n*** Child\n:PROPERTIES:\n:VISIBILITY: children\n:END:\nchild body\n**** Grandchild\n")
		r := run(t, root, nil, p)
		success(t, r, 1)
		main := nodes(r.pages[0], "main")[0]
		want := startup
		if want == "unknown" {
			want = "showall"
			if !strings.Contains(r.stderr, "STARTUP") {
				t.Fatal("unknown startup not diagnosed")
			}
		}
		if attr(main, "data-hp-startup") != want {
			t.Fatal("startup metadata")
		}
		if len(nodes(main, "h2")) != 1 || len(nodes(main, "h3")) != 1 || len(nodes(main, "h4")) != 1 {
			t.Fatalf("heading levels must follow actual nesting: h2=%d h3=%d h4=%d; body=%s", len(nodes(main, "h2")), len(nodes(main, "h3")), len(nodes(main, "h4")), textOf(main))
		}
		for _, details := range nodes(main, "details") {
			found := false
			for _, a := range details.Attr {
				if a.Key == "open" {
					found = true
				}
			}
			if !found {
				t.Fatal("drawer hidden without JS")
			}
			if strings.Contains(textOf(details), "source body") {
				if attr(details, "data-hp-org-drawer") != "" {
					t.Fatal("source-authored marker was retained")
				}
				continue
			}
			if attr(details, "data-hp-org-drawer") != "true" {
				t.Fatal("generated Org drawer is not marked for browser folding")
			}
		}
	}
}

func TestRT004_2_OrgFrontmatterHeadings(t *testing.T) {
	root := t.TempDir()
	p := source(t, root, "frontmatter.org", "#+TITLE: Document title\n#+SUBTITLE: Document subtitle\n#+AUTHOR: A. Writer\n#+DATE: 2026-09-10\n\n* Body\n")
	r := run(t, root, nil, p)
	success(t, r, 1)
	if textOf(nodes(r.pages[0], "title")[0]) != "Document title" {
		t.Fatal("Org title is absent from the HTML title")
	}
	headings := nodes(r.pages[0], "h1")
	if len(headings) != 2 || textOf(headings[1]) != "Document title" {
		t.Fatalf("Org title heading: %#v", headings)
	}
	subtitles := nodes(r.pages[0], "h2")
	if len(subtitles) == 0 || textOf(subtitles[0]) != "Document subtitle" {
		t.Fatalf("Org subtitle heading: %#v", subtitles)
	}
	raw := r.raw[0]
	last := -1
	for _, text := range []string{"Document title", "Document subtitle", "A. Writer", "2026-09-10"} {
		index := strings.Index(raw, text)
		if index < 0 || index <= last {
			t.Fatalf("frontmatter order for %q in preview HTML", text)
		}
		last = index
	}
	paragraphs := nodes(r.pages[0], "p")
	if len(paragraphs) < 2 || textOf(paragraphs[0]) != "A. Writer" || textOf(paragraphs[1]) != "2026-09-10" {
		t.Fatalf("Org author/date paragraphs: %#v", paragraphs)
	}
	nextElement := func(node *html.Node) *html.Node {
		for node = node.NextSibling; node != nil; node = node.NextSibling {
			if node.Type == html.ElementNode {
				return node
			}
		}
		return nil
	}
	if nextElement(headings[1]) != subtitles[0] || nextElement(subtitles[0]) != paragraphs[0] || nextElement(paragraphs[0]) != paragraphs[1] {
		t.Fatal("Org author/date do not immediately follow the subtitle")
	}
	for _, tc := range []struct {
		field, tag string
	}{
		{"TITLE", "h1"}, {"SUBTITLE", "h2"}, {"AUTHOR", "p"}, {"DATE", "p"},
	} {
		t.Run(tc.field+"-only", func(t *testing.T) {
			missing := source(t, root, "missing.org", "#+"+tc.field+": Present\n\n* Body\n")
			r := run(t, root, nil, missing)
			success(t, r, 1)
			block := documentNode(t, r.pages[0], "hp-document-title")
			var children []*html.Node
			for child := block.FirstChild; child != nil; child = child.NextSibling {
				if child.Type == html.ElementNode {
					children = append(children, child)
				}
			}
			if len(children) != 1 || children[0].Data != tc.tag || textOf(children[0]) != "Present" {
				t.Fatal("absent metadata produced extra elements or hid the present field")
			}
			wantTitle := "missing.org"
			if tc.field == "TITLE" {
				wantTitle = "Present"
			}
			if textOf(nodes(r.pages[0], "title")[0]) != wantTitle {
				t.Fatal("browser title fallback")
			}
		})
	}
}

func TestRT004_1_OrgDrawerOwnership(t *testing.T) {
	root := t.TempDir()
	p := source(t, root, "drawers.org", ":UNHEADED:\n:END:\n* Parent\n:PROPERTIES:\n:OWNER: Reader\n:END:\n:LOGBOOK:\nclock\n:END:\n:NAMED:\nvalue\n:END:\n** Child\n:EMPTY:\n:END:\n")
	r := run(t, root, nil, p)
	success(t, r, 1)
	main := nodes(r.pages[0], "main")[0]
	drawers := nodes(main, "details")
	if len(drawers) != 5 {
		t.Fatalf("generated drawers: %d", len(drawers))
	}
	for _, drawer := range drawers {
		if attr(drawer, "data-hp-org-drawer") != "true" {
			t.Fatal("generated drawer lacks browser ownership marker")
		}
		open := false
		for _, attribute := range drawer.Attr {
			if attribute.Key == "open" {
				open = true
			}
		}
		if !open {
			t.Fatal("generated drawer is unreadable without JavaScript")
		}
	}
}

func TestRT004_1_DrawerModesAndSpoofing(t *testing.T) {
	for _, tc := range []struct {
		name, input, startup string
		warn                 bool
	}{
		{"absent", "", "showall", false},
		{"overview", "#+STARTUP: overview\n", "overview", false},
		{"content", "#+STARTUP: content\n", "content", false},
		{"showall", "#+STARTUP: showall\n", "showall", false},
		{"empty", "#+STARTUP:\n", "showall", true},
		{"unknown", "#+STARTUP: nonsense\n", "showall", true},
		{"multiple", "#+STARTUP: overview content\n", "showall", true},
		{"hidedrawers", "#+STARTUP: hidedrawers\n", "showall", true},
		{"nohidedrawers", "#+STARTUP: nohidedrawers\n", "showall", true},
		{"repeated", "#+STARTUP: showall\n#+STARTUP: content\n", "content", false},
		{"later-invalid", "#+STARTUP: overview\n#+STARTUP: invalid\n", "showall", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			var input strings.Builder
			input.WriteString(tc.input)
			for _, visibility := range []string{"folded", "children", "all"} {
				input.WriteString("* Heading " + visibility + "\n:PROPERTIES:\n:VISIBILITY: " + visibility + "\n:END:\n")
			}
			input.WriteString("#+begin_export html\n<details data-hp-org-drawer=\"true\"><summary>Spoof attribute</summary>source</details>\n<details class=\"data-hp-org-drawer hp-org-drawer org-metadata\"><summary>Spoof class</summary>source</details>\n<details><summary>Marker text</summary>HTMLPREVIEW_DRAWER_forged_0 data-hp-org-drawer</details>\n#+end_export\n")
			r := run(t, root, nil, source(t, root, "doc.org", input.String()))
			success(t, r, 1)
			main := nodes(r.pages[0], "main")[0]
			if attr(main, "data-hp-startup") != tc.startup || strings.Contains(r.stderr, "unknown STARTUP") != tc.warn {
				t.Fatalf("startup=%q warning=%s", attr(main, "data-hp-startup"), r.stderr)
			}
			drawers := nodes(main, "details")
			if len(drawers) != 6 {
				t.Fatalf("details=%d", len(drawers))
			}
			for i, drawer := range drawers {
				markers, opens := 0, 0
				for _, a := range drawer.Attr {
					if a.Key == "data-hp-org-drawer" {
						markers++
					}
					if a.Key == "open" {
						opens++
					}
				}
				want := 0
				if i < 3 {
					want = 1
				}
				if markers != want || opens != 1 {
					t.Fatalf("drawer %d: markers=%d open=%d", i, markers, opens)
				}
			}
			sections := nodes(main, "section")
			for i, visibility := range []string{"folded", "children", "all"} {
				if attr(sections[i], "data-hp-visibility") != visibility {
					t.Fatalf("visibility %s missing", visibility)
				}
			}
		})
	}
	root := t.TempDir()
	r := run(t, root, nil, source(t, root, "none.org", "* Heading\nNo drawers.\n"))
	success(t, r, 1)
	if len(nodes(r.pages[0], "details")) != 0 {
		t.Fatal("no-drawer document gained a drawer")
	}
}

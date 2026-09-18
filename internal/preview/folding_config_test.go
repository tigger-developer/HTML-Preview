// ABOUTME: Checks folding overrides at YAML and service registration boundaries.
// ABOUTME: Protects strict validation and separation of render capabilities.
package preview

import (
	"bytes"
	"golang.org/x/net/html"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func TestFoldingConfiguration(t *testing.T) {
	for _, tc := range []struct {
		body  string
		valid bool
	}{
		{"headers: {default: open, todo: open, done: closed}, drawers: closed, default: open", true},
		{"headers: {done: closed}", true},
		{"headers: {}", true},
		{"", true},
		{"headers: open", false},
		{"headers: closed", false},
		{"headers: showall", false},
		{"headers: {todo: showall}", false},
		{"headers: {done: true}", false},
		{"headers: {default: null}", false},
		{"headers: {closed: open}", false},
		{"headers: {todo: open, todo: closed}", false},
		{"headers: null", false},
		{"headers: [open]", false},
		{"drawers: true", false},
		{"default: null", false},
		{"header: open", false},
		{"headers: open, headers: closed", false},
	} {
		t.Run(tc.body, func(t *testing.T) {
			dir := t.TempDir()
			path := source(t, dir, "config.yaml", "version: 1\nfolding:\n  override: {"+tc.body+"}\n")
			_, err := serviceSettings(config{configPath: path, runtimePath: filepath.Join(dir, "runtime")})
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v: %v", tc.valid, err)
			}
			if tc.body == "headers: open" && (err == nil || !strings.Contains(err.Error(), "headers: {default: open}")) {
				t.Fatalf("missing scalar-header migration advice: %v", err)
			}
		})
	}
}

func TestFoldingSemanticHeadingClasses(t *testing.T) {
	for _, declaration := range []string{"TODO", "SEQ_TODO", "TYP_TODO"} {
		t.Run(declaration, func(t *testing.T) {
			dir := t.TempDir()
			cfg := source(t, dir, "preview.yaml", "version: 1\nfolding:\n  override:\n    headers: {default: open, todo: open, done: closed}\n")
			// Deliberately reverse the familiar words: folding must use categories.
			body := "#+" + declaration + ": DONE WAIT | TODO FINISHED\n* DONE Active one\nBody.\n* WAIT Active two\nBody.\n* TODO Complete one\nBody.\n* FINISHED Complete two\nBody.\n* Unmarked\nBody.\n"
			result := run(t, dir, []string{"HTMLPREVIEW_CONFIG=" + cfg}, source(t, dir, "tasks.org", body))
			success(t, result, 1)
			main := documentNode(t, result.pages[0], "hp-document")
			want := map[string]string{"DONE": "todo", "WAIT": "todo", "TODO": "done", "FINISHED": "done"}
			seen := 0
			for _, section := range nodes(main, "section") {
				heading := directHeading(section)
				if heading == nil {
					continue
				}
				for child := heading.FirstChild; child != nil; child = child.NextSibling {
					category, ok := want[textOf(child)]
					if !ok {
						continue
					}
					if !hasClass(child, category) {
						t.Fatalf("%s lacks semantic class %s", textOf(child), category)
					}
					seen++
				}
			}
			if seen != len(want) {
				t.Fatalf("want four semantic heading markers, got %d", seen)
			}
		})
	}
}

func TestFoldingFileAndServiceRendering(t *testing.T) {
	dir := t.TempDir()
	path := source(t, dir, "preview.yaml", "version: 1\nfolding:\n  override: {headers: {default: open, todo: open, done: closed}, drawers: closed, default: closed}\n")
	doc := source(t, dir, "entry.org", "#+TITLE: Folding\n#+STARTUP: overview\n* Heading\n:PROPERTIES:\n:VISIBILITY: folded\n:END:\nBody\n")
	result := run(t, dir, []string{"HTMLPREVIEW_CONFIG=" + path}, doc)
	success(t, result, 1)
	main := documentNode(t, result.pages[0], "hp-document")
	for field, want := range map[string]string{"headers-default": "open", "headers-todo": "open", "headers-done": "closed", "drawers": "closed", "default": "closed"} {
		if got := attr(main, "data-hp-fold-"+field); got != want {
			t.Fatalf("file override %s=%q want %q", field, got, want)
		}
	}
	svc := startTestService(t, NativeHost())
	source(t, svc.root, "linked.md", "# Linked heading\nText\n")
	body := "* Heading\n[[file:linked.md][Linked]]\n"
	first := svc.registerSettings(t, "entry.org", body, map[string]any{"folding": map[string]any{"headers": map[string]string{"default": "open", "todo": "open", "done": "closed"}, "drawers": "closed", "default": "closed"}})
	second := svc.registerSettings(t, "entry.org", body, map[string]any{"folding": map[string]any{"headers": map[string]string{"default": "open", "todo": "open", "done": "open"}, "drawers": "closed", "default": "closed"}})
	if first == second {
		t.Fatal("different folding preferences share a capability")
	}
	for _, tc := range []struct{ url, want string }{{first, "closed"}, {second, "open"}} {
		code, _, data := responseAsset(t, svc, "GET", tc.url)
		if code != 200 {
			t.Fatalf("service status=%d", code)
		}
		parsed, err := html.Parse(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if attr(documentNode(t, parsed, "hp-document"), "data-hp-fold-headers-done") != tc.want {
			t.Fatal("service lost initial folding preference")
		}
		for _, link := range nodes(parsed, "a") {
			if textOf(link) != "Linked" {
				continue
			}
			target, err := url.Parse(attr(link, "href"))
			if err != nil {
				t.Fatal(err)
			}
			base, err := url.Parse(tc.url)
			if err != nil {
				t.Fatal(err)
			}
			code, _, linked := responseAsset(t, svc, "GET", base.ResolveReference(target).String())
			dom, err := html.Parse(bytes.NewReader(linked))
			if code != 200 || err != nil || attr(documentNode(t, dom, "hp-document"), "data-hp-fold-headers-done") != tc.want {
				t.Fatal("linked preview lost inherited folding preference")
			}
		}
	}
}

func TestConfiguredHomeRoots(t *testing.T) {
	home := t.TempDir()
	for _, tc := range []struct {
		input, want string
		valid       bool
	}{
		{"~/docs", filepath.Join(home, "docs"), true},
		{"~", home, true},
		{"~someone/docs", "", false},
		{"relative/docs", "", false},
		{filepath.Join(home, "docs"), filepath.Join(home, "docs"), true},
	} {
		got, err := configuredRootPath(tc.input, home)
		if (err == nil) != tc.valid || (tc.valid && got != tc.want) {
			t.Fatalf("root %q => %q, %v", tc.input, got, err)
		}
	}
}

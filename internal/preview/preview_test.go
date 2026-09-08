// ABOUTME: Verifies preview behaviour through a subprocess and real Pandoc.
// ABOUTME: Only external desktop and forced-failure boundaries are replaced.
package preview

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

type handoff struct {
	URL                     string
	Path                    string
	DirectoryMode, FileMode uint32
}

func TestMain(m *testing.M) {
	if os.Getenv("PREVIEW_TEST_CHILD") != "1" {
		os.Exit(m.Run())
	}
	host := NativeHost()
	actual := host.Execute
	host.Execute = func(ctx context.Context, cmd Command) ([]byte, error) {
		if filepath.Base(cmd.Path) != "open" && filepath.Base(cmd.Path) != "xdg-open" {
			return actual(ctx, cmd)
		}
		if len(cmd.Args) != 1 {
			return nil, fmt.Errorf("opener must receive one URL")
		}
		u, err := url.Parse(cmd.Args[0])
		if err != nil {
			return nil, err
		}
		st, err := os.Stat(u.Path)
		if err != nil {
			return nil, err
		}
		dir, err := os.Stat(filepath.Dir(u.Path))
		if err != nil {
			return nil, err
		}
		entries, err := os.ReadDir(filepath.Dir(u.Path))
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if filepath.Ext(entry.Name()) != ".html" {
				continue
			}
			data, err := os.ReadFile(filepath.Join(filepath.Dir(u.Path), entry.Name()))
			if err != nil {
				return nil, err
			}
			if err := os.WriteFile(filepath.Join(os.Getenv("PREVIEW_TEST_CAPTURE"), entry.Name()), data, 0600); err != nil {
				return nil, err
			}
		}
		f, err := os.OpenFile(filepath.Join(os.Getenv("PREVIEW_TEST_CAPTURE"), "opens.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return nil, err
		}
		err = json.NewEncoder(f).Encode(handoff{cmd.Args[0], u.Path, uint32(dir.Mode().Perm()), uint32(st.Mode().Perm())})
		closeErr := f.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if os.Getenv("PREVIEW_TEST_OPEN_FAIL") == "1" {
			return nil, fmt.Errorf("forced opener failure")
		}
		return nil, nil
	}
	os.Exit(Main(os.Args[1:], os.Environ(), os.Stdout, os.Stderr, "test", "test-revision", host))
}

type result struct {
	code           int
	stdout, stderr string
	pages          []*html.Node
	raw            []string
	opens          []handoff
}

func source(t *testing.T, root, name, content string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func run(t *testing.T, root string, settings []string, args ...string) result {
	t.Helper()
	capture := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, args...)
	cmd.Dir = root
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "HTMLPREVIEW_") {
			cmd.Env = append(cmd.Env, v)
		}
	}
	cmd.Env = append(cmd.Env, "PWD="+root, "PREVIEW_TEST_CHILD=1", "PREVIEW_TEST_CAPTURE="+capture, "HTMLPREVIEW_GRACE=100ms")
	cmd.Env = append(cmd.Env, settings...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	r := result{stdout: stdout.String(), stderr: stderr.String()}
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			r.code = exit.ExitCode()
		} else {
			t.Fatal(err)
		}
	}
	f, err := os.Open(filepath.Join(capture, "opens.jsonl"))
	if err == nil {
		decoder := json.NewDecoder(f)
		for {
			var h handoff
			err := decoder.Decode(&h)
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			r.opens = append(r.opens, h)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(capture)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".html" {
			data, err := os.ReadFile(filepath.Join(capture, entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			doc, err := html.Parse(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			r.pages = append(r.pages, doc)
			r.raw = append(r.raw, string(data))
		}
	}
	return r
}

func nodes(n *html.Node, tag string) []*html.Node {
	var result []*html.Node
	for x := range n.Descendants() {
		if x.Type == html.ElementNode && x.Data == tag {
			result = append(result, x)
		}
	}
	return result
}
func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
func textOf(n *html.Node) string {
	var s strings.Builder
	for x := range n.Descendants() {
		if x.Type == html.TextNode {
			s.WriteString(x.Data)
		}
	}
	return s.String()
}
func success(t *testing.T, r result, pages int) {
	t.Helper()
	if r.code != 0 || len(r.pages) != pages {
		t.Fatalf("status=%d pages=%d expected=%d stderr=%s", r.code, len(r.pages), pages, r.stderr)
	}
}

func TestRT001_1_SourceIdentity(t *testing.T) {
	root := t.TempDir()
	name := "a/Taḋg  & <notes>.md"
	a := source(t, root, name, "# A\n")
	b := source(t, root, "b/notes.org", "* B\n")
	empty := source(t, root, "empty.MARKDOWN", "")
	alias := filepath.Join(root, "alias.md")
	if err := os.Symlink(a, alias); err != nil {
		t.Fatal(err)
	}
	r := run(t, root, nil, name, b, a, empty, alias)
	success(t, r, 4)
	want := []string{a, b, empty, alias}
	if len(r.opens) != len(want) {
		t.Fatalf("open order: %+v", r.opens)
	}
	for i, path := range want {
		h := nodes(r.pages[i], "h1")
		if len(h) != 1 || textOf(h[0]) != path {
			t.Fatalf("source header %d: %v expected %q", i, h, path)
		}
		if attr(h[0], "data-hp-source") != path {
			t.Fatalf("copy value differs from logical path")
		}
		strong := nodes(h[0], "strong")
		if len(strong) != 1 || textOf(strong[0]) != filepath.Base(path) {
			t.Fatal("basename not emphasized")
		}
		title := nodes(r.pages[i], "title")
		if len(title) != 1 || textOf(title[0]) != filepath.Base(path) {
			t.Fatal("tab identity")
		}
	}
}

func TestRT001_2_Ownership(t *testing.T) {
	root := t.TempDir()
	path := source(t, root, "readonly/doc.md", "unchanged")
	sentinel := source(t, root, "sentinel/keep.txt", "keep")
	if err := os.Chmod(filepath.Dir(path), 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(filepath.Dir(path), 0700); err != nil {
			t.Error(err)
		}
	})
	r := run(t, root, nil, path)
	success(t, r, 1)
	for _, h := range r.opens {
		if h.DirectoryMode != 0700 || h.FileMode != 0600 {
			t.Fatalf("permissions: %+v", h)
		}
		if _, err := os.Stat(h.Path); !os.IsNotExist(err) {
			t.Fatalf("owned output not cleaned: %v", err)
		}
	}
	for p, want := range map[string]string{path: "unchanged", sentinel: "keep"} {
		got, err := os.ReadFile(p)
		if err != nil || string(got) != want {
			t.Fatalf("source changed: %s", p)
		}
	}
}

func TestRT001_3_Markdown(t *testing.T) {
	root := t.TempDir()
	p := source(t, root, "doc.md", "# Heading\n\nParagraph *emphasis* **strong** `code`.\n\n> Quote\n\n- first\n    - nested\n\n1. ordered\n\n| Head | Other |\n|---|---|\n| cell | value |\n\n```text\n\ttabbed\n```\n\n![alt](image.png)\n\nFootnote.[^n]\n\n[^n]: Note\n")
	r := run(t, root, nil, p)
	success(t, r, 1)
	for _, tag := range []string{"h2", "em", "strong", "code", "blockquote", "ul", "ol", "table", "td", "pre", "img"} {
		if len(nodes(r.pages[0], tag)) == 0 {
			t.Errorf("missing %s", tag)
		}
	}
	if attr(nodes(r.pages[0], "img")[0], "alt") != "alt" {
		t.Fatal("image alternative")
	}
	if !strings.Contains(textOf(nodes(r.pages[0], "pre")[0]), "\ttabbed") {
		t.Fatal("literal tab changed")
	}
}

func TestRT001_4_OrgPreservation(t *testing.T) {
	root := t.TempDir()
	literal := "\tSCHEDULED: <2026-09-08 Tue>\n:LOGBOOK:\n#+INCLUDE: secret.org\n"
	p := source(t, root, "doc.org", "#+TODO: WAIT(w) | FINISHED(f)\n* WAIT [#A] A [1/3] :tag:\nSCHEDULED: <2026-09-08 Tue 09:00-10:00 +1w -2d>\n:PROPERTIES:\n:ID: internal\n:CUSTOM_ID: public\n:OWNER: Reader\n:END:\n:LOGBOOK:\nclock entry\n:END:\n:LOGBOOK_:\nother drawer\n:END:\n#+begin_src sh\n"+literal+"#+end_src\n#+begin_example\n"+literal+"#+end_example\n")
	r := run(t, root, nil, p)
	success(t, r, 1)
	body := textOf(nodes(r.pages[0], "body")[0])
	for _, s := range []string{"WAIT", "FINISHED", "[#A]", "[1/3]", "tag", "SCHEDULED: <2026-09-08 Tue 09:00-10:00 +1w -2d>", "internal", "public", "OWNER", "Reader", "LOGBOOK_", "clock entry", "other drawer"} {
		if !strings.Contains(body, s) {
			t.Errorf("Org value missing %q", s)
		}
	}
	found := 0
	for _, pre := range nodes(r.pages[0], "pre") {
		if textOf(pre) == literal {
			found++
		}
	}
	if found != 2 {
		t.Fatalf("literal blocks preserved=%d expected=2", found)
	}
}

func TestRT001_5_FontPayloads(t *testing.T) {
	root := t.TempDir()
	p := source(t, root, "doc.md", "fonts")
	r := run(t, root, nil, p)
	success(t, r, 1)
	fonts := regexp.MustCompile(`data:font/woff2;base64,([A-Za-z0-9+/=]+)`).FindAllStringSubmatch(r.raw[0], -1)
	want := map[string]bool{"2a03eab9fff645ef7ffcb6f9c81065a7cbd2d76d87873a8cc5bf4f23d97fdf7a": true, "346d8f2e0c37b3cb777dbe9b759a5b042b1b459db28c0f285760678241ef3f17": true, "974f9d6cf94f8c8c55a279ffd7cbe5aab00c9df63bdeffe7c2450b8820cf756e": true, "ef85723a68f371f853604a8580de306dffc6c6c8d46822a2e1d8f9e474ed7b40": true, "9ab02901db66ac603ca9f08cc99a33199a632f176cdfb76d0cdc8bbb4f58d87b": true, "226bd10650c64a3cd576d358391a4eb6522948a96b169b89b1deb6353c838134": true}
	if len(fonts) != 6 {
		t.Fatalf("font faces=%d", len(fonts))
	}
	for _, match := range fonts {
		data, err := base64.StdEncoding.DecodeString(match[1])
		if err != nil {
			t.Fatal(err)
		}
		hash := fmt.Sprintf("%x", sha256.Sum256(data))
		if !want[hash] {
			t.Fatalf("unexpected font hash %s", hash)
		}
		delete(want, hash)
	}
	for _, file := range []string{"../../LICENSE", "../../assets/fonts/asap/OFL.txt", "../../assets/fonts/iosevka-custom/OFL.md"} {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(r.raw[0], string(data)) {
			t.Errorf("missing full notice %s", file)
		}
	}
}

func TestRT001_9_Invocation(t *testing.T) {
	root := t.TempDir()
	for _, args := range [][]string{{"--help"}, {"-h"}, {"--version"}} {
		r := run(t, root, []string{"HTMLPREVIEW_BAD=ignored", "PATH="}, args...)
		success(t, r, 0)
		if r.stdout == "" {
			t.Fatal("missing information")
		}
	}
	for _, args := range [][]string{nil, {"--unknown"}, {"--help", "x.md"}, {"x.md", "--bad"}, {"--version", "--help"}} {
		r := run(t, root, nil, args...)
		if r.code != 2 {
			t.Errorf("%v status %d", args, r.code)
		}
	}
	for _, s := range []string{"HTMLPREVIEW_BAD=x", "HTMLPREVIEW_LINKS=yes", "HTMLPREVIEW_GRACE=99ms", "HTMLPREVIEW_MAX_DEPTH=-1", "HTMLPREVIEW_MAX_FILES=501", "HTMLPREVIEW_MAX_OUTPUT_BYTES=0", "HTMLPREVIEW_DEADLINE=11m", "HTMLPREVIEW_MODE=other"} {
		r := run(t, root, []string{s}, "doc.md")
		if r.code != 2 {
			t.Errorf("%s status %d", s, r.code)
		}
	}
}

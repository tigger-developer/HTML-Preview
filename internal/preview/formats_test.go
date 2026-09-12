// ABOUTME: Exercises broader input formats through the public CLI and real Pandoc.
// ABOUTME: Keeps original text, selected formats and generated presentation independently observable.
package preview

import (
	"encoding/base64"
	"path/filepath"
	"strings"
	"testing"
)

func TestRT008_3_CodeAndTextWrappers(t *testing.T) {
	for _, c := range []struct {
		name, body, language string
		heading              bool
	}{
		{"hello.go", "package main\nfunc main() { println(42) }\n", "go", true},
		{"notes.txt", "* Literal\n<b>text</b>\n", "plaintext", false},
		{"empty.txt", "", "plaintext", false},
		{"Taḋg *literal*\n#+AUTHOR: injected.go", "package main\n", "go", true},
		{"CMakeLists.txt", "set(VALUE 42)\n", "cmake", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			path := source(t, root, c.name, c.body)
			r := run(t, t.TempDir(), nil, path)
			success(t, r, 1)
			if textOf(nodes(r.pages[0], "title")[0]) != c.name {
				t.Fatal("original basename lost in title")
			}
			main := documentNode(t, r.pages[0], "hp-document")
			blocks := nodes(main, "pre")
			if len(blocks) != 1 || textOf(blocks[0]) != c.body {
				t.Fatalf("literal body differs: %v", blocks)
			}
			heads := nodes(main, "h2")
			if c.heading && (len(heads) != 1 || textOf(heads[0]) != "Source code") {
				t.Fatal("missing unique source heading")
			}
			if !c.heading && len(heads) != 0 {
				t.Fatal("plaintext gained a synthetic heading")
			}
			if c.body != "" && c.language != "plaintext" && len(nodes(blocks[0], "span")) == 0 {
				t.Fatal("code was not highlighted")
			}
			fm := documentNode(t, r.pages[0], "hp-frontmatter")
			keys, values := nodes(fm, "dt"), nodes(fm, "dd")
			if len(keys) != 2 || len(values) != 2 || textOf(values[0]) != c.name || textOf(values[1]) != "HTML-Preview version test" {
				t.Fatal("wrapper metadata differs or filename injected fields")
			}
			if attr(documentNode(t, r.pages[0], "hp-source"), "data-hp-source") != path {
				t.Fatal("temporary wrapper became source identity")
			}
		})
	}
}

func TestRT008_4_LiteralPayload(t *testing.T) {
	inputs := []string{"\ufeffa\r\n\tb\r\n", "no final newline", "#+END_SRC\n  #+eNd_sRc\n\t#+END_SRC\n,#+END_SRC\n,,#+END_SRC\n* Heading\n,* Comma\n#+END_EXPORT\n<script>bad()</script>\n", "\n\n", ""}
	for _, suffix := range []string{".txt", ".go"} {
		for _, original := range inputs {
			root := t.TempDir()
			p := source(t, root, "literal"+suffix, original)
			r := run(t, root, nil, p)
			success(t, r, 1)
			main := documentNode(t, r.pages[0], "hp-document")
			blocks := nodes(main, "pre")
			want := strings.TrimPrefix(strings.ReplaceAll(original, "\r\n", "\n"), "\ufeff")
			if len(blocks) != 1 || textOf(blocks[0]) != want {
				t.Fatalf("display != original normalized text %q", want)
			}
			codes := nodes(blocks[0], "code")
			copyText, err := base64.StdEncoding.DecodeString(attr(codes[0], "data-hp-copy-base64"))
			if err != nil || string(copyText) != original {
				t.Fatalf("copy payload %q != %q: %v", copyText, original, err)
			}
			if len(nodes(main, "script")) != 0 || len(nodes(main, "a")) != 0 {
				t.Fatal("code escaped its literal block")
			}
		}
	}
	for _, invalid := range []string{"nul\x00", "\xff"} {
		root := t.TempDir()
		r := run(t, root, nil, source(t, root, "bad.txt", invalid))
		if r.code != 1 || len(r.opens) != 0 {
			t.Fatal("invalid text accepted")
		}
	}
}

func TestRT008_5_JSONFormatting(t *testing.T) {
	for _, c := range []struct {
		input, display string
		invalid        bool
	}{
		{`{"b":123456789012345678901234567890,"a":1e-009,"b":-0}`, "{\n  \"b\": 123456789012345678901234567890,\n  \"a\": 1e-009,\n  \"b\": -0\n}", false},
		{`{"a":[true,null,"{}\\\""]}`, "{\n  \"a\": [\n    true,\n    null,\n    \"{}\\\\\\\"\"\n  ]\n}", false},
		{"{invalid\n", "{invalid\n", true},
	} {
		root := t.TempDir()
		r := run(t, root, nil, source(t, root, "data.json", c.input))
		success(t, r, 1)
		code := nodes(nodes(documentNode(t, r.pages[0], "hp-document"), "pre")[0], "code")[0]
		if textOf(code) != c.display {
			t.Fatalf("JSON display: %q want %q", textOf(code), c.display)
		}
		payload, err := base64.StdEncoding.DecodeString(attr(code, "data-hp-copy-base64"))
		if err != nil || string(payload) != c.input {
			t.Fatal("JSON original copy lost")
		}
		if c.invalid && !strings.Contains(r.stderr, "invalid JSON") {
			t.Fatal("missing invalid JSON notice")
		}
	}
}

func TestRT008_2_ReaderSelection(t *testing.T) {
	root := t.TempDir()
	path := source(t, root, "document.unknown", "# Selected\n")
	for _, args := range [][]string{{"--from=markdown", path}, {"--from", "markdown+smart", path}} {
		r := run(t, root, nil, args...)
		success(t, r, 1)
		if !strings.Contains(textOf(documentNode(t, r.pages[0], "hp-document")), "Selected") {
			t.Fatal("selected reader content missing")
		}
	}
	for _, args := range [][]string{{"-f", "markdown", path}, {"--from=html+raw_html", path}, {"--from=../reader.lua", path}, {"--from=markdown+no_such_extension", path}, {"--from", "markdown", "--from=org", path}, {"--list-input-formats", path}} {
		r := run(t, root, nil, args...)
		if r.code != 2 || len(r.opens) != 0 {
			t.Fatalf("invalid selection %v: status=%d %s", args, r.code, r.stderr)
		}
	}
	r := run(t, root, nil, "--list-input-formats")
	success(t, r, 0)
	for _, reader := range []string{"org", "markdown", "docx", "odt", "json"} {
		if !strings.Contains("\n"+r.stdout, "\n"+reader+"\n") {
			t.Fatalf("reader missing: %s", reader)
		}
	}
	r = run(t, root, nil, filepath.Base(path))
	if r.code != 1 || !strings.Contains(r.stderr, "--from") {
		t.Fatal("unknown suffix lacks selection guidance")
	}
}

func TestRT008_2_ReaderSelectionLengthAndLiteralFilename(t *testing.T) {
	root := t.TempDir()
	path := source(t, root, "document.unknown", "# Bounded reader\n")
	valid := "markdown" + strings.Repeat("+smart", 40) + "+raw_tex"
	oversized := "markdown" + strings.Repeat("+smart", 40) + "+raw_html"
	if len(valid) != 256 || len(oversized) != 257 {
		t.Fatal("fixture no longer exercises the reader length boundary")
	}
	r := run(t, root, nil, "--from="+valid, path)
	success(t, r, 1)
	r = run(t, root, nil, "--from="+oversized, path)
	if r.code != 2 || len(r.opens) != 0 || !strings.Contains(r.stderr, "invalid --from") {
		t.Fatal("oversized otherwise valid reader selection accepted")
	}
	source(t, root, "--literal.txt", "Literal option-shaped filename")
	r = run(t, root, nil, "--", "--literal.txt")
	success(t, r, 1)
	if textOf(nodes(documentNode(t, r.pages[0], "hp-document"), "pre")[0]) != "Literal option-shaped filename" {
		t.Fatal("option-shaped filename was not previewed literally")
	}
}

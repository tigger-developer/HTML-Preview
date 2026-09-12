// ABOUTME: Exercises every approved filename mapping through the CLI conversion boundary.
// ABOUTME: Keeps fixture expectations independent of the production resolver tables.
package preview

import (
	"strings"
	"testing"
)

// Rows are the signed-off W008 code table; they are test inputs, not a view of
// production mappings. Every row asserts literal output and the actual wrapper.
const codeCases = `bash .sh .bash
zsh .zsh
plaintext .fish .vim
perl .pl .pm
python .py .pyw
go .go
java .java
javascript .js .mjs .cjs
javascriptreact .jsx
typescript .ts .tsx
lua .lua
c .c .h
cpp .cc .cpp .cxx .hh .hpp .hxx
cs .cs
rust .rs
ruby .rb .rake Rakefile Gemfile
php .php
swift .swift
kotlin .kt .kts
scala .scala .sc
r .r
julia .jl
dart .dart
elixir .ex .exs
erlang .erl .hrl
haskell .hs
clojure .clj .cljs .cljc .edn
commonlisp .lisp .lsp
scheme .scm .ss
ocaml .ml .mli
fsharp .fs .fsx
sql .sql
powershell .ps1 .psm1 .psd1
dosbat .bat .cmd
css .css
scss .scss
sass .sass
json .json
yaml .yaml .yml
toml .toml
ini .ini .cfg .conf
xml .xml .xsd .xsl .xslt
graphql .graphql .gql
protobuf .proto
terraform .tf .tfvars
nix .nix
cmake .cmake CMakeLists.txt
makefile .mk Makefile makefile GNUmakefile
dockerfile Dockerfile Containerfile .dockerfile
diff .diff .patch
zig .zig
nim .nim
tcl .tcl
fortranfree .f90 .f95 .f03 .f08`

func TestRT008_2_AllCodeMappings(t *testing.T) {
	for _, row := range strings.Split(codeCases, "\n") {
		fields := strings.Fields(row)
		t.Run(fields[0], func(t *testing.T) {
			root := t.TempDir()
			var files []string
			for _, name := range fields[1:] {
				if strings.HasPrefix(name, ".") {
					name = "source" + strings.ToUpper(name)
				}
				files = append(files, source(t, root, name, `"literal fixture"`))
			}
			r := run(t, root, nil, files...)
			success(t, r, len(files))
			if len(r.wrappers) != len(files) {
				t.Fatal("missing private Org wrappers")
			}
			for i, wrapper := range r.wrappers {
				if !strings.Contains(wrapper, "\n#+BEGIN_SRC "+fields[0]+"\n") {
					t.Fatalf("incorrect source language for %s", files[i])
				}
				code := nodes(documentNode(t, r.pages[i], "hp-document"), "pre")
				if len(code) != 1 || textOf(code[0]) != `"literal fixture"` {
					t.Fatalf("literal content lost for %s", files[i])
				}
			}
		})
	}
}

const documentCases = `markdown .md .markdown .mdown .mkd .mkdn
org .org
asciidoc .adoc .asciidoc
bibtex .bib .bibtex
biblatex .biblatex
bits .bits
commonmark .commonmark
commonmark_x .commonmark_x
creole .creole
csljson .csl.json
csv .csv
tsv .tsv
djot .dj .djot
docbook .dbk .docbook
docx .docx
dokuwiki .dokuwiki
endnotexml .endnote.xml .endnotexml
epub .epub
fb2 .fb2
gfm .gfm
haddock .haddock
ipynb .ipynb
jats .jats .jats.xml
jira .jira
latex .tex .latex .ltx
man .man .1 .2 .3 .4 .5 .6 .7 .8 .9
mdoc .mdoc
mediawiki .mediawiki .wiki
muse .muse
native .native
odt .odt
opml .opml
pod .pod
pptx .pptx
ris .ris
rst .rst .rest
rtf .rtf
t2t .t2t
textile .textile
tikiwiki .tikiwiki
twiki .twiki
typst .typ .typst
vimwiki .vimwiki
xlsx .xlsx`

func TestRT008_2_AllDocumentMappings(t *testing.T) {
	for _, row := range strings.Split(documentCases, "\n") {
		fields := strings.Fields(row)
		t.Run(fields[0], func(t *testing.T) {
			root := t.TempDir()
			data := readerFixture(t, root, fields[0], false)
			var files []string
			for _, suffix := range fields[1:] {
				files = append(files, source(t, root, "document"+strings.ToUpper(suffix), string(data)))
			}
			r := run(t, root, nil, files...)
			success(t, r, len(files))
			for i, doc := range r.pages {
				if !strings.Contains(textOf(nodes(doc, "body")[0]), "Reader fixture") {
					t.Fatalf("detected document content lost for %s", files[i])
				}
			}
		})
	}
}

func TestRT008_2_SelectionDoesNotPropagate(t *testing.T) {
	root := t.TempDir()
	parent := source(t, root, "parent.json", "[self](parent.json) [child](child.org)\n")
	source(t, root, "child.org", "* Org child\n")
	r := run(t, root, []string{"HTMLPREVIEW_LINKS=1"}, "--from=markdown", parent)
	success(t, r, 3)
	if len(r.wrappers) != 1 || !strings.Contains(r.wrappers[0], "#+BEGIN_SRC json") {
		t.Fatal("selected entry reader leaked into linked representation")
	}
	if !strings.Contains(textOf(documentNode(t, r.pages[2], "hp-document")), "Org child") {
		t.Fatal("linked Org reader lost")
	}
}

func TestRT008_3_MissingHighlighter(t *testing.T) {
	root := t.TempDir()
	r := run(t, root, []string{"PREVIEW_TEST_FAULT=missing-highlighter"}, source(t, root, "source.go", "package main\n"))
	success(t, r, 1)
	if len(r.wrappers) != 1 || !strings.Contains(r.wrappers[0], "#+BEGIN_SRC plaintext") || !strings.Contains(r.stderr, "highlighting language") {
		t.Fatal("missing highlighter did not preserve literal code with a notice")
	}
}

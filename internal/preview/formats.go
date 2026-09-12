// ABOUTME: Selects bounded input routes from installed Pandoc capabilities and filename mappings.
// ABOUTME: Carries original source kind separately from rendering intermediates for transport reuse.
package preview

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

type inputFormat struct {
	kind, reader, language string
}

func (f inputFormat) key() string   { return f.kind + ":" + f.reader + ":" + f.language }
func (f inputFormat) wrapped() bool { return f.kind == "code" || f.kind == "plaintext" }
func (f inputFormat) binary() bool {
	switch readerBase(f.reader) {
	case "docx", "odt", "epub", "pptx", "xlsx":
		return true
	}
	return false
}

type formatCatalogue struct {
	readers, languages map[string]bool
	listing            string
}

func discoverFormats(ctx context.Context, host Host, pandoc string) (*formatCatalogue, error) {
	readers, err := helper(ctx, host, pandoc, []string{"--list-input-formats"}, nil)
	if err != nil {
		return nil, fmt.Errorf("list Pandoc input formats: %w", err)
	}
	languages, err := helper(ctx, host, pandoc, []string{"--list-highlight-languages"}, nil)
	if err != nil {
		return nil, fmt.Errorf("list Pandoc highlighting languages: %w", err)
	}
	c := &formatCatalogue{readers: make(map[string]bool), languages: make(map[string]bool), listing: string(readers)}
	for _, reader := range strings.Fields(string(readers)) {
		if !regexp.MustCompile(`^[a-z][a-z0-9_]*$`).MatchString(reader) {
			return nil, fmt.Errorf("invalid installed reader name")
		}
		c.readers[reader] = true
	}
	if len(c.readers) == 0 {
		return nil, fmt.Errorf("Pandoc reported no input readers")
	}
	for _, language := range strings.Fields(string(languages)) {
		c.languages[language] = true
	}
	return c, nil
}

func readerBase(value string) string {
	base, _, _ := strings.Cut(value, "+")
	base, _, _ = strings.Cut(base, "-")
	return base
}

func (c *formatCatalogue) validateSelection(ctx context.Context, host Host, pandoc, selection string) error {
	if len(selection) > 256 || !regexp.MustCompile(`^[a-z][a-z0-9_]*([+-][a-z][a-z0-9_]*)*$`).MatchString(selection) {
		return fmt.Errorf("invalid --from reader; use --list-input-formats")
	}
	base := readerBase(selection)
	if !c.readers[base] {
		return fmt.Errorf("--from reader %q is unavailable; use --list-input-formats", base)
	}
	qualifiers := selection[len(base):]
	if qualifiers == "" {
		return nil
	}
	if base == "html" {
		return fmt.Errorf("--from=html does not accept reader extensions")
	}
	data, err := helper(ctx, host, pandoc, []string{"--list-extensions=" + base}, nil)
	if err != nil {
		return fmt.Errorf("query extensions for %s: %w", base, err)
	}
	allowed := make(map[string]bool)
	for _, ext := range strings.Fields(string(data)) {
		allowed[strings.TrimLeft(ext, "+-")] = true
	}
	for _, ext := range strings.FieldsFunc(qualifiers, func(r rune) bool { return r == '+' || r == '-' }) {
		if !allowed[ext] {
			return fmt.Errorf("unsupported --from extension %q for %s", ext, base)
		}
	}
	return nil
}

type formatMapping struct{ names, value string }

var documentMappings = []formatMapping{
	{".md .markdown .mdown .mkd .mkdn", "markdown"}, {".org", "org"}, {".adoc .asciidoc", "asciidoc"},
	{".bib .bibtex", "bibtex"}, {".biblatex", "biblatex"}, {".bits", "bits"}, {".commonmark", "commonmark"},
	{".commonmark_x", "commonmark_x"}, {".creole", "creole"}, {".csl.json", "csljson"}, {".csv", "csv"},
	{".tsv", "tsv"}, {".dj .djot", "djot"}, {".dbk .docbook", "docbook"}, {".docx", "docx"},
	{".dokuwiki", "dokuwiki"}, {".endnote.xml .endnotexml", "endnotexml"}, {".epub", "epub"}, {".fb2", "fb2"},
	{".gfm", "gfm"}, {".haddock", "haddock"}, {".ipynb", "ipynb"}, {".jats .jats.xml", "jats"}, {".jira", "jira"},
	{".tex .latex .ltx", "latex"}, {".man .1 .2 .3 .4 .5 .6 .7 .8 .9", "man"}, {".mdoc", "mdoc"},
	{".mediawiki .wiki", "mediawiki"}, {".muse", "muse"}, {".native", "native"}, {".odt", "odt"}, {".opml", "opml"},
	{".pod", "pod"}, {".pptx", "pptx"}, {".ris", "ris"}, {".rst .rest", "rst"}, {".rtf", "rtf"}, {".t2t", "t2t"},
	{".textile", "textile"}, {".tikiwiki", "tikiwiki"}, {".twiki", "twiki"}, {".typ .typst", "typst"},
	{".vimwiki", "vimwiki"}, {".xlsx", "xlsx"},
}

var codeMappings = []formatMapping{
	{".sh .bash", "bash"}, {".zsh", "zsh"}, {".fish", "plaintext"}, {".pl .pm", "perl"}, {".py .pyw", "python"},
	{".go", "go"}, {".java", "java"}, {".js .mjs .cjs", "javascript"}, {".jsx", "javascriptreact"}, {".ts .tsx", "typescript"},
	{".lua", "lua"}, {".c .h", "c"}, {".cc .cpp .cxx .hh .hpp .hxx", "cpp"}, {".cs", "cs"}, {".rs", "rust"},
	{".rb .rake Rakefile Gemfile", "ruby"}, {".php", "php"}, {".swift", "swift"}, {".kt .kts", "kotlin"},
	{".scala .sc", "scala"}, {".r", "r"}, {".jl", "julia"}, {".dart", "dart"}, {".ex .exs", "elixir"}, {".erl .hrl", "erlang"},
	{".hs", "haskell"}, {".clj .cljs .cljc .edn", "clojure"}, {".lisp .lsp", "commonlisp"}, {".scm .ss", "scheme"},
	{".ml .mli", "ocaml"}, {".fs .fsx", "fsharp"}, {".sql", "sql"}, {".ps1 .psm1 .psd1", "powershell"},
	{".bat .cmd", "dosbat"}, {".css", "css"}, {".scss", "scss"}, {".sass", "sass"}, {".json", "json"},
	{".yaml .yml", "yaml"}, {".toml", "toml"}, {".ini .cfg .conf", "ini"}, {".xml .xsd .xsl .xslt", "xml"},
	{".graphql .gql", "graphql"}, {".proto", "protobuf"}, {".tf .tfvars", "terraform"}, {".nix", "nix"},
	{".cmake CMakeLists.txt", "cmake"}, {".mk Makefile makefile GNUmakefile", "makefile"},
	{"Dockerfile Containerfile .dockerfile", "dockerfile"}, {".diff .patch", "diff"}, {".vim", "plaintext"},
	{".zig", "zig"}, {".nim", "nim"}, {".tcl", "tcl"}, {".f90 .f95 .f03 .f08", "fortranfree"},
}

func documentFormat(reader string) inputFormat {
	kind := "document"
	switch readerBase(reader) {
	case "org", "markdown", "html":
		kind = readerBase(reader)
	}
	return inputFormat{kind: kind, reader: reader}
}

func (c *formatCatalogue) resolve(path, selected string) (inputFormat, error) {
	if selected != "" {
		return documentFormat(selected), nil
	}
	name := filepath.Base(path)
	ext := strings.ToLower(filepath.Ext(name))
	if ext == ".html" || ext == ".htm" || ext == ".xhtml" {
		return documentFormat("html"), nil
	}
	for _, mapping := range codeMappings {
		for _, candidate := range strings.Fields(mapping.names) {
			if candidate[0] != '.' && candidate == name {
				return inputFormat{kind: "code", language: mapping.value}, nil
			}
		}
	}
	best := 0
	f := inputFormat{}
	if ext == ".txt" {
		best = 4
		f = inputFormat{kind: "plaintext", language: "plaintext"}
	}
	for _, mapping := range documentMappings {
		for _, suffix := range strings.Fields(mapping.names) {
			if len(suffix) > best && strings.HasSuffix(strings.ToLower(name), suffix) {
				best = len(suffix)
				f = documentFormat(mapping.value)
			}
		}
	}
	for _, mapping := range codeMappings {
		for _, suffix := range strings.Fields(mapping.names) {
			if suffix[0] == '.' && len(suffix) >= best && strings.HasSuffix(strings.ToLower(name), suffix) {
				best = len(suffix)
				f = inputFormat{kind: "code", language: mapping.value}
			}
		}
	}
	if best == 0 && c.readers[strings.TrimPrefix(ext, ".")] {
		f = documentFormat(strings.TrimPrefix(ext, "."))
		best = len(ext)
	}
	if best == 0 {
		return f, fmt.Errorf("unknown input format; select --from FORMAT or use --list-input-formats")
	}
	if f.reader != "" && !c.readers[f.reader] {
		return f, fmt.Errorf("Pandoc reader %q is unavailable; use --list-input-formats", f.reader)
	}
	return f, nil
}

func identifyFormat(path, root, selected string, catalogue *formatCatalogue) (sourceContext, error) {
	f, err := catalogue.resolve(path, selected)
	if err != nil {
		return sourceContext{}, err
	}
	src, err := identify(path, root)
	if err != nil {
		return src, err
	}
	src.input = f
	src.selected = selected != ""
	src.key += "\x00" + f.key()
	return src, nil
}

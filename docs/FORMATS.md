# Supported input formats

Version: 1  
Updated: 12 September 2026

htmlpreview previews documents using the installed Pandoc readers, wraps common
source code and plain text, and preserves passive authored HTML. The W008
candidate is under delivery; its [validation record](../specs/008-input-formats/validation.org)
distinguishes development checks from browser qualification.

## Selecting an input

```sh
htmlpreview report.docx notes.odt source.go README.txt
```

```sh
htmlpreview --list-input-formats
```

```sh
htmlpreview --from=markdown+smart document.unusual
```

`--from FORMAT` and `--from=FORMAT` are equivalent. One selection applies to
all explicitly supplied files. Linked documents select their own format.
The short `-f` option is refused. Use `--` before a filename beginning with
a dash. Help and version require no Pandoc installation; reader listing does.

Selection precedence is:

1. An explicitly selected built-in reader.
2. Native HTML suffixes: `.html`, `.htm`, `.xhtml`.
3. An exact code basename from the code table.
4. The longest matching document, code or `.txt` suffix.
5. An installed reader whose name exactly matches the suffix.
6. An error suggesting explicit selection.

Suffixes are case-insensitive; named basenames are exact. A code mapping wins
an equal-length conflict. Thus `.csl.json` is a bibliography, `.json` is
ordinary JSON code, and `CMakeLists.txt` is CMake code. Ambiguous `.m` is
unmapped. Reader availability comes from Pandoc, so a table entry can report
that the installed dependency does not provide its reader.

Only installed built-in readers and their supported extension qualifiers are
accepted. A selection is limited to 256 ASCII characters. Custom reader paths,
filters, executable code and arbitrary Pandoc options are excluded.

## Document aliases

| Filename suffix | Reader |
|-----------------|--------|
| .md, .markdown, .mdown, .mkd, .mkdn | markdown |
| .org | org |
| .adoc, .asciidoc | asciidoc |
| .bib, .bibtex | bibtex |
| .biblatex | biblatex |
| .bits | bits |
| .commonmark | commonmark |
| .commonmark_x | commonmark_x |
| .creole | creole |
| .csl.json | csljson |
| .csv | csv |
| .tsv | tsv |
| .dj, .djot | djot |
| .dbk, .docbook | docbook |
| .docx | docx |
| .dokuwiki | dokuwiki |
| .endnote.xml, .endnotexml | endnotexml |
| .epub | epub |
| .fb2 | fb2 |
| .gfm | gfm |
| .haddock | haddock |
| .ipynb | ipynb |
| .jats, .jats.xml | jats |
| .jira | jira |
| .tex, .latex, .ltx | latex |
| .man, .1, .2, .3, .4, .5, .6, .7, .8, .9 | man |
| .mdoc | mdoc |
| .mediawiki, .wiki | mediawiki |
| .muse | muse |
| .native | native |
| .odt | odt |
| .opml | opml |
| .pod | pod |
| .pptx | pptx |
| .ris | ris |
| .rst, .rest | rst |
| .rtf | rtf |
| .t2t | t2t |
| .textile | textile |
| .tikiwiki | tikiwiki |
| .twiki | twiki |
| .typ, .typst | typst |
| .vimwiki | vimwiki |
| .xlsx | xlsx |

Each installed reader is also available through explicit selection, including
readers added by a compatible Pandoc patch. Metadata-only readers, including
bibliographies, show their parsed entries as structured frontmatter.

Binary formats use Pandoc's supported content model. They do not reproduce
word-processor pagination, execute spreadsheet formulas, or promise features
the installed reader does not understand. The XLSX fixture uses shared-string
cells; the inspected reader does not expose inline-string cell content.
Malformed, encrypted or unsupported documents fail without a published preview.
PDF input requires an actual installed Pandoc reader; no PDF reader is invented.

## Code and plain text

| Suffix or exact basename | Source-block language |
|--------------------------|-----------------------|
| .sh, .bash | bash |
| .zsh | zsh |
| .fish | plaintext |
| .pl, .pm | perl |
| .py, .pyw | python |
| .go | go |
| .java | java |
| .js, .mjs, .cjs | javascript |
| .jsx | javascriptreact |
| .ts, .tsx | typescript |
| .lua | lua |
| .c, .h | c |
| .cc, .cpp, .cxx, .hh, .hpp, .hxx | cpp |
| .cs | cs |
| .rs | rust |
| .rb, .rake, Rakefile, Gemfile | ruby |
| .php | php |
| .swift | swift |
| .kt, .kts | kotlin |
| .scala, .sc | scala |
| .r | r |
| .jl | julia |
| .dart | dart |
| .ex, .exs | elixir |
| .erl, .hrl | erlang |
| .hs | haskell |
| .clj, .cljs, .cljc, .edn | clojure |
| .lisp, .lsp | commonlisp |
| .scm, .ss | scheme |
| .ml, .mli | ocaml |
| .fs, .fsx | fsharp |
| .sql | sql |
| .ps1, .psm1, .psd1 | powershell |
| .bat, .cmd | dosbat |
| .css | css |
| .scss | scss |
| .sass | sass |
| .json | json |
| .yaml, .yml | yaml |
| .toml | toml |
| .ini, .cfg, .conf | ini |
| .xml, .xsd, .xsl, .xslt | xml |
| .graphql, .gql | graphql |
| .proto | protobuf |
| .tf, .tfvars | terraform |
| .nix | nix |
| .cmake, CMakeLists.txt | cmake |
| .mk, Makefile, makefile, GNUmakefile | makefile |
| Dockerfile, Containerfile, .dockerfile | dockerfile |
| .diff, .patch | diff |
| .vim | plaintext |
| .zig | zig |
| .nim | nim |
| .tcl | tcl |
| .f90, .f95, .f03, .f08 | fortranfree |

A private Org wrapper gives code a filename title, an application-version
SCHEMA field and one **Source code** section. Plain `.txt` uses a plaintext
block with the same metadata and no synthetic section heading. No author, date
or filesystem timestamp is invented.

Code is never executed. Highlighting uses Pandoc's installed language catalogue;
a missing language keeps the literal text and emits a notice. Delimiter-like
lines, existing commas and markup stay inside the displayed block.

Code/text must be valid UTF-8 without NUL. Display omits a leading BOM and
normalizes CRLF to LF; copying retains the original decoded text, including
the BOM and original line endings. Wrapper framing is never copied.

Ordinary JSON is indented by two spaces without changing member order,
duplicate keys or number lexemes. Invalid JSON remains visible with a notice.
Copying retains the original text in either case. `--from=json` instead reads
Pandoc's specialist JSON document AST.

## Passive authored HTML

Native HTML bypasses Pandoc conversion and the application reading template.
It keeps the authored title, structure and permitted inline, embedded and local
stylesheets. It receives no filename header, bundled fonts or outline controls.
`--from=html` selects this route explicitly; HTML extension qualifiers are
refused.

CSS is parsed into rules and tokens. Static styling is retained; imports,
font-face rules, unsupported at-rules and resource-loading functions are removed.
Supported media/supports/layer groups retain their permitted contents. CSS
resource URLs, including escapes, pass through the same raster checks as images.
Local stylesheets are inlined; their image paths retain the stylesheet's
original directory. Stylesheet chains and remote fonts are not loaded.

Scripts, event handlers, submissions, refresh, frames, executable embeds and
foreign active content are removed or made inert. A restrictive content security
policy blocks scripts and automatic network requests. Ordinary external anchors
remain explicit navigation with no referrer or opener propagation.

The first valid base URL determines relative paths and is then removed. A
remote base never authorizes local files or automatic remote loading. Supported
image `srcset` candidates are admitted individually; refused resources retain
alternative text and diagnostic notices.

## Resources and limits

Admitted PNG, JPEG, GIF, WebP and AVIF images must match their filename suffix,
declared MIME type when supplied, and bounded-header signature. Container and
data-URI images require matching MIME and signature. SVG is not a raster.
Accepted bytes are embedded in the preview; denied images keep alternative text.

Local images and stylesheets stay inside the source's configured root and
filesystem, including after symlink resolution. Every asset is limited to
10 MiB, with stricter configured source, total-source and output budgets taking
precedence. Source-relative anchors retain their original logical context.

Known ZIP document containers are checked before Pandoc reads them: at most
4096 entries and 100 MiB inflated data, with no traversal, absolute names,
duplicate normalized names, symlinks or special files. Streaming checks enforce
actual inflation and checksums. The 10 MiB media limit also applies inside
containers. Expanded bytes, wrappers, staged output, media and embedded
presentation count towards the existing session budget.

Container images are exported only from Pandoc's already-populated media bag,
through an owned bounded record in private output. Generic resource fetching
and `--extract-media` are not enabled. A missing sandbox-safe export capability
is a dependency error; conversion is never retried without sandboxing.

## Linked files and original identity

With `HTMLPREVIEW_LINKS=1`, the current file transport pre-generates eligible
linked documents of every automatically supported kind, including code, text,
binary documents and native HTML. It preserves the existing depth, file-count,
root and byte limits. Code/text contents do not create document links.

With graph mode off, links retain original-file destinations; selecting one
does not convert it on demand. Failed or skipped targets keep the established
diagnostic behaviour. The approved [W006 local service](../specs/006-local-preview-service/spec.org)
will provide on-demand conversion separately; W008 does not start a service.

Converted-page headers identify the original logical source, never a wrapper
or extraction path. Native HTML retains its own presentation and original
identity in the publication mapping. Every source stays read-only. A generated
Org wrapper does not make a code or office document eligible for annotations.

## History

- Version 1: W008 candidate format selection, literal wrappers, bounded resources
  and passive authored HTML.

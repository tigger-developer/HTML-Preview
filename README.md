# htmlpreview

Preview local Pandoc documents, source code, plain text and passive native HTML
in the system default browser, using a private temporary directory. Converted
documents use embedded Asap and Iosevka Custom fonts; native HTML retains its
authored presentation. See [supported formats](docs/FORMATS.md) for mappings,
reader selection and resource limits.

The [W008 input-format candidate](specs/008-input-formats/spec.org) extends the
earlier Org/Markdown-only scope. Its [validation record](specs/008-input-formats/validation.org)
tracks automated evidence and pending native qualification separately.

The optional [local service](docs/SERVICE.md) renders supported linked files on
request within explicitly configured roots. Start it with `htmlpreview --serve`;
normal invocations then use its loopback HTTP URLs. Absent service or an input
outside its roots selects a single-document file preview. Installation never
starts the service. [Service validation](specs/006-local-preview-service/validation.org)
distinguishes regression evidence from pending native qualification.

Service previews of genuine Org and Markdown sources also offer
[attributed annotations](docs/ANNOTATIONS.md). The composer autosaves current native
footnotes. Existing native notes remain editable after close, with their IDs
preserved and attribution labelled Created or Edited. Source changes refresh without
discarding an active draft; read-only sources use an adjacent Org sidecar.
The persistent info bar also offers literal Org/Markdown source inspection.
[Reader validation](specs/009-reader-annotation-ux/validation.org) records the
remaining paired browser checks.

**Evidence:** See [the specification](specs/001-local-document-preview/spec.org)
and its [audit record](specs/001-local-document-preview/audits.org).
Browser qualification and the Homebrew installation trial remain pending in
[the validation record](specs/001-local-document-preview/validation.org).

[The approved code and navigation change](specs/002-code-and-outline/spec.org)
adds highlighting, code copying, margin bars and configurable contents.
Its [audit record](specs/002-code-and-outline/audits.org) retains the delivery
review; browser qualification remains pending in the
[change evidence](specs/002-code-and-outline/validation.org).

```sh
htmlpreview README.md docs/notes.org
```

Each distinct source context receives a separate preview. The header identifies
the original source; activating the filename text copies its full logical path. When browser
clipboard access is unavailable, the header offers manual copying.

Code uses local syntax highlighting and the embedded Iosevka Custom font.
Click inline code, Org verbatim or a code block to copy its literal text, or
activate its copy glyph with the keyboard. Existing selections and dragging
retain normal selection behaviour. Code inside a link uses a separate copy
button so the link keeps its navigation action. Clipboard refusal offers a
readonly field for manual copying; empty code has no enabled copy action.

The [Org ledger](examples/work.org) demonstrates TODO markers, tags, drawers,
planning and source blocks. The [Markdown companion](examples/code.md) includes
highlighting, inline code, duplicate headings and long lines:

Org previews render `#+TITLE` and `#+SUBTITLE` as leading document headings.
When present, `#+AUTHOR` and `#+DATE` follow them as document metadata.
Leading Org fields share one left-aligned, initially open frontmatter panel
above the separator, using compact Iosevka text and a settings glyph. The
filename is left-aligned in the sticky info bar, with controls to the right. Frontmatter never activates path copying.
The title also supplies the browser tab title, with the filename as fallback.
Successful path and code copying briefly overlays the copied text with a fading
confirmation; refused clipboard writes retain the manual-copy fallback.

```sh
htmlpreview examples/work.org examples/code.md
```

Every preview is a complete styled HTML document. Navigation defaults to on
through source heading level three in both formats. At widths of 72rem and
above it stays in a left sidebar; narrower Org previews hide it and Markdown
previews place it immediately below the header/frontmatter. An explicit
`HTMLPREVIEW_TOC=0` disables it for every document, including linked pages.
Navigation branches have separate disclosure buttons; their heading links still
navigate. On initial load, the sidebar shows three navigation levels if they
fit its viewport height, otherwise two, then one. It scrolls if one level is
still too tall. Resizing and annotation refreshes preserve the chosen folds.
This replaces the earlier default-off setting for Org. A thick accent margin
bar opens a folded section; the thin bar closes it. Headings also toggle their
sections, and folded sections show a large disclosure triangle and a labelled
Show more button. These supersede the earlier small bottom-plus indicator.
Overview, Contents and Show all sit beside the filename in subtly coloured
buttons for both formats. Generated Org drawers remain closed when sections open.
Tags are plain muted-pink text. Frontmatter and drawers use a quieter background
than code. Drawer summaries name the drawer once; property keys have muted
labels and darker backgrounds, while values retain the normal foreground.
Opening and closing drawer delimiters are omitted, and free text has no inner box.
Without JavaScript, the full document remains open and code stays selectable.
Printing includes all content and hides the interactive controls.

## Reading and navigation

Quick previews retain their files for three seconds after the last browser
handoff. This delay does not establish browser readiness. Use a reading session
for slow browser startup, reloading, or linked navigation:

```sh
HTMLPREVIEW_MODE=read htmlpreview README.md
```

The command stays in the foreground until Ctrl+C or SIGTERM. It then removes
only its own session directory. SIGKILL or a system failure can leave that
directory behind; the reported path identifies the exact directory for manual
removal. The optional service has its own lifetime; there is no cross-session
scavenger.

For linked browsing, configure and explicitly start the optional service as
described in [the service guide](docs/SERVICE.md). HTTP-only invocations return
after browser handoff. Each followed link is converted on request; no filesystem
scan or eager graph conversion occurs. Outside-root links remain inactive with
an explanation. Unique Org heading and custom-ID searches resolve on request;
fileless IDs use only the bounded catalogue of already rendered documents.

### Historical graph mode

The earlier file transport used opt-in graph pre-generation. These examples
record that superseded interface:

```sh
HTMLPREVIEW_LINKS=1 htmlpreview docs/VISION.md
```

Traversal stayed within each entry's physical parent directory and filesystem.
An explicit root permitted a wider document neighbourhood:

```sh
HTMLPREVIEW_LINKS=1 HTMLPREVIEW_ROOT="$PWD" htmlpreview docs/VISION.md
```

The former graph followed rendered anchors breadth-first under depth/count
budgets; cycles reused source contexts and symlink aliases retained their logical
parents. W008 expanded it to all supported kinds. W006 replaces that graph with
on-demand HTTP and single-document file fallback. `HTMLPREVIEW_LINKS` and
`HTMLPREVIEW_MAX_DEPTH` retain validation and one migration notice, with no graph
or retention effect. File fallback keeps original-file link destinations.

Validated local/container rasters are embedded in file previews and served
through authorized asset routes in HTTP previews, replacing original-file image
URLs. Original source context remains the basis for relative references.

Reading leaves sources unchanged; annotation composition updates selected native
footnote definitions and new-note references in service mode. Source scripts, event handlers, executable embeds,
and automatic remote resources are removed or made passive. Org includes remain
visible without expansion. Literal source/example blocks remain literal.
This is a local preview, not a portable export or a whole-process sandbox.

## Prerequisites and installation

Binary use requires **Pandoc 3.9.0.2 through the 3.9 patch series**, including its
bundled Lua 5.4. Prefix installation does not download Pandoc. The command checks
compatibility before allocating preview output; a distribution package is not
automatically a compatible version.

| Platform | Default-browser handoff |
| --- | --- |
| macOS | `/usr/bin/open` and the default local HTML association |
| Linux desktop | `xdg-open` from xdg-utils and an active desktop session |
| WSL1 or WSL2 | Existing `wslpath`, built-in Windows `powershell.exe`, enabled interoperation, and Windows access to the distribution |

WSL opens the Windows default browser, including when WSLg is present. It needs
private Linux temporary storage; Windows-mounted temporary output is rejected.
Unrepresentable original paths become inactive references. No browser
association, font cache, file permission, shell profile, or WSL setting is
changed. Native Windows executables are outside the supported build targets.

Building requires Go **1.26.8**. Normal binary use needs neither Go nor a
standalone Lua or JavaScript runtime.

```sh
make build
```

```sh
make install
```

The default installation creates `~/.local/bin/htmlpreview` as an absolute
symlink to this checkout's `bin/htmlpreview`. Keep the checkout at a stable
location; rebuilding the binary updates the linked command. Reinstalling accepts
the matching link. A conflicting file, directory or different link is preserved
and reported; move it aside before retrying.

Put `~/.local/bin` on PATH. Check command shadowing with
`command -v htmlpreview`, particularly if you already have a personal script.
An executable symlink works without neighbouring assets: fonts, templates,
CSS, browser JavaScript, and Lua are embedded in the binary. The default link
uses the checkout's licence notices and does not register system fonts.

For a copied installation independent of the checkout, supply a non-empty,
absolute prefix explicitly:

```sh
make install PREFIX="$HOME/.local"
```

This copies the binary and notices into the prefix; repeated prefix installation
replaces its managed binary and licence files. Put that prefix's `bin` on PATH.
When switching from a linked installation, move the existing link aside first;
copy installation refuses symlink destinations.
`DESTDIR` stages either mode beneath an absolute temporary root. A staged
default symlink still points to the checkout; use explicit prefix installation
for packaging. Neither mode invokes sudo or downloads Pandoc. The former
`/usr/local` default is superseded by the user-local symlink; an earlier
installation there is not removed automatically.

## Configuration

Empty values select defaults. Unknown `HTMLPREVIEW_` settings are errors.
Every supplied setting is validated, including those inactive in the selected
mode. The service uses a strict versioned YAML configuration for permitted roots;
there is no arbitrary Pandoc-argument interface. Use `--` before a filename
beginning with `-`.

An optional `--from FORMAT` or `--from=FORMAT` selects an installed built-in
reader for the explicit input batch. It never propagates to linked documents.
`--list-input-formats` lists available readers without opening a browser.
Ordinary `.json` defaults to pretty-printed code; `--from=json` selects Pandoc's
JSON document AST. Unknown extensions need explicit selection. Native HTML
uses its separate passive policy and receives no application reading controls.

| Setting | Default | Accepted values |
| --- | --- | --- |
| `HTMLPREVIEW_TOC` | `1` | `0` or `1`; enable navigation for all documents |
| `HTMLPREVIEW_TOC_DEPTH` | `3` | Integer, 1 to 6; maximum source heading level |
| `HTMLPREVIEW_LINKS` | `0` | Deprecated; `0` or `1`, no graph effect |
| `HTMLPREVIEW_MODE` | `quick` | `quick` or `read`; file retention only |
| `HTMLPREVIEW_ROOT` | Each entry's canonical parent | Existing directory containing every explicit canonical source |
| `HTMLPREVIEW_GRACE` | `3s` | Go duration, `100ms` to `1h` |
| `HTMLPREVIEW_MAX_FILES` | `50` | Integer, 1 to 500 source contexts |
| `HTMLPREVIEW_MAX_DEPTH` | `3` | Deprecated; integer, 0 to 10, no graph effect |
| `HTMLPREVIEW_MAX_SOURCE_BYTES` | `10485760` | Integer, 1 to 10485760 |
| `HTMLPREVIEW_MAX_TOTAL_SOURCE_BYTES` | `52428800` | Integer, 1 to 52428800 |
| `HTMLPREVIEW_MAX_OUTPUT_BYTES` | `104857600` | Integer, 1 to 104857600; includes embedded fonts and staging |
| `HTMLPREVIEW_DEADLINE` | `60s` | Go duration, `100ms` to `10m` |
| `HTMLPREVIEW_CONFIG` | First existing `./config.yaml`, then `~/.config/htmlpreview/config.yaml` | Absolute version-1 YAML override; no merging |
| `HTMLPREVIEW_RUNTIME_DIR` | Platform user cache directory + `htmlpreview/runtime` | Absolute user-owned private directory |
| `HTMLPREVIEW_USER_DISPLAY_NAME` | Trimmed `USER` | Annotation label; at most 128 Unicode characters and 512 UTF-8 bytes, without controls |

With neither default config present, explicit `htmlpreview --serve` serves its
startup directory and descendants. An existing config with empty roots grants
nothing; invalid or unreadable configuration is an error. Service roots remain
fixed until restart.

HTTP requests additionally apply the service's 10 MiB source, 50 MiB output
and 60-second conversion ceilings. `HTMLPREVIEW_ROOT` restricts explicit inputs
before transport selection and can only narrow configured service roots.
The [service guide](docs/SERVICE.md) describes exact paths, permissions, limits
and restart behaviour.

Published entry URLs go to stdout; diagnostics go to stderr. Exit status is 0
for success, 1 for an operational failure, and 2 for an invalid invocation.
Successful entries may still open when another input fails. Publication is
transactional: a publication failure opens no entry. Help and version requests
have no preview side effects and bypass environment validation.

Standalone output is always enabled. `HTMLPREVIEW_STANDALONE` is unsupported
and rejected as an unknown setting. Unset/empty TOC enables navigation for both
formats. Depth controls the displayed source levels. Application defaults
and explicit settings override source TOC metadata; depth is validated even
when the TOC is disabled. A document with no eligible
headings has no empty contents navigation.

## Development and packaging

```sh
make test
```

```sh
make lint
```

```sh
make vulncheck
```

`make test` uses Go's race detector, real Pandoc conversion, subprocess tests,
and controlled desktop boundaries. It also checks staged installation and
cross-compiled archives. Its per-package timeout is twenty minutes for the full
reader and filename-mapping corpus; application deadlines remain separate.
Cross-compilation and doubles do not establish native
execution on another OS or actual browser behaviour.

Provision development tools separately: golangci-lint 1.64.8, StyLua 2.5.2,
and govulncheck 1.7.0 are the inspected tool versions for this delivery.
Lint includes Go formatting, vet, the selected Go linters, StyLua, and a Lua
check in Pandoc's own host. No Node/npm or standalone Lua runtime is used;
Native oxlint and biome check browser JavaScript and CSS; paired human checks
cover actual browser interaction and appearance. The historical browser fixture
is retired from the current workflow and is not execution evidence.
The vulnerability target neither installs tools nor updates dependencies.

```sh
make release VERSION=0.1.0
```

Release generation produces `dist/htmlpreview-VERSION-OS-ARCH.tar.gz` for
darwin/amd64, darwin/arm64, linux/amd64, and linux/arm64, plus `SHA256SUMS`
and `dist/Formula/htmlpreview.rb`. WSL uses a Linux archive. Each archive
contains a CGO-free binary and the application, font, and dependency licences.

The macOS-only Homebrew formula declares Pandoc and selects the corresponding
architecture's archive and checksum. Its default URLs point to the actual local
archives. `RELEASE_BASE_URL` can identify an existing HTTP(S) archive location;
generation verifies those remote bytes before emitting that formula. It does
not publish assets, create a tap, or install a formula automatically.

`make sync` stages the whole working tree, commits when needed, then pulls and
pushes. `COMMIT_MESSAGE` defaults to `chore: sync`. Invoke it only when you intend
to include all current changes.

## Design and licensing

See [VISION.md](docs/VISION.md), [ARCHITECTURE.md](docs/ARCHITECTURE.md), and
[the owned Org prototype assessment](docs/ORG-FIDELITY-REVIEW.md).

Project code and documentation use [Apache 2.0](LICENSE). Asap and Iosevka
Custom retain their SIL OFL 1.1 licences. Dependency licences and font provenance
are listed in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).

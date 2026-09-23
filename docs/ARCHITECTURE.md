---
title: Architecture
version: 9
last-updated: 2026-09-24
---

# Architecture

The application has two conversion backends and two publication transports.
Org, Markdown and internal code/plaintext wrappers use bundled Go converters;
other readers use optional Pandoc. Passive authored HTML has its own sanitization
route. A shared Go pipeline applies resource policy, reference rewriting and
the reading interface before publishing a file preview or an authorized HTTP page.

Service annotations use current native footnotes with guarded create, edit and
confirmed-delete operations. Source-bound block probes establish eligible
insertion boundaries. Directory notifications drive server-sent events; the
browser defers display updates during typing while saves continue independently.
The [annotation guide](ANNOTATIONS.md) defines the current interaction and limits.

The converted-page info bar includes a theme toggle before the filename.
System colour-scheme detection remains the default; an explicit choice lives
only on the current document root and survives reader-region refreshes.
The existing reader lifecycle owns the button and media-query listeners. CSS
applies the existing light/dark palettes and retains light print output.
No preference is stored or sent to the service. See
[W015 theme controls](../specs/015-theme-toggle/spec.org).

**Validation:** Native Org reader and annotation reviews passed on
22 September 2026. See [migration validation](../specs/011-conversion-performance/validation.org).
The native Markdown migration was accepted and merged on 20 September 2026;
its [validation record](../specs/014-native-markdown/validation.org) includes
the reading and annotation user tests. Native Linux/WSL execution remains
separate from Go/HTTP and cross-build verification.

**Historical sections:** The dated service, annotation and input-format proposals
retain earlier design decisions. Explicitly superseded graph, append-history,
polling and browser-runner descriptions are not current implementation contracts.
Current operational interfaces are in [SERVICE.md](SERVICE.md),
[FORMATS.md](FORMATS.md) and [ANNOTATIONS.md](ANNOTATIONS.md).

## Native Markdown conversion - 20 September 2026

[W014 - Native Markdown previews](../specs/014-native-markdown/spec.org) adds pinned
Goldmark 1.8.6 without a fork. Its adapter normalizes heading sections and IDs,
contents, highlighted code records and footnote/backlink markup to the established
browser contract. Existing block-probe, projection and source-write boundaries
remain authoritative. The browser does not need a second Markdown implementation.

`internal/convertworker` shares the disposable-worker protocol, output bounds and
memory supervision between Org and Markdown. `internal/converthtml` shares Chroma
highlighting. Each conversion still owns a separate process; there is no worker
pool, pass reduction or new source-offset mapping. Those are future opportunities.

Markdown raw-HTML AST nodes are omitted before rendering. A private transport
marker carries one omission flag into the page record; it is removed before
publication. Optional Markdown readers apply equivalent omission in the owned
Lua filter before generated transport is added. The banner uses the existing
replaceable frontmatter region, outside canonical annotation text. HTTP response
metadata lets an explicit CLI invocation emit one notice per affected document.

Native requests never query Pandoc. Optional-reader discovery is lazy, bounded
and cached on success for the service lifetime; failed discovery remains retryable.
Native conversion failure never falls back to Pandoc. File-mode, service, link,
annotation and packaging evidence is retained in
[W014 validation](../specs/014-native-markdown/validation.org).

## Native Org conversion - 20 September 2026

[W011 - Fast Org previews](../specs/011-conversion-performance/spec.org) replaces
Pandoc for Org and the internal Org wrappers for code/plaintext. That migration
left Markdown on Pandoc; W014 subsequently replaced it with Goldmark. Other
readers retain optional Pandoc. The existing preservation, annotation projection,
block-proof passes, passive-content policy and publication pipeline remain.
Earlier descriptions below of Pandoc owning Org output describe the superseded
converter; their source, browser and security contracts continue to apply.

`internal/orgconvert` runs through a private mode of the same executable,
resolved with `os.Executable`. Each conversion receives one bounded JSON request
on stdin, never a source filename or resource path. The existing process executor
owns cancellation, reaping and combined output bounds. The worker has a 512 MiB
Go runtime memory target and checks accounted runtime memory every 10 ms. This is
not a hard RSS ceiling. Intermediary/protocol bytes count against the existing
session allowance, and worker output is limited to 50 MiB or the smaller remaining
allowance. Conversion errors publish no partial page and do not fall back to
Pandoc for Org.

The pinned go-org parser has narrow local corrections for long admitted lines,
Org's default numeric-list grammar and mixed descriptive/ordinary list boundaries.
Its writer hooks produce the established heading/code/contents transport
and footnote HTML contract. Chroma supplies escaped literal spans using the
existing highlight classes. The application then applies the same sanitization,
resource authorization, heading catalogue and browser controllers. Parser file
reads are denied; includes cannot fetch local or remote resources.

The adapter handles repeated reference/backlink associations, declared task
partitions, checkbox variants and same-document heading searches without
weakening block proof. Only current generated block-reference labels receive an
invisible conversion-only predecessor; authored footnote definitions do not.
Generated preservation fragments avoid introducing an extra blank line that
would prematurely terminate a native footnote definition. Source bytes remain
under the existing annotation writer's ownership.

The distribution retains parser, highlighter and transitive licences. The
vulnerability gate checks the linked application and a locked unreplaced
upstream module, preserving advisory coverage despite the local replacement.
See [parser provenance](../third_party/go-org/README.md) and
[validation](../specs/011-conversion-performance/validation.org) for the patches,
execution evidence and accepted human reader and annotation reviews.

The accepted compatibility layer adds some formatting work. Future optimization
may reduce converter passes, use native source offsets or avoid redundant HTML
traversals once equivalent proof is demonstrated. Worker reuse would require a
new isolation/lifetime decision. W014 supplies the separate Markdown replacement
and benchmark; further pass reduction remains deferred. These opportunities are recorded in W011's solution
design and are outside this implementation.

## Local service design history - 11 September 2026

[W006 - Local preview service and automatic fallback](../specs/006-local-preview-service/spec.org)
defines a second delivery transport around the existing renderer. A per-user Go
HTTP service renders requested documents within configured roots. A private
Unix-domain control socket registers previews; browser URLs use a random token
followed by a root-relative path. Binding is hardcoded to IPv4 loopback, with
HTTP and no remote-address setting. Documents and permitted assets undergo
authorization and rooted file-handle checks on every request.

The command falls back to single-document file delivery when the service is
unavailable or the explicit source is outside the configured roots. Rendering
failures remain errors. The candidate specifies restart-scoped capabilities,
bounded caching/concurrency, Org destination handling, optional user-service
packaging and native platform qualification. It deliberately replaces graph
pre-generation while retaining file quick/read lifetimes and the current
presentation contracts.

Taḋg approved this design on 11 September 2026 and explicitly deferred
implementation. Its exact contracts supersede the no-service, pre-generation
and file-only resource-policy portions below as design authority; the earlier
sections remain baseline history. No service implementation or native
qualification is claimed by this document update.

The service implementation now exists as a delivery candidate under
`internal/preview/service*.go`, sharing the W008 renderer and format catalogue.
Its [validation record](../specs/006-local-preview-service/validation.org) retains
the staged implementation evidence and pending native qualification. File mode
now converts explicit inputs only; legacy graph settings are validated and
diagnosed without pre-generation. The earlier processing and graph descriptions
below are retained as design history, superseded by W006's two transports.
The [service guide](SERVICE.md) supplies the current configuration and lifecycle
interface.

The operator's 13 September configuration amendment replaces the OS-specific
config default: explicit `HTMLPREVIEW_CONFIG`, then `./config.yaml`, then
`~/.config/htmlpreview/config.yaml`, with first-found selection. If neither
default file exists, explicit foreground service startup grants its canonical
working directory. Existing empty or invalid configuration never enables that
fallback. The startup grant stays fixed; later CLI working directories cannot
expand it. Packaged managers retain an explicit user config path, now under
`~/.config` on every platform. W006 owns this amendment and its delivery evidence.

## Annotation design history - 12 September 2026

[W007 - Attributed autosaved annotations in service previews](../specs/007-service-annotations/spec.org)
depends on delivered and qualified W006 service behaviour. It proposes a separate
Go annotation boundary for constrained append-only writes, versioned embedded
records, read-only sidecars and deterministic passage anchors. Browser composers
autosave revisions and preserve unsaved text across automatic source refresh;
every append rechecks the current file revision. Closed comments are read-only.

Private registration captures the CLI display name. Purpose-specific document
write capabilities, exact-origin requests and rooted file handles constrain
mutation. W006 read-only registration remains compatible; new annotation routes
narrowly extend its GET/HEAD-only HTTP contract. File previews do not annotate.
Append history and refresh do not prevent an unrelated editor from later saving
an old buffer over newer file contents; the specification states that limit.

The proposed polling refresh supersedes the earlier live-reload exclusion for
service previews. Its Go runner with browser-native JavaScript assertions also
supersedes the unselected browser-harness baseline below and W006's persistent
browser-harness exclusion. This narrowly scoped test runner supports the required
autosave and refresh regressions without Node/npm. Headless CI retains Go tests;
actual browser qualification is required on the supported native matrix before
the implementation gate, with unavailable browsers reported as unexecuted failures.

The earlier definition-only statement is superseded by the W007 implementation
candidate. `internal/annotation` owns framing, history projection, canonical text
and rooted append operations. The existing preview service owns author-bound
grants and a bounded shared polling snapshot; every append bypasses that snapshot.
Browser composer, text mapping and presentation modules are packaged with the
existing reader script. Refresh reuses its teardown/initialization boundary while
retaining the composer outside replaced document regions. No dependency was added.
The [annotation guide](ANNOTATIONS.md) records storage and operational limits;
[validation](../specs/007-service-annotations/validation.org) distinguishes local
regressions from pending native qualification. The operator authorized proceeding
on 13 September while Linux/WSL qualification remains pending.

The source-preservation rule gains an explicit exception only for
annotation records. A future Exodan deployment would require its own definition
over server-owned documents and storage, never a client's personal filesystem.

## Input format design history - 12 September 2026

[W008 - Input formats](../specs/008-input-formats/spec.org) introduces a shared
format resolver ahead of the existing conversion and publication boundaries.
It discovers installed Pandoc readers, admits binary snapshots, creates literal
Org wrappers for code/text, formats ordinary JSON without changing copy content,
and handles bounded container-owned media. Original source identity remains
separate from any generated Org intermediate.

Native HTML bypasses Pandoc and the application reading template. A distinct
passive policy preserves authored structure and sanitized local/inline CSS while
blocking source scripts, form submissions and automatic external requests.
This supersedes the blanket source-style prohibition only for native HTML;
ordinary converted pages retain their current presentation and CSP.

The amended [W006 service](../specs/006-local-preview-service/spec.org) consumes
this format contract for explicit inputs and linked targets. Reader selectors
are validated representation choices, never filesystem grants. Scoped embedded
media and inlined stylesheet dependencies participate in authorization, cache
revalidation and resource budgets. Link rewriting happens before publication,
including native HTML, so document navigation needs no browser JavaScript.

W008 file-mode delivery precedes W006 format integration without a dependency
cycle. W007 annotation storage remains limited to genuine Org/Markdown sources.
Until W006 replaces it, the existing opt-in file graph admits all W008-supported
formats under unchanged traversal budgets. Node identity includes the selected
format; graph-disabled links retain original-file destinations.
Taḋg approved these amendments on 12 September 2026 and authorized delivery in
the order W008, W006, W007. These approvals are not implementation claims. The
11 September W006 approval and hold remain history; the delivery instruction
explicitly released that hold. Each work item's admission and qualification
requirements remain applicable.

## Reader and footnote design history - 14 September 2026

[W009 - Persistent reader controls and native footnote annotations](../specs/009-reader-annotation-ux/spec.org)
implements one sticky info bar and a shared responsive grid in the existing page
template. It replaces the fixed navigation width and floating annotation panel;
phone annotation mode reserves the larger lower pane for comments while keeping
document text selectable above. No iframe or additional browsing context is used.

The existing reader lifecycle owns mode controls. The annotation panel captures
a collapsed insertion point and delegates to canonical text mapping and composer
scheduling. The comment box gets initial focus, with a secondary editable footnote
label below. One guarded close path handles point changes,
leaving annotation mode and entering plaintext without hiding unacknowledged text.
Native Org timestamps provide the displayed and saved attribution format.
The annotation package now owns a
native-footnote codec and current-record replacement, superseding the
earlier event-log writer and selected-text anchors. Each current record retains
readable attribution without hidden metadata or history. Latest retry identity
and composer state remain only in bounded service memory. Closed comments and
ordinary named notes are editable in annotation mode.

The source reference uses normal Org/Markdown notation and the editable label;
a separate stable annotation identity supports renaming and retry handling.
New insertion points require one bounded marker round-trip through the existing
renderer. Unprovable points report an error rather than inserting elsewhere.
New-note saves patch only their reference tokens and definitions; existing-note
edits patch the selected definition, then atomically replace
the freshly checked rooted file. Metadata that cannot be preserved blocks saving.
The v2 annotation adapter retains existing grants/origin checks and retires v1
mutation. The existing decoder remains for legacy reading and import on save.
Sidecars keep current Org footnotes and point context; they cannot physically
insert a reference into their read-only source.

For genuine Org/Markdown, Go retains the original admitted snapshot before
preprocessing and embeds it as bounded inert data. Plaintext uses textContent,
without highlighting, and includes embedded annotation records. Existing page
GET/refresh supplies current text in service mode; file previews use their
snapshot. No raw-source endpoint, binary extraction, wider capability or dependency is
introduced. Payload growth counts against existing output/cache budgets.

The selected converter supplies the shared footnote HTML contract: native reference anchors, one endnotes
section and its backlinks. Annotation mode moves that existing section into the
aside and returns it to the document end on exit. It does not clone or regenerate
the notes. Refresh and print restore placement through the reader lifecycle;
canonical insertion-point text excludes endnotes and reference labels in both
Go and browser mappings. The separate annotation list/print appendix is retired.

This is a paired implementation candidate. Taḋg withdrew permanent autosave history
and selected footnotes and point insertion on 14 September. Its specification
identifies the superseded W005/W007 presentation, storage and mutation requirements;
the earlier paragraphs remain historical baseline. Root confinement and
passive-content contracts remain. The legacy filesystem writer and v1 HTTP
mutation path are retired. The legacy decoder reads old stores until an authorized
save imports the selected destination's latest values.

For sidecars and unplaced or legacy notes, a temporary projection combines native
footnotes with the original document for the selected converter. Private markers verify virtual
positions against the original canonical text; unverified positions become
explicitly unplaced endnotes without false backlinks. The projection never
replaces the source payload or authored revision. This uses the existing conversion
and output budgets, with no separate annotation HTML renderer.

Current verification uses Go/HTTP regressions and the standard native HTML/JS/CSS
checks. On 14 September the operator stopped browser-fixture execution in favour
of paired human interaction checks. That instruction supersedes the W007 browser
runner prescription above. [W009 validation](../specs/009-reader-annotation-ux/validation.org)
records objective evidence separately from pending browser/platform qualification.

### Editable-footnote amendment - 14 September 2026

The operator approved editing supported native footnotes, including ordinary
notes and closed annotations. The parser reconstructs named definitions and
byte spans from the current source or sidecar. An explicit edit identifies the
label, store and definition digest through the existing authenticated v2 route.
The shared atomic writer replaces only that definition, refreshes a matching
Author line to the current reviewer and Edited timestamp, and checks the file
revision. Ordinary notes gain no ownership metadata. Equal
current content makes a repeated save harmless; conflicting text retains the
browser draft. An unrelated source change can refresh and retry while the
selected definition digest remains unchanged.

Conversion-only markers associate the converter's endnotes with native labels, including
repeated references. Markers are removed before publication; application-owned
attributes retain the mapping. The sidebar uses the existing composer and
source refresh. An edit fixes the existing ID and finishes after its latest
save acknowledgement, without a separate persisted close event. Earlier
closed-comment and ordinary-note editing restrictions are superseded. Native
named definitions support continuation paragraphs; ambiguous or unresolvable
forms remain readable without unsafe guessed edits. Existing read-only source
definitions remain unchanged; existing sidecar notes are edited in place.

## System shape


The implementation uses `cmd/htmlpreview`, `internal/preview`, a root Go asset
bundle, and `internal/buildtool` for build/package tasks. Editable presentation
sources live under `assets/web`; owned Pandoc defaults and Lua live under
`assets/pandoc`. The prototype remains an assessment reference and is not a
runtime dependency.

The reviewed delivery toolchain is Go 1.26.8. This uses the specification's
permission to select a compatible patch after the initial 1.26.3 baseline.
The first vulnerability scan reported standard-library findings involving
`os.Root`, `html/template`, and network packages; the scan with 1.26.8 reported
no vulnerabilities. The [Go release history](https://go.dev/doc/devel/release)
records the relevant security and correctness updates. This selection changes
the build baseline, not the runtime dependency contract.

The complete dependency inventory is in `go.mod` and
[the notices](../THIRD_PARTY_NOTICES.md). Original direct dependencies include
`golang.org/x/net` 0.58.0 for HTML5 parsing and
`github.com/microcosm-cc/bluemonday` 1.0.27 for allowlist sanitization. The
standard library has no equivalent parser/sanitizer. Their module checksums are
tracked; the two transitive CSS-parser dependencies and their licences are
recorded in [the notices](../THIRD_PARTY_NOTICES.md). No source CSS is enabled
merely because the sanitizer contains a CSS parser.

W006 adds `go.yaml.in/yaml/v3` 3.0.5 for strict user-authored service YAML.
The pinned package adds no runtime executable or transitive module dependency;
its mixed MIT/Apache licence is retained in the distribution notices. The service
owns the startup root snapshot, private control socket, read capabilities,
in-flight conversions and bounded memory cache. Every conversion owns and cleans
its temporary directory before publication; cleanup failure returns an error and
reports the remaining owned path. Token-free availability rechecks permit file
fallback only after confirmed endpoint loss, before any batch handoff.

W008's native HTML implementation selects `github.com/tdewolff/parse/v2`
2.8.16 for its CSS grammar and token parser. The standard library has no CSS
parser; the existing transitive sanitizer parsers do not supply this boundary's
modern grammar handling. The selected release was published on 11 August 2026
and uses the MIT licence, retained with the distribution. Its runtime packages
use the standard library; its declared test helper is not linked into the
application. It adds no runtime executable or service. Dependency scanning and
binary-size comparison remain required candidate verification.

A 12 September macOS arm64 comparison with Go 1.26.8 and `-trimpath` measured
8,506,802 bytes at the input-selection checkpoint and 8,969,378 bytes after the
container, raster and native HTML implementation. The combined increase was
462,576 bytes (5.4%); this includes the surrounding implementation and is not
an isolated measurement of the CSS dependency. Candidate verification is
recorded in [the input-format validation record](../specs/008-input-formats/validation.org).

`htmlpreview` is a local Go command-line application that uses go-org for Org
and Goldmark for Markdown, with optional Pandoc for other readers, repairs references, and opens
the result in a browser. [VISION.md](VISION.md) defines the product intent.

File previews own a private temporary directory per invocation. Service renders
own temporary workspaces and publish through a bounded in-memory page cache.
Reading leaves original sources unchanged; service annotation writes use the
separate guarded native-footnote writer.

The processing sequence is:

1. Validate input files and invocation settings; discover Pandoc only for optional readers.
2. Select service transport where available and authorized, otherwise file delivery.
3. Allocate each conversion workspace, register cleanup and render the explicit inputs.
4. Rewrite references for that transport; HTTP links render targets on request, while file links retain original-file destinations.
5. Publish prepared pages through the selected transport and open the explicit inputs.
6. Clean up HTTP conversion workspaces before publication; retain file output for the selected reading mode before cleanup.

## Historical foundations

The personal preview command provides the basic interaction: one or more input
files, a styled Pandoc conversion, source-path metadata, browser opening, and a
three-second delay before cleanup. It writes random HTML filenames beside each
source, rather than necessarily in the shell's current directory. Its personal
shell helpers and Pandoc data paths are not a public installation contract.

The repository prototype supplies a different, complementary foundation:

| Source | Intended contribution |
| --- | --- |
| [org2html](../prototype/org2html) | Conversion wiring and Pandoc options |
| [org-fidelity.css](../prototype/org-fidelity.css) | Typography, document elements, Org details, themes, and print styling |
| [org-fidelity.lua](../prototype/org-fidelity.lua) | Org-specific structural and inline transformations |
| [org-prepass.awk](../prototype/org-prepass.awk) | Preservation of planning lines and logbook drawers before parsing |
| [org-fold.html](../prototype/org-fold.html) | Browser outline controls |

The Org fidelity prototype is project-owned code, including for design, build,
licensing, and verification. Reuse does not exempt it from refactoring or the
project's engineering standards. It must become part of the normal build and
test boundaries rather than an opaque external tool.

These sources are not an established compatibility suite.
The prototype's [README](../prototype/README.md) records reader limitations,
including startup visibility and mixed definition/checkbox lists. Its features
need verification against the supported Pandoc release.

The [initial code assessment](ORG-FIDELITY-REVIEW.md) identifies source-context
handling, identifier loss, keyboard interception, presentation state, and print
coverage as areas requiring work. It recommends retaining the split between
source preservation, Pandoc transformations, and presentation while replacing
the shell pipeline and ad hoc text transformations.

## Technology and ownership

Go is the selected application language. Its additional build and release
toolchain is accepted in exchange for structured path handling, graph traversal,
process control, cleanup ownership, and tests. Lua remains useful inside Pandoc;
it does not need a separate installed interpreter. Pandoc provides that runtime.
[Pandoc Lua documentation](https://pandoc.org/lua-filters.html).

| Component | Responsibility |
| --- | --- |
| Go entry point | Inputs, settings, dependency checks, diagnostics, and exit status |
| Session manager | Private workspace, output budget, retention, cancellation, and cleanup |
| Renderer | Source preservation, native Org/Markdown worker or optional Pandoc invocation, assets, and conversion errors |
| Reference resolver | Source-relative URLs, filesystem identity, authorized HTTP targets and file mappings |
| HTML processor | Structured discovery and rewriting of rendered links and resource references |
| Browser adapter | Open the finished entry pages using the platform's desktop mechanism |
| HTML/CSS and browser JavaScript | Presentation, accessible folding, and source-path/code copying |

These are responsibilities, not a requirement for one package per row. A small
`cmd/htmlpreview` entry point, private implementation under `internal/`, and
versioned presentation assets are sufficient starting boundaries.

There is no JavaScript command-line runtime. Bash is limited to optional build
and packaging glue. The prototype's pre-processing should move into Go, avoiding
an additional text-processing runtime dependency.

## Rendering and presentation

Package the template, defaults, stylesheet, Lua transformations, browser script,
and help text with the executable. Go's [embed package](https://pkg.go.dev/embed)
supports packaging these files. Extract files required by Pandoc into the
session, and include application-owned CSS and JavaScript in each output page.
Keep their editable sources as separate repository files.

Use explicit packaged defaults and asset paths. Normal rendering must not depend
on personal Pandoc defaults or machine-specific fonts. Provide system font
fallbacks and a consistent base presentation for both input formats.

Asap is the selected default for body text, headings, and interface text. The
regular and italic variable WOFF2 files are included under
[assets/fonts/asap](../assets/fonts/asap/README.md). Inspection of both files
confirmed version 3.002, weight 100 to 900, and width 75% to 125%, with defaults
400 and 100%. Use matching normal and italic `@font-face` declarations with
those ranges, and sensible `font-weight` and `font-stretch` values. Verify Irish
characters and dotted letters in the presentation checks.

Iosevka Custom is the selected default for code blocks, inline code,
preformatted text, and all other fixed-width styling. The
[four included WOFF2 faces](../assets/fonts/iosevka-custom/README.md) are static
version 34.1.0 fonts at normal width: regular/italic at weight 400 and
bold/bold italic at weight 700. Declare each face separately and route
fixed-width styling through the same family, with a generic monospace fallback.
Use the packaged data URLs without a `local()` source, so installed fonts
cannot replace the selected assets. This choice, made on 8 September 2026,
specifies the earlier generic monospace requirement.

Embed the font bytes in the executable and include them as WOFF2 data URLs in
generated CSS. This avoids dependence on locally installed fonts, remote font
services, or font-file URLs outside the generated page. Account for both Asap
faces and all four Iosevka Custom faces, including base64 expansion, in the
output budget. Iosevka adds 1,923,456 bytes of base64 payload per page before
CSS and notices; the existing byte budget may therefore stop traversal before
the document-count limit. Include both families' copyright notices and full
OFL text in readable HTML source and release packages. The initial architecture
specified this before implementation; the renderer now supplies the embedded
payloads, with browser qualification tracked separately.

Embedding resolves repository assets at build time. At runtime the renderer
reads those embedded bytes and emits inline `@font-face` data URLs. Neither
the executable's location nor the invocation directory participates in font
lookup. An executable symlink therefore needs no adjacent font directory.
`make install` installs the binary and notices, without modifying system font
directories or caches. The prototype's sibling-file wrapper and system font
stacks do not yet implement this contract.

For Org, preserve planning information and drawers before native parsing, then
restore their owned presentation fragments. Pre-processing must recognize literal
source and example blocks so that text inside them is not transformed. Keep
section wrappers for outline folding. Preserve source content and identifiers;
do not recompute task statistics or execute source blocks.

Org metadata remains document content in the final preview: `TITLE` and
`SUBTITLE` become leading headings, followed by `AUTHOR` and `DATE` when present.
Go retains the leading Org keyword fields in source order, including repeated
and unknown fields. Literal blocks and drawers are excluded. The final template
escapes their values into one native details panel above the separator; the
panel begins open, left-aligned, in small Iosevka text. Leading STARTUP and TODO
values no longer create independent boxes in the body. Their parser semantics
remain active, and include/setup directives remain inert with diagnostics.
Title, subtitle, author and date form a separate title block below the separator.
W005 places navigation before that block in reading order and in a separate
column on wide screens. The native Org worker leaves that title block to the
final template. Native Markdown metadata uses bounded YAML parsing; optional
readers retain the Lua metadata filter.

The browser script should remain a small enhancement. Ordinary reading and
navigation must work without it. Folding must preserve keyboard navigation,
visible focus, and fragment destinations, including revealing folded ancestors
when a link targets their contents.

The Org pre-pass marks only its own restored drawer fragments. The publication
sanitizer permits that marker on generated details elements, while source HTML
cannot retain it. During enhancement, marked drawers close after outline state
is applied. Unfolding a section leaves them closed; native summary, fragment
revelation, runtime Show all, and print retain their defined roles.

W005 replaces raw drawer delimiters with presentation structure. Go collects
only the inner content and groups consecutive property lines and free text.
The embedded drawer template escapes each key and value, renders property runs
as definition lists, and leaves other text preformatted without an inner border.
The drawer name appears once in a gear-labelled summary. Empty and repeated
properties remain visible; missing terminators retain consume-to-EOF behaviour.
The template is parsed once per Org conversion and errors propagate normally.
Frontmatter and drawers share subtle metadata colours distinct from code panels;
keys have darker backgrounds, values use the normal foreground, and tags use
plain muted pink without the TODO badge's border or background.

The original filename header is required, including copying its full source
path. The sticky info bar places the directory and emphasized filename on the
left, controls on the right, and a separator above the document body.
Use Iosevka Custom at 400 for the directory and 700 for the filename, with
accessible contrast and wrapping for long paths. The browser tab title uses
the Org document title when supplied, falling back to the original filename.
The 10 September 2026 emergency layout amendment supersedes the earlier
filename-only tab title. The source path remains the dedicated copying target.
On a titled Org page the document title is the sole semantic H1 and the filename
is a non-heading source label. Without an Org title, retain the filename H1.

Display the logical source path with the home directory abbreviated as `~`;
copy the full absolute logical path, including the Linux path from WSL.
Do not copy the temporary HTML
name, a Windows handoff URL, or a shell-quoted string. Preserve spaces and
special characters by keeping the copy value separate from formatted text;
the personal template's whitespace trimming is not a path-serialization rule.

Enhance the header with a native button supporting click, Enter, and Space.
Call `navigator.clipboard.writeText` directly from that user activation, with
no clipboard reads or automatic copying. Show and announce success only after
the write succeeds. If the API is unavailable or denied, keep the complete path
selectable and show a manual-copy instruction. Without JavaScript, retain a
plain selectable header; printing retains source identity without controls.
Clipboard access depends on browser policy and must be checked in the actual
file previews across the qualification matrix.
[Clipboard API working draft](https://www.w3.org/TR/clipboard-apis/#dom-clipboard-writetext).

Application styling is included locally. Authorized raster bytes are embedded in
file previews and served through registered asset routes in HTTP previews.
Pandoc's `--embed-resources` also incorporates document resources and can fetch
remote assets, so it is not the default mechanism for packaging only the theme.
[Pandoc resource options](https://pandoc.org/MANUAL.html#option--embed-resources).

### Code, contents and browser controls

[W002 - Code and document navigation](../specs/002-code-and-outline/spec.org)
extends the approved renderer. The Pandoc route runs with standalone output and
the owned `page.html5` template. This intermediate wrapper includes only the
generated body and optional contents; native Org and Markdown supply the same contract.
The final Go template adds the original
source header, fonts, policy and browser script once.

`HTMLPREVIEW_TOC` defaults to `1` for both Markdown and Org, accepting an
explicit `0` or `1` for all documents in the invocation, including linked pages.
`HTMLPREVIEW_TOC_DEPTH` defaults to `3`, accepting source levels `1` through `6`.
Empty values select defaults. Each page carries an owned format attribute
derived from the selected input format. The shared configuration
is never changed by rendering an Org file, so mixed inputs and linked targets
remain independent. Validation precedes session allocation, including
depth when contents are disabled. Source TOC metadata does not override these
application choices. No standalone setting is exposed.
This follows [W005 - Responsive navigation and quieter Org metadata](../specs/005-navigation-and-metadata/spec.org).
It supersedes W003's Org default-off choice; W003 had previously replaced W002's
default-on choice. Existing explicit overrides retain their meaning.

The final template emits one navigation landmark immediately after the source
header/frontmatter and before the title/body. With eligible navigation, CSS Grid
uses a content-sized left column at widths of 72rem and above. Sticky positioning keeps
it visible; a viewport height limit and independent overflow keep long lists
reachable. Below the breakpoint it is hidden for Org and flows above Markdown
content. Disabled or empty navigation reserves no column. Print returns it to
normal flow without a height limit. The existing fragment handler reveals folded
targets. Navigation disclosure buttons fold branches; initial fitting tries three
visible levels, then two, then one, within the configured maximum depth.

The Org pre-pass preserves source-block bodies in string-only JSON records in
an unpredictable per-document raw format. Lua validates the records and makes
real Pandoc code blocks on the retained route. The native Org worker consumes
the same preservation records. Code blocks receive owned associations and
literal-value records. Chroma supplies native Org/Markdown highlighting; Pandoc supplies
highlighting for its readers. Both emit compatible semantic spans;
the owned stylesheet supplies the light/dark palette. Shell aliases `sh` and
`shell` use Bash. Unknown languages and examples remain plain.

Go validates and removes only its transport records before source sanitization.
It retains writer text when exact, restores a single omitted final newline,
or falls back to the exact plain value with a bounded diagnostic. Source and
output limits cover conversion input, transport output and final pages.
No language label selects an external syntax definition or executes code.

Temporary heading identities bind the selected converter's contents to concrete heading nodes.
Go restores source identifiers, applies Org properties and allocates unique
final IDs before retargeting those links. Contents are separated before document
link processing and receive their own passive sanitization. Heading images use
alternative text in contents labels; document images keep normal resolution.
Empty contents are omitted. The filename header never enters the document outline.

The browser module owns one clipboard service for the source header and code.
It captures literal text before inserting native copy buttons and permits one
pending write per page. Code within links keeps navigation; its copy button is
outside the link. Direct copying respects active selections, dragging and
modifier clicks. Success follows the resolved write; failure exposes a labelled
readonly field without reading the clipboard or moving focus.
Only the element with the owned source-path attribute becomes the filename
button; the surrounding row and frontmatter are never wrapped in it. Successful
writes place a temporary CSS overlay on that source button or the copied code
surface, with a separate live-region announcement. Literal text stays intact.
New copying, printing, and teardown clear the prior overlay. A two-second
confirmation fades, except when reduced motion is requested. Refusal shows the
existing labelled manual field and no success overlay.

Each enhanced section reserves a separate gutter for its native folding button.
The hit target fills its visible height and is at least 24 CSS pixels wide.
A hidden body uses a four-pixel accent bar, large right-facing triangle and
labelled Show more button; a visible body uses a one-pixel bar. These replace
the small bottom plus after the operator's 10 September correction. The native
bar and Show more buttons share the toggle action. Heading text has no folding
listener; eligible text instead participates in annotation mode. Buttons have
the appropriate pointer cursor. There is one
bar per section and no additional decorative nested border.
Compact blue, green and amber buttons for document outline actions share the
filename row in both formats. They have filled backgrounds and no outline or
gap, as requested by the operator. Frontmatter spans its own row beneath them.
Global modes, initial Org visibility, fragment reveal and print restoration
retain their existing roles. Control names capture heading text before code
buttons are inserted. Lifecycle teardown removes listeners, controls and timers;
page restoration enhances the passive document once.

## Temporary files and reference resolution

The reference table below records the original file-graph design. Current file
previews retain original document destinations and embed admitted rasters;
HTTP previews use authorized document/asset routes. Native HTML may render as
a document; PDF/SVG download policy is described in [SERVICE.md](SERVICE.md).

Allocate a private session directory with the operating system API, using
`os.MkdirTemp` with its default temporary location. This API creates a directory
with owner-only permissions before the process umask is applied.
[Go temporary-directory documentation](https://pkg.go.dev/os#MkdirTemp).

Keep generated names independent of source filenames, for example numbered
pages inside the randomly named session. Maintain a mapping from each resolved
source identity to its generated page; two files named `README.md` must remain
distinct. Write final pages before opening any browser window.

Resolve every supported relative reference against its containing source's
directory, never against the process working directory or the temporary output
directory. Separate URL paths, queries, and fragments before resolving a local
path. Encode the resulting file URL correctly for spaces, Unicode, percent
signs, and literal filename delimiters. Use Go's
[URL types](https://pkg.go.dev/net/url) alongside filesystem path operations.

| Original reference | Preview destination |
| --- | --- |
| `guide.md#install`, when converted | The generated guide page with `#install` |
| `../notes.org`, when converted | The generated notes page |
| A local document not converted | An absolute file URL to the original document |
| `images/chart.png` | An absolute file URL to the original image |
| `appendix.pdf` or an existing HTML file | An absolute file URL to the original file |
| `#heading` or a footnote fragment | The same generated page and fragment |
| An external URL | The original external URL |

Discover links and rewrite output with an HTML parser. This includes ordinary
links emitted by the converters and anchors retained by the passive native-HTML
route. Authored raw HTML in Markdown is omitted before this stage.
The dependency is `golang.org/x/net/html`, the Go project's HTML5
parser. It handles HTML structure; the standard library's escaping helpers and
XML parser are insufficient substitutes. Its maintained release series is a
reason to prefer it to a project-written parser; `go.mod` pins the version.
[HTML parser documentation](https://pkg.go.dev/golang.org/x/net/html).

Reference coverage must include supported resource attributes as well as anchor
destinations. `srcset`, inline CSS `url()` references, and SVG fragments need
syntax-specific handling; an HTML parser alone does not resolve these. Define
and test that raw HTML/CSS subset before claiming all relative references work.
Unsupported forms must be reported. Dynamically constructed JavaScript URLs
are outside the initial document-preview contract.

A source-directory `<base>` element is an alternative, but is not the proposed
default: it changes the meaning of same-page links and would need corresponding
fragment repairs. Explicit rewriting keeps generated-page navigation local.
[HTML base-element semantics](https://html.spec.whatwg.org/multipage/semantics.html#the-base-element).
Source-supplied base elements also require an explicit policy before support.

This output remains a local preview, not a portable export. Absolute resource
references still depend on the original files being present and accessible.

## Historical linked-document pre-generation

This section records the superseded file-graph design. The service now renders
linked documents on demand; no eager graph is generated. Configured roots and
request budgets are documented in [SERVICE.md](SERVICE.md).

The proposed extension is opt-in. Treat the explicit inputs as graph roots,
follow supported local Markdown and Org hyperlinks in source order, and render
each admitted document once. A breadth-first queue gives nearby documents
priority when the limit is reached. Do not enumerate unrelated directories.

Resolve the containing source's path first, then apply the traversal boundary.
The proposed default boundary is each entry source's directory tree, with an
explicit wider root available when sibling documentation is needed. Resolve
symlinks before checking containment and filesystem identity. A shared device
alone is too broad a scope: it can include unrelated documents.

Keep the logical source location used for relative references distinct from
the resolved file identity used for containment and cycle detection. Define how
symlink aliases with different relative contexts are handled; do not silently
merge contexts whose links resolve differently.

Proposed starting limits, to be confirmed against representative documents:

| Limit | Proposed value |
| --- | --- |
| Unique rendered documents, including explicit inputs | 50 per invocation |
| Link depth, with explicit inputs at depth zero | 3 |
| Source size | 10 MiB per document |
| Total source bytes read | 50 MiB per invocation |
| Total generated session data | 100 MiB |
| Conversion deadline | 60 seconds for discovery and rendering |

Enforce limits during work, including bounded subprocess output. Stop admitting
new linked documents when a limit is reached and report what was skipped.
Images, directories, remote URLs, other filesystems, and non-document formats
are not graph nodes. Multiple fragments of the same document share a page.

Rendering and publication form two phases. First retain rendered documents,
their outgoing references, and their actual HTML identifiers. Then rewrite
links using the set of successful outputs. A failed or excluded target must
never be rewritten to an HTML file that does not exist. Open only explicit
inputs, not every linked document.

Plain Org file links participate alongside Markdown links. Org search suffixes
such as `file:notes.org::#custom-id` and `file:notes.org::*Heading` require
translation to actual generated identifiers. Custom identifiers and unambiguous
heading targets are candidates for support. Arbitrary search expressions,
line-number searches, and ambiguous headings should retain the original target
with an explanation until their semantics are defined.

Missing or unreadable linked documents should not prevent a readable entry
preview. Report the affected links and retain original-file destinations.
Failure of an explicitly requested input produces a non-zero exit status even
if another requested preview can be opened.

## Platform browser handoff

The operator selected macOS, Linux, and WSL support on 8 September 2026 and
confirmed the system default browser. This supersedes the earlier macOS-first
proposal. Keep browser handoff and browser-facing path conversion separate
from Go's filesystem admission and source-relative resolution.

Use the platform's default HTML/URL association: `open` on macOS, `xdg-open`
on a Linux desktop, and Windows shell association through the built-in Windows
PowerShell `Start-Process` command from WSL. These mechanisms use configured
associations; a usable default browser must be configured for local HTML.
Do not select a named browser, alter associations, or fall back to a WSLg
browser. The short PowerShell bridge is owned platform glue, with a fixed
command and the generated URL passed as data through child stdin.
[Linux desktop opening](https://portland.freedesktop.org/doc/xdg-open.html),
[Windows shell associations](https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.management/start-process?view=powershell-5.1).

WSL runs the Linux binary, but Windows must read its generated pages and original
assets. Use `wslpath` to translate resolved local paths before constructing file
URLs, including distribution paths and mounted Windows drives. Keep output in
the private Linux temporary directory; use the existing WSL filesystem bridge
to reach it. Never guess a drive mount or copy output to a more permissive
location. Source URLs cannot supply arbitrary UNC hosts; only application
translations to the current distribution's WSL share are allowed in output.
[WSL path translation and filesystem access](https://learn.microsoft.com/en-us/windows/dev-environment/wsl-interop).

Missing desktop tools, disabled interoperation, inaccessible paths, and failed
handoff require actionable diagnostics. A successful process exit does not
prove browser loading. Validate actual Windows-browser access to temporary
pages, images, linked previews, and fonts before claiming WSL compatibility.
The specification defines the platform matrix and bounded failure behaviour.

## Browser lifetime and cleanup

The existing three-second delay is a compatibility reference, not evidence that
the browser has read the file. Successful desktop handoff does not establish
page readiness. Deleting output also prevents later reloads and linked-page
navigation, regardless of whether the first page loaded.

File transport distinguishes two lifetimes:

- **Quick preview:** open explicit documents, retain output for a configurable
  grace period, then remove the session. Three seconds is the default,
  inherited from the personal command. This mode does not promise reload after
  cleanup or eliminate slow-browser races.
- **Reading session:** keep all generated pages while the command remains
  active; end the session with Ctrl+C or SIGTERM. It does
  not try to infer whether a browser tab is still open.

`HTMLPREVIEW_MODE=quick` or `read` selects file retention. The optional service
has an independent lifetime and handles linked browsing; these settings do not
start or stop it. No detached file-cleanup service is started.

Register cleanup immediately after allocation. Normal exit, conversion failure,
browser-launch failure, and supported termination signals must remove only the
directory owned by that invocation. Report cleanup failure and the remaining
path. Forced process termination or system failure may leave a directory behind;
automatic deletion of other sessions based only on a filename prefix is excluded.

## Installation and local boundaries

Distribute a Go executable for each supported platform, containing its own
presentation assets, Asap, and Iosevka Custom fonts. Project code and documentation
are under [Apache License 2.0](../LICENSE); both font families retain OFL 1.1 as recorded in
[THIRD_PARTY_NOTICES.md](../THIRD_PARTY_NOTICES.md). Build and release checks
must include the appropriate licence files alongside packaged assets.
Pandoc is a separately managed optional runtime dependency; the Go
toolchain is needed only to build from source. Check compatibility before
creating output and give an actionable dependency error.

A macOS Homebrew formula is the selected package-manager route. Pandoc is
optional and separately provisioned for specialist readers; previews never
download or install dependencies. A prefix-based binary installation
serves macOS, Linux, and WSL, with Pandoc provisioned separately and checked by
the command. The current specification defines release targets and qualification
baselines; it establishes no published package name or release URL.
[Homebrew dependency declaration](https://docs.brew.sh/Formula-Cookbook#specifying-other-formulae-as-dependencies).

The operator's installation correction on 8 September 2026 supersedes the
original `/usr/local` default: plain `make install` builds the host executable
and creates `~/.local/bin/htmlpreview` as an absolute symlink to the resolved
checkout binary. The checkout must remain available. Repeated installation
accepts the matching link and atomically replaces different or dangling links.
Conflicting regular files and directories remain protected. Moving the checkout
requires reinstalling. Licence files remain available in the checkout.
A non-empty absolute `PREFIX` retains copied binary and licence installation.
An absolute `DESTDIR` stages either mode; staged default links deliberately
retain their checkout target. Releases and explicit prefix installs remain
independent of the checkout. No shell profile or old installation is changed.

Release candidates contain macOS and Linux executables for amd64 and arm64;
WSL uses the matching Linux executable. Use direct executable invocation with
argument arrays for Pandoc and platform tools. Never interpolate document names
into shell commands. Go remains a build dependency; neither standalone Lua nor
an additional PowerShell distribution is installed for normal use.

The tool does not publish documents or widen file permissions. Temporary-directory
isolation protects generated-file placement; it does not sanitize source HTML
or sandbox Pandoc. Raw HTML, Org includes, document metadata that names external
resources, and source-supplied scripts follow the signed-off passive-content
policy. Graph limits must not be presented as limits on those separate
mechanisms. The template itself needs no network resources.

## Verification and remaining decisions

Inspection on 7 September 2026 found Pandoc 3.9.0.2 installed. Small conversion
probes confirmed that Markdown document links remain source-file links and Org
search suffixes survive parsing without becoming HTML fragment destinations.
These observations support the need for reference rewriting; they do not verify
the proposed application or the prototype's full fidelity.

Implementation verification should cover installation outside the checkout,
real native Org/Markdown and optional Pandoc output, reference resolution from nested and
read-only sources, special characters in paths, service routing and limits, partial
failures, simultaneous invocations, cancellation, and cleanup ownership.
Browser review should cover image loading from temporary pages, linked
navigation, fragments inside folded sections, keyboard use, both themes, print
layout, and a deliberately slow browser launch. No browser harness is selected
by this document.

The initial architecture left public settings, dialects, supported versions,
cleanup, passive content, aliases, traversal limits, and packaging for definition.
The signed-off W001 specification now defines these contracts. Its approved
defaults govern the implementation; the initial proposals remain in Git history.

The regression suite uses the real native converter, Pandoc and synthetic documents. Most command
cases run the same command coordinator in a subprocess with controlled OS
boundaries. An additional staged-install regression selects a desktop capture
adapter with the `htmlpreview_test_desktop` Go build tag, then installs and invokes
the same executable entry point through a symlink from an unrelated directory.
Ordinary builds bind that entry point to NativeHost and include no public browser
override. This replaces the initial source-overlay approach after audit.
Actual browser handoff, clipboard permission, typography, and Linux/WSL host
qualification remain distinct user tests.

Source-controlled warnings have a 64 KiB allowance with an omission message;
lifecycle failures and the retained-session path use separate diagnostics.
Stylesheet placeholders retain their original-resource URLs, including when a
resource is also an explicit preview input. Go-side provenance survives DOM
cloning and prevents discovery or rewriting to generated document navigation.
Long functions in the Org pre-pass and session coordinator retain one ordered
state transition per phase. Their size is a review concern, not a reason to
split context-sensitive preservation or cleanup ownership across hidden globals.

### Native footnote simplification, 14 September 2026

The paired W009 refinement supersedes hidden annotation COMMENT metadata.
Newly saved notes use ordinary native footnote definitions with optional readable
author attribution and inactive Org timestamps, for example
`[2026-09-14 Mon 03:12]`. All supported named footnotes are editable regardless of
origin. The sidebar is labelled **Annotations & Footnotes**; its subtly shaded
note blocks open editing by click or keyboard, without separate Edit buttons.
The toolbar toggle remains **Annotations**.

Native labels and definition digests identify notes on disk. Creation-session
receipts remain bounded and transient; they do not create persisted history.
Read-only sidecars retain native definitions and readable point context, allowing
reconstruction after restart. Original notes without attribution stay unattributed.

The later paired attribution amendment refreshes a matching final `Author:` line
on saved edits, using the current reviewer and local time in
`Author: Taḋg; Edited: [2026-09-14 Mon 03:12]` format. The first save uses Created;
unchanged closes and retries preserve the label and timestamp. This supersedes
preservation of that attribution on edits. Notes without attribution remain valid. Creation
and editing use the same native footnote markup, guarded against breaking out of
the selected definition; they do not infer origin from literal blocks or fences.

### Paired service updates, 15 September 2026

The operator replaced one-second browser polling with SSE invalidations for
annotation-enabled Org/Markdown previews. The existing authorized annotation
endpoint accepts `events=1` on GET. Each visible page keeps one stream; the server
caps concurrent streams at 32 and releases watchers on disconnect or shutdown.
`fsnotify` 1.10.1 provides native directory events, filtered to the selected source
and adjacent sidecar. Directory watches retain coverage after atomic replacement;
there are no recursive watches. Notifications contain no source data. A 100 ms
coalescing window handles bursts; 30-second heartbeat comments do not read files.
Reconnect triggers reconciliation, and saves retain revision guards. Browser
interaction is manual UT; stream and filesystem behaviour use Go HTTP RTs.

LaunchAgent generation captures stdout/stderr in the private service log described
in SERVICE.md. Annotation failures log bounded codes and opaque document IDs,
not request bodies or token-bearing URLs. This supersedes the former discarded
service output. TMPDIR roots expand from the runtime environment when config
loads; generated plists do not freeze a temporary-directory path.

### Extended-attribute save correction, 15 September 2026

Atomic annotation replacement copies extended attributes through already-open
file descriptors, using the existing x/sys dependency. It snapshots the original,
restores owner/mode and attributes on the staged file, then verifies both attribute
sets before rename. An intervening metadata change or copying failure aborts the
save without replacing the original. Names and binary values have a combined
16 MiB allocation bound. This supports ordinary macOS metadata and existing
sidecars; ACLs and macOS flags retain their explicit refusal. The W009 emergency
amendment records the regression evidence and pending live platform checks.

### Explicit section folding controls, 16 September 2026

Section heading text no longer toggles folding in any mode. Only the existing
bar/indicator and Show more controls perform the per-section toggle; toolbar
outline actions and fragment reveal retain their roles. This supersedes the
heading listener and clickable-heading styling described above. Removing the
listener avoids mode-dependent heading behaviour. Heading and hyperlink
annotation eligibility remains unchanged and is separate from folding.

### Heading and link annotation placement, 16 September 2026

The paired amendment enables ordinary heading text and complete-link annotation
points. An after-link flag accompanies the existing transient point request;
the browser derives its canonical position by placing a marker after the anchor
in a detached clone. The source mapper recognizes balanced Org/Markdown links
and the existing Pandoc marker proof verifies the rendered position before any
write. Native source link bytes remain intact. The same heading mapper excludes
Org task states, priority markers and trailing tags. Configured TODO states are
read from the source directives. Ambiguous or unsupported candidates still fail
without source mutation. No annotation persistence format changes.

Document link clicks in annotation mode are intercepted before navigation;
footnote references/backlinks and navigation controls keep their actions.
Heading keyboard placement and pencil cues use the same eligibility rules.
This supersedes the heading/link exclusion in the preceding folding amendment.

### Deferred rendering during annotation editing, 16 September 2026

Annotation state refresh and rendered-document replacement have separate revision
tracking. Autosave and SSE state checks continue while editor focus suppresses
page replacement; successful close applies the latest render. The replacement
path rechecks focus after fetching HTML. Point rebasing requires the displayed
and saved body revisions to agree. External prose changes pause point saves;
leaving the editor refreshes before safe reattachment, retaining drafts on failure.

### Stable annotation feedback and recovery, 17 September 2026

The annotation panel reserves connection and save-status space. SSE failures
receive a 15-second grace period, with a one-second native reconnect hint.
Browser annotation requests and server annotation work have a 15-second deadline.
Slow requests show amber feedback after two seconds without blocking interaction;
the browser reports an explicit timeout when its deadline expires.
Stream and state-fetch failures are tracked separately from confirmed save
failures. A native modal owns Copy, Revert and Try again, keeps drafts selectable,
and prevents background document replacement and automatic save-failure recovery
until a decision. Revert abandons browser edits without rolling back disk content.
This supersedes inline recovery controls above the composer. Source revision,
operation identity and filesystem safeguards remain unchanged.

Long sidebar footnotes have a short collapsed preview and explicit expansion
control. The same full converter-generated endnotes remain available for editing, ordinary
reading and print; truncation never modifies saved text. Interaction evidence is
tracked in the reader-annotation specification's validation record.

### Independent autosave and reader refresh, 17 September 2026

Save receipts advance composer revisions independently of displayed revisions.
SSE notifications and acknowledgements set one pending-refresh flag. The panel
defers background state/render requests and annotation-list replacement until
15 seconds without editor activity or successful editor exit, also waiting for
pending writes and input-method composition. Responses recheck the hold before
application. The existing editor node and its selection/scroll state survive
refresh. This supersedes the preceding focus-only suppression rule.

Automatic writes use the existing 300 ms/two-second debounce, with one request
in flight. Follow-up text returns through scheduling after acknowledgement;
explicit close still flushes through the latest text. Actual stale-write recovery
can request state while presentation remains held. Existing server revision,
native-definition and source-placement checks remain authoritative. No server
write, source format or general merge contract changes. Browser interaction
remains manual UT; the state-flow change has native lint and build checks.

### Verified annotation blocks, 18 September 2026

New annotation creation uses source-bound semantic blocks instead of clicked-word
searches. The annotation boundary proposes native insertion positions outside
literal content and definitions. One additional bounded conversion uses
temporary native footnotes to verify that those references parse. The preview
adapter associates supported block ends with the unchanged rendered structure
and canonical text, then adds application-owned HTML attributes to the original
render. Probe references, definitions and IDs never enter source files or the
published document.

The immutable page cache owns the source-revision-specific block map and charges
its memory to the existing cache budget. Saves accept a mapped block ID only
against the current source and body revisions; browser offsets confer no authority.
The existing atomic writer inserts the reference and native definition. Code
blocks and Markdown headings use a following reference paragraph, preserving
literal syntax and automatic heading anchors. Existing reference paragraphs are
extended. Org headings and prose use inline references at the verified boundary.

Only verified blocks expose an annotation cursor and keyboard/click activation.
The selected block is highlighted. Annotation mode suppresses code copying and
routes inline code and document links to the containing eligible block. Native
footnote controls retain their actions. Existing editing, autosave, refresh and
recovery are reused. Legacy point requests and sidecar context reattachment retain
their existing restrictions; this change does not invent a persisted block-ID
format or approximate reattachment across source changes.

The same after-block convention covers pipe tables and rendered Org block
containers. Source scanning proposes the complete outer block's end; the converter
must produce its expected table, quote, verse or named container before that
boundary is exposed. Nested content selects the outer container rather than
individual cells or internal paragraphs. Hidden comment/export blocks and
unverified syntax remain inactive. No new annotation storage format is introduced.

### Semantic initial heading folding, 18 September 2026

Configuration version 1 now uses a headers mapping with default, todo and done
choices under folding.override. This intentionally replaces the scalar headers
field; the YAML parser reports migration guidance rather than accepting both
shapes. Category choices travel through the existing private registration,
capability/cache identity and application-owned page attributes. The outline
controller uses normalized direct heading task classes, the same semantics used
for active/completed heading styling. A category choice overrides headers.default;
without either choice, authored startup/visibility remains. Restored reader state
wins afterwards. The outer default continues to govern other foldables, including
frontmatter; it does not supply a heading or drawer fallback. No source keywords,
user config files or persisted annotations are rewritten.

### Passive task checkboxes, 18 September 2026

Converter checkbox inputs become owned, labelled span indicators with a common
square style and an explicit text separator. Their label wrappers are unwrapped
before annotation block verification, matching the final passive DOM rather than
leaving native references inside a non-block label. First-position list markers
cover lowercase checked and slash/dash partial extensions; standalone prose and
literal code are excluded. The Org conversion prepass preserves native partial
markers before conversion, without changing source bytes.
This adds neither task-state writeback nor interactive form controls.

### Confirmed native-footnote deletion

The annotation editor's labelled X control uses native confirmation before
submitting the existing authenticated write endpoint's `delete` action. The
composer cancels its debounce, waits for an in-flight save and uses the last
acknowledged definition revision. Confirmed unsaved drafts close locally.
Deletion never automatically rebases across a stale source response.

The native writer removes the selected definition and nonliteral reference
spans through the existing atomic replacement path. Empty generated annotation
reference lines and the selected sidecar context line are removed with them.
Definition/source revisions and read-only boundaries remain enforced. An absent
note with no remaining nonliteral occurrences permits a harmless retry;
a changed or ambiguous definition is refused. Deleted creation sessions become
eligible for the existing bounded-slot eviction. No persistent deletion history
or new endpoint is introduced.

Desktop article grid rows above the body remain content-sized. The body row
absorbs surplus height required by the spanning annotation or navigation pane,
preventing composer growth from inserting blank space before the title. The
phone annotation layout retains its explicit reader/pane split.

## Checkout installation and service replacement, 20 September 2026

[W012 - Replace stale installation links and launchd services](../specs/012-install-service-replacement/spec.org)
supersedes the earlier refusal of mismatched user-local executable links and the
load/unload service sequence. Default installation atomically replaces an existing
symlink with the current checkout target, preserving the former target. Ordinary
files/directories at the executable destination and prefix-copy safeguards remain.

The Go build tool replaces only `gui/<uid>/org.htmlpreview.agent`: prepare the
new plist, boot out the old registration, enable the label and bootstrap the new
plist. Absent jobs are normal. Activation failure restores the previous plist
and attempts to restart the prior job; failures retain actionable diagnostics and
recovery files where needed. Configuration, service logs and other jobs remain
untouched. This supports moving the stable installation between checkouts,
including a submodule, without changing rendering or source authorization.

## Document changes

- Version 9: describe page-local theme selection and unchanged system defaults.
- Version 8: reconcile native Org user acceptance with the retained implementation
  evidence and separate platform-validation limits.
- Version 7: reconcile accepted native Markdown routing, shared endnote handling
  and the raw-HTML boundary; retain superseded converter decisions as history.
- 20 September 2026: native Markdown via Goldmark; Pandoc becomes optional,
  with HTML omission notices and unchanged shared annotation boundaries.

- Version 5: reconcile current converter, transport, folding, annotation and
  installation contracts; label retained superseded designs as history.
- Version 4: document native Org conversion, resource bounds and retained compatibility ownership.

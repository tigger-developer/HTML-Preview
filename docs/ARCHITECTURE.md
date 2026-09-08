# Architecture

**Status:** Design adopted through
[the approved local-preview specification](../specs/001-local-document-preview/spec.org)
on 8 September 2026. Implementation and verification are in progress. Earlier
proposal wording below is retained as design history; the specification supplies
the exact adopted contracts. Browser and release qualification require the
separate validation record.

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

Direct dependencies are `golang.org/x/net` 0.58.0 for HTML5 parsing and
`github.com/microcosm-cc/bluemonday` 1.0.27 for allowlist sanitization. The
standard library has no equivalent parser/sanitizer. Their module checksums are
tracked; the two transitive CSS-parser dependencies and their licences are
recorded in [the notices](../THIRD_PARTY_NOTICES.md). No source CSS is enabled
merely because the sanitizer contains a CSS parser.

`htmlpreview` is a local Go command-line application that uses Pandoc to render
Markdown and Org as HTML, repairs references for temporary output, and opens
the result in a browser. [VISION.md](VISION.md) defines the product intent.

The application owns one private temporary directory per invocation. Sources
remain in their original locations and are read without modification. The
browser reads generated files through local file URLs; no HTTP server is needed
for the proposed design.

The processing sequence is:

1. Validate input files, installed Pandoc, and invocation settings.
2. Allocate the session directory and register its cleanup.
3. Render the explicit inputs and, when enabled, discover bounded linked inputs.
4. Finalize the source-to-preview mapping and rewrite document references.
5. Publish the generated pages within the session and open the explicit inputs.
6. Retain the session for the selected reading mode, then clean up its directory.

## Existing foundations

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
| Renderer | Org pre-processing, Pandoc invocation, packaged assets, and conversion errors |
| Reference resolver | Source-relative URLs, filesystem identity, document graph, and output mapping |
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

For Org, preserve planning information and logbooks before Pandoc parses the
source, then apply the adapted Lua filter. Pre-processing must recognize literal
source and example blocks so that text inside them is not transformed. Keep
section wrappers for outline folding. Preserve source content and identifiers;
do not recompute task statistics or execute source blocks.

The browser script should remain a small enhancement. Ordinary reading and
navigation must work without it. Folding must preserve keyboard navigation,
visible focus, and fragment destinations, including revealing folded ancestors
when a link targets their contents.

The original filename header is required, including copying its full source
path. Preserve the personal template's directory followed by an emphasized
filename, compact right alignment, and separator above the document body.
Use Iosevka Custom at 400 for the directory and 700 for the filename, with
accessible contrast and wrapping for long paths. The browser tab title uses
the original filename. This requirement, confirmed on 8 September 2026,
supersedes the earlier optional-copy proposal.

Display and copy the absolute logical source path used for that preview,
including the Linux path when viewed from WSL. Do not copy the temporary HTML
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

Application styling is included locally. Source images remain references to
their original files unless a later requirement explicitly adds embedding.
Pandoc's `--embed-resources` also incorporates document resources and can fetch
remote assets, so it is not the default mechanism for packaging only the theme.
[Pandoc resource options](https://pandoc.org/MANUAL.html#option--embed-resources).

### Code, contents and browser controls

[W002 - Code and document navigation](../specs/002-code-and-outline/spec.org)
extends the approved renderer. Pandoc always runs with standalone output and
the owned `page.html5` template. This intermediate wrapper includes only the
generated body and optional contents; the final Go template adds the original
source header, fonts, policy and browser script once.

`HTMLPREVIEW_TOC` defaults to `1` for Markdown and `0` for Org, accepting an
explicit `0` or `1` for all documents in the invocation, including linked pages.
`HTMLPREVIEW_TOC_DEPTH` defaults to `3`, accepting source levels `1` through `6`.
Empty values select defaults. Configuration retains whether TOC was explicitly
set; rendering selects the effective default per document using the same format
decision as Pandoc input. The shared configuration is never changed by rendering
an Org file, so mixed inputs and linked targets remain independent. Depth alone
does not enable Org contents. Validation precedes session allocation, including
depth when contents are disabled. Source TOC metadata does not override these
application choices. No standalone setting is exposed.
This follows [W003 - Format-specific contents defaults](../specs/003-format-contents-defaults/spec.org),
which supersedes W002's original default-on choice for both formats.

The Org pre-pass preserves source-block bodies in string-only JSON records in
an unpredictable per-document raw format. Lua validates the records and makes
real Pandoc code blocks. Every AST code block receives an owned association
and literal-value record. Pandoc's built-in highlighter supplies semantic spans;
the owned stylesheet supplies the light/dark palette. Shell aliases `sh` and
`shell` use Bash. Unknown languages and examples remain plain.

Go validates and removes only its transport records before source sanitization.
It retains writer text when exact, restores a single omitted final newline,
or falls back to the exact plain value with a bounded diagnostic. Source and
output limits cover conversion input, transport output and final pages.
No language label selects an external syntax definition or executes code.

Temporary heading identities bind Pandoc's contents to concrete heading nodes.
Go restores source identifiers, applies Org properties and allocates unique
final IDs before retargeting those links. Contents are separated before source
graph discovery and receive their own passive sanitization. Heading images use
alternative text in contents labels; document images keep normal resolution.
Empty contents are omitted. The filename header never enters Pandoc's outline.

The browser module owns one clipboard service for the source header and code.
It captures literal text before inserting native copy buttons and permits one
pending write per page. Code within links keeps navigation; its copy button is
outside the link. Direct copying respects active selections, dragging and
modifier clicks. Success follows the resolved write; failure exposes a labelled
readonly field without reading the clipboard or moving focus.

Each enhanced section reserves a separate gutter for its native folding button.
The hit target fills its visible height and is at least 24 CSS pixels wide.
A hidden body uses a four-pixel accent bar and bottom plus; a visible body uses
a one-pixel bar. Local activation switches between fully open and folded.
Global modes, initial Org visibility, fragment reveal and print restoration
retain their existing roles. Control names capture heading text before code
buttons are inserted. Lifecycle teardown removes listeners, controls and timers;
page restoration enhances the passive document once.

## Temporary files and reference resolution

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
links emitted by Pandoc and literal HTML anchors preserved in source documents.
The proposed dependency is `golang.org/x/net/html`, the Go project's HTML5
parser. It handles HTML structure; the standard library's escaping helpers and
XML parser are insufficient substitutes. Its maintained release series is a
reason to prefer it to a project-written parser; select and pin the actual
version during implementation.
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

## Optional linked-document conversion

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

The proposed interface distinguishes two lifetimes:

- **Quick preview:** open explicit documents, retain output for a configurable
  grace period, then remove the session. Three seconds is the initial candidate
  inherited from the personal command. This mode does not promise reload after
  cleanup or eliminate slow-browser races.
- **Reading session:** keep all generated pages while the command remains
  active; end the session explicitly from the terminal. Linked browsing uses
  this lifetime so pages remain available until the reader ends it. It does
  not try to infer whether a browser tab is still open.

The exact interaction and non-interactive retention option remain to be
specified. Do not start a detached cleanup service or add a local server merely
to retain files. Keep the whole linked set for the same lifetime.

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
Pandoc is a separately managed runtime dependency; the Go
toolchain is needed only to build from source. Check compatibility before
creating output and give an actionable dependency error.

A macOS Homebrew formula declaring Pandoc is the selected package-manager route.
This provides dependency installation through the package manager rather than
download or installation during previewing. A prefix-based binary installation
serves macOS, Linux, and WSL, with Pandoc provisioned separately and checked by
the command. The current specification defines release targets and qualification
baselines; it establishes no published package name or release URL.
[Homebrew dependency declaration](https://docs.brew.sh/Formula-Cookbook#specifying-other-formulae-as-dependencies).

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
real Pandoc output for both formats, reference resolution from nested and
read-only sources, special characters in paths, graph cycles and limits, partial
failures, simultaneous invocations, cancellation, and cleanup ownership.
Browser review should cover image loading from temporary pages, linked
navigation, fragments inside folded sections, keyboard use, both themes, print
layout, and a deliberately slow browser launch. No browser harness is selected
by this document.

The initial architecture left public settings, dialects, supported versions,
cleanup, passive content, aliases, traversal limits, and packaging for definition.
The signed-off W001 specification now defines these contracts. Its approved
defaults govern the implementation; the initial proposals remain in Git history.

The regression suite uses real Pandoc and synthetic documents. Most command
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

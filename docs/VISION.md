# Vision

**Status:** Product direction approved on 8 September 2026 through
[the local-preview specification](../specs/001-local-document-preview/spec.org).
That approval includes opt-in linked browsing. Implementation and verification
are in progress; the proposal wording below records the design's development
and does not establish release qualification.

## Purpose

`htmlpreview` makes local Markdown and Org documents comfortable to read in a
browser. A globally installed command converts a document to a temporary HTML
preview, applies a carefully maintained visual presentation, and opens it in
the default browser.

The tool should remain small and direct: select a document, read it, and leave
the source untouched. Pandoc provides document conversion; Go manages the
application and its temporary files.

## The reading experience

The intended basic invocation is:

```sh
htmlpreview README.md
```

Multiple explicit inputs should produce separate previews, preserving the
existing command's useful batch behaviour:

```sh
htmlpreview README.md docs/notes.org
```

The reader should receive:

- **Readable presentation:** embedded Asap variable fonts, considered spacing,
  tables, code, quotations, lists, images, footnotes, and light and dark
  appearances. Asap provides adjustable weight and width; code blocks, inline
  code, and other fixed-width text use embedded Iosevka Custom. Declared code
  languages receive local syntax highlighting while literal text remains intact.
- **Org fidelity:** recognizable task states, tags, priorities, timestamps,
  planning information, drawers, and an outline that can be folded.
- **Direct document navigation:** complete styled pages with Markdown contents
  enabled by default through source heading level three and Org contents off
  by default. Contents can be enabled, disabled or given another depth.
  Clickable margin bars open and close sections.
- **Code copying:** inline code, Org verbatim and code blocks offer explicit
  copying with keyboard controls, honest feedback and manual fallback. Selection
  and links retain their normal actions. Code is never executed.
- **Source context:** a visible filename and location, without allowing a long
  path to dominate the document. Keep the original directory-and-filename
  header, with the filename emphasized; activating it copies the full original
  source path. Keyboard activation and clear success/failure feedback are part
  of this interaction. The path remains selectable without JavaScript.
- **Working local references:** images and ordinary file links continue to
  resolve relative to the document that contains them.
- **Predictable lifetime:** a preview remains available for the reading mode
  selected, with an explicit cleanup policy.

Fidelity means preserving useful document structure and meaning. It does not
promise a full Emacs implementation or execution of Org source blocks.

The code-copying, margin-bar and contents direction was approved through
[W002 - Code and document navigation](../specs/002-code-and-outline/spec.org)
on 8 September 2026. Standalone output is always enabled.
The operator's later [W003 - Format-specific contents defaults](../specs/003-format-contents-defaults/spec.org)
changes the Org default to off, superseding W002's default-on setting for both
formats while retaining explicit overrides.

## Temporary output

Each invocation should own a private directory allocated through the operating
system's temporary-directory facility. Generated HTML, packaged presentation
assets needed during conversion, and session records belong there.

Relative references must be resolved against each original source document and
rewritten for its generated page. This permits previews of read-only source
directories and keeps generated files out of working trees.

The earlier approach of placing randomly named HTML beside each source was
useful because it preserved relative paths without rewriting. Temporary-directory
output supersedes that approach, provided reference handling is verified.

## Linked-document browsing

An optional extension would let a reader follow links between local Markdown
and Org documents without manually previewing each target.

The proposed approach is to convert the reachable documents before opening the
entry page, then direct their links to the generated HTML. Traversal should:

- Follow actual document links, including links across Markdown and Org.
- Stay within an explicit directory boundary and the entry document's
  filesystem.
- Stop at finite document, depth, size, and conversion-time limits.
- Handle cycles and repeated references without repeated conversion.
- Explain skipped targets while retaining a route to their original files.

The goal is browsing a useful neighbourhood of documents. Whole-filesystem
indexing and website publication are outside this scope. Concrete initial
limits and link semantics are proposed in [the architecture](ARCHITECTURE.md).

## Installation and technology

The distribution should install `htmlpreview` on `PATH` and declare Pandoc as
a runtime dependency. Normal use should not require a Go toolchain, a source
checkout, personal shell functions, or a user's existing Pandoc configuration.

Support macOS, Linux desktops, and WSL. Open the system default browser;
from WSL this means the Windows default browser, including when WSLg is
available. Translate local references for that browser without changing system
associations, file permissions, or WSL configuration.

Fonts belong to the application and each generated page. Installation copies
the executable and licence notices; it does not install system fonts. Running
through a symlink on `PATH`, or from another directory, must retain the same
fonts and presentation.

The selected stack is Go, Pandoc, Lua where useful for Pandoc transformations,
HTML/CSS, and limited JavaScript running only in the browser. Command-line
JavaScript runtimes are excluded. Bash may support build or packaging tasks;
application logic belongs in Go.

Project-owned code and documentation, including the Org fidelity code, use the
[Apache License 2.0](../LICENSE). The bundled Asap fonts retain their separate
[SIL Open Font License 1.1](../assets/fonts/asap/OFL.txt). Their regular and
italic WOFF2 files should be embedded in the distributed application and its
generated previews, without a font download at viewing time.

The selected [Iosevka Custom faces](../assets/fonts/iosevka-custom/README.md)
provide regular, italic, bold, and bold italic fixed-width text. Their separate
[SIL Open Font License 1.1](../assets/fonts/iosevka-custom/OFL.md) and copyright
notice accompany the fonts in packages and generated previews. Selecting
Iosevka Custom on 8 September 2026 specifies the earlier generic monospace
requirement; Asap remains the proportional default.

## Boundaries and priorities

The first priority is a dependable single-document preview informed by
[the prototype](../prototype/README.md). Reused prototype code is maintained
project code: its structure, correctness, Org coverage, accessibility, and build
integration must be assessed and improved where necessary. Its existing shape
is not a compatibility requirement. Linked browsing is a separate extension
and must not make basic previewing cumbersome.

Editing, live reload, a persistent background service, network hosting, remote
document crawling, PDF conversion, and a general plugin system are outside the
initial scope. Ordinary external hyperlinks remain ordinary hyperlinks.

Success should be demonstrated by installing the packaged command, reading
representative Markdown and Org documents, following their supported references,
and verifying that source files remain unchanged and cleanup affects only the
invocation's own temporary output. Visual quality requires human review.

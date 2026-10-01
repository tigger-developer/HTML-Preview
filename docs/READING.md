---
title: Reading and navigation
version: 1
last-updated: 2026-10-01
---

# Reading and navigation

The examples below run from the project root. For a first preview:

```sh
htmlpreview README.md examples/work.org
```

## Browser controls

Each distinct source context receives a separate preview. The header identifies
the original source; activating the filename text copies its full logical path. When browser
clipboard access is unavailable, the header offers manual copying.

The sun/moon button immediately before the filename switches light and dark
appearance. New pages follow the system theme until the button is used. A manual
choice lasts for the current page, including annotation refreshes and plaintext
view; reloading or opening another page returns to automatic detection. The
button supports keyboard activation and is hidden when printing or without JavaScript.

Code uses local syntax highlighting and the embedded Iosevka Custom font.
In reading mode, click inline code, Org verbatim or a code block to copy its literal text, or
activate its copy glyph with the keyboard. Existing selections and dragging
retain normal selection behaviour. Code inside a link uses a separate copy
button so the link keeps its navigation action. Clipboard refusal offers a
readonly field for manual copying; empty code has no enabled copy action.

The [Org ledger](../examples/work.org) demonstrates TODO markers, tags, drawers,
planning and source blocks. The [Markdown companion](../examples/code.md) includes
highlighting, inline code, duplicate headings and long lines:

Org previews render `#+TITLE` and `#+SUBTITLE` as leading document headings.
When present, `#+AUTHOR` and `#+DATE` follow them as document metadata.
Leading Org fields share one left-aligned, initially open frontmatter panel
above the separator, using compact Iosevka text and a settings glyph. The
filename is left-aligned after the theme toggle in the sticky info bar, with
reading controls to the right. Frontmatter never activates path copying.
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
bar opens a folded section; the thin bar closes it. Heading text never toggles
folding. Folded sections show a large disclosure triangle and a labelled
Show more button. These supersede the earlier small bottom-plus indicator.
Overview, Contents and Show all sit beside the filename in subtly coloured
buttons for both formats. By default, generated Org drawers remain closed when
sections open. [Folding configuration](SERVICE.md#initial-folding) can set
different initial states for headings, semantic TODO/DONE categories and drawers.
Tags are plain muted-pink text. Frontmatter and drawers use a quieter background
than code. Drawer summaries name the drawer once; property keys have muted
labels and darker backgrounds, while values retain the normal foreground.
Opening and closing drawer delimiters are omitted, and free text has no inner box.
Without JavaScript, the full document remains open and code stays selectable.
Printing includes all content and hides the interactive controls.

## Preview lifetime and linked browsing

Quick previews retain their files for three seconds after the last browser
handoff. This delay does not establish browser readiness. Use a reading session
for slow browser startup or reloading:

```sh
HTMLPREVIEW_MODE=read htmlpreview README.md
```

The command stays in the foreground until Ctrl+C or SIGTERM. It then removes
only its own session directory. SIGKILL or a system failure can leave that
directory behind; the reported path identifies the exact directory for manual
removal. The optional service has its own lifetime; there is no cross-session
scavenger.

For linked browsing, configure and explicitly start the optional service as
described in [the service guide](SERVICE.md). HTTP-only invocations return
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

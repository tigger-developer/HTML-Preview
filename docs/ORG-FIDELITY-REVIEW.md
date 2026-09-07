# Org fidelity code assessment

**Status:** Initial design review, 7 September 2026. Findings come from reading
the prototype sources and small Pandoc 3.9.0.2 conversion probes. Browser
behaviour has not been exercised. This is not an independent audit or approval
of the implementation.

## Recommendation

Treat the prototype as owned application code. Retain useful rendering ideas,
but integrate them through the Go renderer, normal asset packaging, and tests
of generated output. The shell wrapper and unrestricted pre-pass should not
become production dependencies.

The existing three-stage concept is appropriate: preserve Org information
before parsing, transform the parsed document, and style or enhance the result.
The responsibilities need clearer interfaces and stronger preservation rules.

## Findings

### Source preservation needs context

[org-prepass.awk](../prototype/org-prepass.awk) rewrites any matching planning
line or `:LOGBOOK:` line. Its complete rule set contains no state for source
blocks, example blocks, comments, or drawer context. Static inspection therefore
shows that matching literal examples would also be rewritten. Renaming every
logbook to `LOGBOOK_` also uses a name that a source document can already contain.
The pre-pass was inspected but not executed during this review.

The Go replacement should preserve literal regions byte-for-byte and retain
planning and drawer metadata through an explicit intermediate representation.
Support real Org block and drawer boundaries, indentation, and keyword case.
Do not replace this with a broader collection of regular expressions or attempt
to duplicate the entire Org parser. Keep Pandoc responsible for ordinary Org
syntax; preserve only information demonstrably lost at its boundary.

### Identifier handling must match the current reader

[org-fidelity.lua](../prototype/org-fidelity.lua) tries to recover an `id`
attribute on a heading by renaming it to `org-id`. The current probe gives a
different boundary:

- A heading with only `:ID: only-id` produces the HTML section identifier
  `only-id` directly.
- A heading with `:ID: example-id` and `:CUSTOM_ID: target` produces identifier
  `target`; `example-id` is absent from the parsed document.
- An ordinary `:OWNER: Reader` property survives and the filter renders its
  property drawer.

Consequently, a filter after parsing cannot recover every original ID. Capture
both IDs before parsing when both must be retained, carry their association with
the heading, and emit distinct alias anchors where required. Derive link targets
from actual generated identifiers. Test repeated headings and conflicting IDs
without depending on the prototype's historical duplicate-attribute explanation.

### Inline recognition is deliberately narrow

The Lua filter recognizes priorities and statistics cookies with token patterns.
Timestamp recognition searches at most seven inline tokens and expects specific
token types. The probe confirmed basic priorities, cookies, tags, timestamps,
and a repeater, but not full Org timestamp syntax.

Define supported timestamp ranges, time ranges, repeaters, warning delays,
punctuation, and statistics cookies. Tests should verify text preservation as
well as styling, including negative examples and literal code. Separate inline
recognition from property and heading transformations so each has a small,
reviewable contract. Remove obsolete comments and redundant temporary values
when the code is revised.

### Folding intercepts normal keyboard navigation

[org-fold.html](../prototype/org-fold.html) attaches a document-wide handler that
prevents the default action of every Shift+Tab event. It also prevents Tab on
focused headings. Static inspection establishes interception of the browser's
ordinary focus-navigation keys, even outside an outline control for Shift+Tab.

Use explicit focusable controls and preserve normal Tab and Shift+Tab traversal.
Offer outline shortcuts only within a clearly scoped interaction. Ensure visible
focus, consistent accessible expanded state, and usable touch targets. Keyboard
behaviour needs browser validation after revision.

### Folding state should be separate from initial metadata

`setState` both changes the requested section and recursively changes child
sections. Child behaviour depends on the presence of `data-visibility`, even
when the reader is applying a later interaction. There is no fragment-navigation
handler to reveal a destination hidden in a folded ancestor.

Read initial visibility once into explicit outline state. Keep changing one
section separate from applying a global operation. Add fragment handling on
initial load and subsequent navigation, with a defined rule for revealing
ancestors. Repeated initialization must not duplicate controls or handlers.

### CSS and printing need a single presentation model

[org-fidelity.css](../prototype/org-fidelity.css) has reusable colour and spacing
variables, but its body defaults to a serif stack rather than the selected Asap
font. Named TODO colours include fixed values outside the theme variables.
Initial nested margins are later reset, and `data-theme` changes only
`color-scheme`; the actual colour variables follow the operating-system query.

The print rules reveal `.org-folded` content but do not override the separate
`.org-contents` hiding rule or explicitly open closed property drawers. These
are source-level observations; their visual consequences remain to be checked.

Extract common document styling from Org-specific decoration, adopt embedded
Asap, consolidate theme variables, and define print visibility for every fold
state. Keep a readable fallback when JavaScript is disabled. Check colour
contrast, narrow screens, long paths, tables, and source blocks in a browser.

### The wrapper should be replaced, not nested inside Go

[org2html](../prototype/org2html) locates adjacent assets and runs an AWK-to-Pandoc
pipeline under `/bin/sh` with `set -eu`. It does not enable pipeline failure
propagation, and its output default is derived from the source filename. Those
choices do not provide the selected application's temporary-session, cancellation,
or error-handling contract.

Go should invoke Pandoc directly with explicit arguments, own preprocessing and
temporary files, and surface each failure. Package the revised Lua, CSS, and
browser script through the same build as the Go executable. Do not build or
invoke a second application from the prototype directory at runtime.

## Evidence needed during implementation

Create small synthetic Org fixtures for source blocks containing planning
examples, nested and custom drawers, planning fields, ID/custom-ID combinations,
custom TODO sequences, tags, priorities, timestamps, checkboxes, definition lists,
tables, verse, and links. Assert source preservation and actual generated HTML;
avoid tests that only inspect implementation text.

Use real Pandoc at the rendering boundary. Add browser review for focus order,
folding, fragment navigation, print visibility, and typography. The existing
large example document can supplement these cases, but should not be the sole
test fixture or be treated as approved output.

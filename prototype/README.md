# org-fidelity — high-fidelity HTML from `pandoc -f org`

A stylesheet + Lua filter + fold script that render pandoc's org conversion
close to what Emacs shows you: TODO states, tags, priorities, timestamps,
property drawers, custom drawers, and click-to-fold headlines.

## Files

| file | what it does |
|---|---|
| `org-fidelity.css` | the stylesheet — targets the DOM pandoc's org reader actually emits |
| `org-fidelity.lua` | filter: property drawers → `<details>`, priorities/cookies/timestamps → spans, fixes the duplicate-`id` bug |
| `org-fold.html` | `--include-after-body` script: outline folding, TAB / SHIFT-TAB cycling, Overview/Contents/Show-all toolbar |
| `org-prepass.awk` | rescues `DEADLINE:`/`SCHEDULED:`/`CLOSED:` and `:LOGBOOK:`, which the reader discards before filters run |
| `org2html` | wrapper that wires all of the above together |

## Use

```sh
./org2html notes.org notes.html
```

or by hand:

```sh
awk -f org-prepass.awk notes.org |
pandoc -f org -t html5 -s --section-divs \
       --lua-filter=org-fidelity.lua \
       --css=org-fidelity.css \
       --include-after-body=org-fold.html \
       -M document-css=false \
       --embed-resources \
       -o notes.html
```

`--section-divs` is not optional: without it there are no `<section>`
wrappers, so nothing can fold and the property `data-*` attributes have
nowhere to land.

`-M document-css=false` suppresses pandoc's own built-in stylesheet (note
`-M`, not `-V` — `-V document-css=false` sets a *truthy string* and does
nothing).

## What pandoc already preserves (no filter needed)

| org | HTML |
|---|---|
| `TODO` / `NEXT` keyword | `<span class="todo TODO">` |
| `DONE` / `CANCELLED` | `<span class="done DONE">` |
| `:tag:` | `<span class="tag" data-tag-name="tag">` |
| `:PROPERTIES:` entries | `data-*` attributes on `<section>` and the heading |
| `:CUSTOM_ID:` | the element `id` |
| `:VISIBILITY: folded` | `data-visibility="folded"` |
| `:MYDRAWER:` … `:END:` | `<div class="MYDRAWER drawer">` |
| `#+CAPTION:` | `<div class="captioned-content"><div class="caption">` |
| `#+begin_src lang :args` | `<div class="sourceCode" data-args="…">` |
| `#+begin_example` | `<pre class="example">` |
| `#+begin_verse` | `<div class="line-block">` |
| `#+begin_foo` (any block) | `<div class="foo">` |
| `#+ATTR_HTML: :class x` | `class="x"` on the element |

## What it adds

* `<details class="org-properties">` — the property drawer, rendered and foldable
* `<span class="priority priority-A">` — `[#A]`
* `<span class="cookie">` — `[1/3]`, `[50%]` (`.cookie-done` when complete)
* `<span class="timestamp active|inactive|repeater">` — `<2026-09-20 Sun>`, `[…]`
* `<span class="tags">` wrapper so tags float right in source order
* `data-org-id` instead of a second `id` attribute (org's `:ID:` and
  `:CUSTOM_ID:` otherwise both become `id=`, which is invalid HTML)
* `<div class="planning">` and `<div class="LOGBOOK_ drawer">` (via the awk pre-pass)

## Known limits

* **Planning lines and `:LOGBOOK:` are dropped by the reader**, before any
  Lua filter can see them. The awk pre-pass is the workaround; without it
  they simply are not in the output.
* `#+STARTUP: overview` is not carried through — set folding per-headline
  with `:VISIBILITY:`, or click **Overview** in the toolbar.
* `- term :: definition` only becomes a `<dl>` when the list isn't mixed with
  checkbox items.
* Statistics cookies in headlines are text, so `[1/3]` is styled but never
  recomputed.

## Customising

Everything is a CSS custom property at the top of `org-fidelity.css`.
Add your own `#+TODO:` keywords by copying the `.todo.NEXT` pattern —
pandoc puts the literal keyword in the class list, so `.todo.WAITING`,
`.done.DELEGATED` and so on all work.

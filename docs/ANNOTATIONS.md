---
title: Document annotations
version: 3
last-updated: 2026-09-19
---

# Document annotations

Service previews of genuine Org and Markdown files offer an **Annotations**
button in the persistent info bar. Comments autosave as native footnotes in the
source. Read-only sources use an adjacent Org sidecar. Reading a page never
creates annotations; file previews, code, plain text, binary documents and native
HTML remain reading-only.

This is the [W009 native-footnote candidate](../specs/009-reader-annotation-ux/spec.org).
Its [validation record](../specs/009-reader-annotation-ux/validation.org) separates
Go/HTTP checks from pending browser, accessibility and platform qualification.

**Superseded contract:** W007 originally stored successive JSON draft events
and attached comments to selections or whole documents. On 14 September 2026,
Taḋg withdrew that history and selected insertion-point footnotes. The
[original specification](../specs/007-service-annotations/spec.org) retains the
historical contract. Current saves retain only each annotation's latest value.

## Start and annotate

Start the [local service](SERVICE.md) from a documentation directory:

```sh
htmlpreview --serve
```

From another terminal, open a document with the desired attribution:

```sh
HTMLPREVIEW_USER_DISPLAY_NAME='Review name' htmlpreview notes.org
```

An empty or unset display name falls back to `USER`. Both values are trimmed.
Missing identity permits reading saved comments but disables creation. Invalid
non-empty names fail before browser opening: names permit at most 128 Unicode
characters and 512 UTF-8 bytes, without controls or line breaks. The initiating
command captures the name; the service account does not substitute its own name.
Existing tabs and links retain their captured attribution.

1. Activate **Annotations**, then click an eligible paragraph, heading, list-item paragraph, code block, table or rendered Org block.
2. Type in the focused comment box. **Auto saved** means the latest value was acknowledged.
3. Optionally change the subdued **Footnote ID** below the comment box.
4. Click outside the editor, move keyboard focus out, or press Escape to finish
   the comment. Saving must be acknowledged before the editor closes.

Annotation mode uses a ✎ pencil cursor over verified blocks, with a crosshair
fallback. An outline highlights the selected block while editing. The reference
is appended at the block boundary, outside inline formatting and links. Code
blocks, tables, rendered Org blocks and Markdown headings use a following
`Annotations:` paragraph; repeated
annotations extend that paragraph. This preserves Markdown heading anchors.
Table cells and nested Org blocks belong to the whole outer block; their notes
go after its closing syntax. Hidden comments and drawers remain inactive.
Unsupported locations do not open a composer. After close, the normal footnote
number and link remain. **Auto saved** appears beneath the textarea for two seconds before
fading, accompanied by a green border flash. The feedback keeps its reserved
space after fading. Sustained connection problems and confirmed save failures
open a recovery dialog without adding controls above the editor.

Checkbox list items are annotation targets too. Their unchecked `[ ]`, checked
`[X]`/`[x]` and partial `[-]`/`[/]` states use equally sized passive indicators
with spacing before the item text. Clicking an item in annotation mode attaches
its footnote at the item end; it does not toggle the task state. Source checkbox
spelling is preserved. Standalone bracket markers and code remain literal.

Keyboard activation uses Tab to reach an eligible block and Enter or Space to
open its composer. Folding controls retain their own actions. Code copying and
its buttons are disabled in annotation mode; clicking inline code or a document
link selects its eligible containing block. Reading mode restores code copying
and normal navigation. Footnote references and backlinks remain navigable in
both modes.

The **Annotations & Footnotes** sidebar gives each note a subtle background.
Long notes initially show approximately five text lines. The **…** button
expands the note; **Show less** collapses it. Editing always shows the full text.
Normal endnotes and printed notes remain unabridged.
Click a note's content area, or focus it and press Enter, to edit it. This applies
to ordinary authored footnotes as well as comments created in HTML Preview. The editor loads the current content from the file.
The existing ID is read-only and all references keep that ID. Ordinary notes
retain their Org/Markdown markup; unattributed notes remain unattributed. On a
saved edit, a matching final Author line receives the current reviewer's name
and local date/time in Org format, labelled **Edited**. The first save uses
**Created**; unchanged closes and retries retain the existing attribution.
Closing and reopening the page does not affect editability. This supersedes the
earlier closed-comment and HTMLPreview-only editing restrictions.

The **✕** control in the editor is labelled **Delete annotation**. It opens a
confirmation dialog. Cancel keeps the note and editor; confirmation removes the
selected footnote definition and its references. It waits for any active save
first. An unsaved draft is discarded without writing it. Ordinary authored
footnotes use the same control. Read-only original notes remain protected;
sidecar notes are deleted in their sidecar. Changed source or definition
revisions reject deletion and retain the draft in the recovery dialog.

The ID defaults to a normalized display name and the next unused counter:
`Taḋg` produces `tadg-001`; `Tadhg O'Brien` produces `tadhg-o-brien-001`.
Ordinary footnotes also reserve their labels. IDs accept 1--64 ASCII letters,
digits, underscores or hyphens, beginning with a letter. A conflicting ID pauses
saving and remains editable; it is never silently renamed.

Autosave waits 300 milliseconds after typing pauses and submits within two
seconds during continuous typing when no previous save is in flight. Only one
save runs at a time. Text entered during a save is batched through the debounce
after acknowledgement; it does not trigger immediate back-to-back submissions.
Leaving the editor flushes the latest text. Input-method composition suspends submission.
Comments accept up to 4,000 Unicode characters and 16 KiB. Opening an empty
composer writes nothing. Clearing an active saved draft removes its owned
reference and definition. Editing an existing footnote requires non-empty text;
clearing it does not delete it. Recovery now uses **Copy**, **Revert** and
**Try again** in a modal dialog, as described below. This replaces the earlier
inline Restore last autosave, Use current footnote and Copy draft controls.

Trailing line breaks are accepted while typing and omitted from the saved note
text. Internal paragraph breaks, spaces and native markup remain intact. A change
consisting only of trailing line breaks does not refresh the attribution.

## Reading and source inspection

Pandoc renders ordinary and owned footnotes in one endnotes section. Annotation
mode moves that same section into the comment pane; switching it off returns
it to the document end. Native reference links and backlinks remain available.
Printing uses the normal endnotes, without a duplicate annotation appendix.

The info bar keeps the source filename left and controls right while scrolling.
The displayed home directory is abbreviated as `~`; copying retains the absolute
source path. Wide viewports show navigation, the article and the annotation pane.
Phone annotation mode keeps document text above a larger comment pane.

**Show plaintext** displays the original Org or Markdown source, including
embedded footnotes and their optional attribution. It disables annotation and retains the
info bar. **Overview**, **Contents** or **Show all** returns to formatted content
and applies that folding preset. Sidecar content is not substituted for source text. TXT, source-code
wrappers, HTML and binary documents do not offer this view.

## Source changes and failures

Visible annotation-enabled pages keep one server-sent event connection open.
The service watches the source directory for document and sidecar changes,
including atomic replacements, and notifies the browser that a refresh is pending.
Notifications and save acknowledgements coalesce into one pending update.
This supersedes the previous one-second polling. Refresh
preserves the active composer. A saved embedded reference identifies its source
point; sidecar points use unique before/after context. Missing or ambiguous
locations remain explicitly unplaced in the endnotes. There is no fuzzy matching
or automatic movement to a nearby paragraph.

New previews use source-bound block IDs and revision checks. Repeated words and
rendered punctuation do not require source-text searches. The conversion checks
that native references parse at proposed boundaries before making blocks
interactive. Temporary markers and block IDs are never written into the source.

The previous clicked-text path considered at most eight matching source
fragments. That limit remains only for compatibility with older point requests;
it does not govern block creation in newly opened previews. Reload an older
preview to use block targets. Sidecar reattachment retains its context matching;
unprovable sidecar locations remain explicitly unplaced.

**Not saved** retains the draft. Temporary
failures retry the same operation after one, two and four seconds, with a small
jitter; persistent failures require an explicit decision in the recovery dialog:

- **Copy** copies the retained text and keeps the dialog open. If clipboard
  access fails, the text remains selectable for manual copying.
- **Try again** retries the save with its existing operation identity and
  revision checks. A failed retry keeps the dialog and text available.
- **Revert** abandons unsaved browser edits and returns to reading. Anything
  already acknowledged on disk stays saved; this does not roll back the file.
  Connection recovery pauses until annotation mode is activated again.

Escape and backdrop clicks cannot dismiss this dialog. Retry and Revert wait
while a save is still in flight. Closing, changing points,
leaving annotation mode and application navigation wait for acknowledged saves.
Browser navigation warnings are best effort: terminating the process can still
lose text that was never acknowledged.

New annotations patch their own reference tokens and definitions. Explicit
footnote edits patch only the selected definition in a fresh source snapshot. A private sibling temporary file is flushed and atomically renamed,
then its directory is synchronized before success. Owner and mode are preserved;
extended attributes are copied and checked before replacement. Unsupported ACLs
or file flags still prevent replacement rather than being silently removed. File identity, source content and permissions are
rechecked before publication. Uncertain acknowledgement retains retry recovery.

An unrelated editor can still save an old buffer over newer comments. Filesystem
rename is not cross-process compare-and-swap. External editor buffers need
reloading when another tool has changed the file. A source rename requires
reopening its new path. Restarting the service revokes URLs and write grants;
open the document again through the command.

Edits identify a native named definition by its label, storage location and
content digest. A conflicting change to that definition pauses autosave; an
unrelated source change refreshes the page before retrying. Duplicate or
unresolvable definitions cannot be edited by guessing. Supported definitions use
`[fn:name]` at the beginning of an Org definition or `[^name]:` in Markdown,
including their native continuation paragraphs. Inline anonymous definitions
remain readable. Original definitions in read-only files cannot be changed;
existing sidecar footnotes can be edited in their sidecar.

Confirmed deletion uses those same checks and atomic replacement. It also removes
the selected note's reference tokens, an otherwise empty generated `Annotations:`
line, or its generated sidecar context line. Other definitions and literal code
examples are preserved. A retry after a lost acknowledgement succeeds without
another change when the definition and nonliteral references are already absent.

## Native footnote storage

Org uses `[fn:tadg-001]` references and named definitions. Markdown uses
`[^tadg-001]` references and `[^tadg-001]:` definitions. New comments contain current
text and readable author/creation attribution. No hidden COMMENT block or
persistent autosave history is written. Existing notes may have no attribution.
Creation and editing use native footnote markup. Text that would break out of
the definition is refused with the draft retained; ordinary authored code blocks
are not decoded or rewritten as tool-specific content.

New attribution uses an inactive Org timestamp in the service's local time, for
example `Author: Taḋg; Created: [2026-09-14 Mon 03:12]`. Date-only timestamps such
as `[2026-09-14 Mon]` are also recognized. The sidebar retains that native format;
older ISO dates are displayed in Org format too. Each saved edit replaces a
matching final non-empty `Author:` line ending in an Org timestamp with
`Author: CURRENT USER; Edited: [YYYY-MM-DD Day HH:MM]`. Both Created and Edited lines are recognized. Editing an older managed
note also replaces its redundant hidden metadata with the native representation.

This supersedes the earlier hidden UUID/state/operation block. The service keeps
only bounded, transient creation-session receipts for retries. The native label
and definition digest identify saved notes after reopening; browser memory is
not required. A service restart ends active creation sessions: reopen the saved
footnote rather than retrying the obsolete creation request.

Sidecars use the complete filename: `notes.org-annotations.org` or
`notes.md-annotations.org`. They contain Org footnotes and readable point context.
New sidecars are private, mode 0600. An unrecognized existing sidecar is never
overwritten. A read-only source receives no physical marker; HTML Preview adds
verified virtual references. Other readers require the separate sidecar.

When source permissions change, an existing composer keeps its selected store;
a new composer may use the writable source. Both stores are read together and
identified by their native labels and storage locations. Saving migrates valid legacy records
only in the destination being written, retaining latest text, author, timestamps
and identity. Reading alone never migrates. Invalid legacy records block writing.
Unresolved legacy comments remain visible as unplaced notes. Older preview pages
cannot append history through the retired v1 write route; reload them.

Comments, names, timestamps and stored point context travel with shared files.
A display name is attribution, not authentication. Restart-scoped URLs and
separate document write grants retain the service's loopback, permitted-root and
filesystem restrictions. No general editor, upload endpoint or remote service is
introduced.

Each source and sidecar is limited to 10 MiB. Combined annotation data is limited
to 10 MiB and 10,000 current annotations, with a 64 KiB per-record limit.
Previous draft versions no longer consume those quotas. The service permits
64 composer slots and 256 document write grants. Closing makes its slot reusable;
restart releases grants; reopening the document reloads its editable footnotes.

## Verification

`make test` runs Go protocol, filesystem and rendering regressions. Generated
HTML is checked with `htmltest`, `htmlq` and `tidy`; JavaScript with `oxlint`,
and CSS with native `biome`. These checks do not execute browser interactions.
Current keyboard, responsive layout, save feedback and accessibility checks use
paired human validation. The historical browser-fixture runner is not used for
this iteration. Linux/WSL qualification remains pending.

## Live-update connection

Hidden tabs close their event connection; returning to a tab refreshes state and
reconnects. The stream sends a notification on every connection so an edit made
while disconnected is not missed. A 30-second heartbeat keeps the stream alive
without reading the document. Native EventSource reconnects after a connection
failure, with a one-second retry interval. A brief interruption shows amber
feedback in reserved space. If it lasts 15 seconds, a recovery dialog appears.
Annotation requests have a **15-second deadline** in the browser and server.
After two seconds awaiting a response, amber feedback and “Waiting for service…”
appear in reserved space without blocking typing. A request timeout retains the
draft and reports the elapsed limit instead of a generic abort message. Actual
save rejections still use the recovery controls.

Successful connection recovery clears a connection-only dialog; it never clears
a confirmed save failure. This replaces the immediate inline Reconnect button.
Browsers without
EventSource require manual refresh. There is no polling fallback.

Save-time source revision checks still prevent conflicting writes. Filesystems
without usable native change notifications require manual refresh; Linux/WSL
filesystem behaviour remains a platform user test. The service admits at most
32 simultaneous event streams. Changes are coalesced over 100 milliseconds.

Section heading text does not fold sections in either reading or annotation
mode. Use the vertical bar/indicator or Show more button. Ordinary heading text accepts annotations. TODO/DONE markers, tags and priority
badges are excluded. Links select the containing block, preserving their label
and destination. Ambiguous or unsupported source mappings are refused
without changing the document.

While an annotation is being edited, its saved sidebar card is hidden to avoid
showing the same text twice. Other footnotes retain their original numbers.
Closing the editor restores the saved card; printing includes all saved notes.
Multiple paragraphs remain supported within one footnote.

While typing, autosave continues independently of document and sidebar refresh.
Save acknowledgements update the reserved save indicator and acknowledged
revisions; they do not immediately request another rendered page. Each keystroke
or input restarts a **15-second quiet period**. When it expires, the latest
pending update is applied after any save or input-method composition finishes.
The editor remains open, preserving the draft, caret and scroll positions.
Leaving the editor saves pending text and refreshes immediately after closing.
If typing resumes during a refresh request, applying it is deferred again.

This supersedes the earlier rule that focus alone suppressed rendering
indefinitely. The 15-second interval affects display updates only. Server checks
remain active on every save. An actual stale-write response may fetch current
state to reconcile an unchanged target without updating the display. Conflicting
edits to the same note retain the draft; no automatic document merge or overwrite
is introduced. Deferred preview errors do not interrupt typing with a modal;
confirmed save failures retain the recovery dialog.

## Document changes

- 19 September 2026: Fifteen-second request deadlines and connection recovery
  grace supersede the earlier five-second deadlines and two-second grace.
- 17 September 2026: Reserved feedback space, connection grace period, modal
  recovery and expandable long sidebar notes.
- Version 2: Batched follow-up saves and independent refresh after 15 seconds
  of inactivity or editor exit.

- Version 3: Verified block targets, block highlighting, annotation-mode copy
  suppression and separate references after code blocks and Markdown headings.

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

1. Activate **Annotations**, then click an insertion point in ordinary prose.
2. Type in the focused comment box. **Auto saved** means the latest value was acknowledged.
3. Optionally change the subdued **Footnote ID** below the comment box.
4. Activate **Finish comment** (the check mark), or press Escape, to save and close.
   The cross closes the comment and leaves annotation mode.

Keyboard placement uses Tab to reach a prose block, Enter to begin placing the
caret, Left/Right or Home/End to move it, then Enter to open the composer.
Escape cancels caret placement. Links, code-copy controls and folding controls
retain their own actions.

Comments are read-only after closing. Corrections become new comments.
Reopening a page exposes saved drafts as read-only recovered comments.
The ID defaults to a normalized display name and the next unused counter:
`Taḋg` produces `tadg-001`; `Tadhg O'Brien` produces `tadhg-o-brien-001`.
Ordinary footnotes also reserve their labels. IDs accept 1--64 ASCII letters,
digits, underscores or hyphens, beginning with a letter. A conflicting ID pauses
saving and remains editable; it is never silently renamed.

Autosave waits 300 milliseconds after typing pauses and submits within two
seconds during continuous typing. Input-method composition suspends submission.
Comments accept up to 4,000 Unicode characters and 16 KiB. Opening an empty
composer writes nothing. Clearing an active saved draft removes its owned
reference and definition. **Restore last autosave** restores the acknowledged
text and ID.

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
embedded footnotes and their metadata. It disables annotation and retains the
info bar. Sidecar content is not substituted for source text. TXT, source-code
wrappers, HTML and binary documents do not offer this view.

## Source changes and failures

Visible annotation-enabled pages check for changes once per second. Refresh
preserves the active composer. A saved embedded reference identifies its source
point; sidecar points use unique before/after context. Missing or ambiguous
locations remain explicitly unplaced in the endnotes. There is no fuzzy matching
or automatic movement to a nearby paragraph.

**Not saved** retains the draft and offers Retry and Copy draft. Temporary
failures retry the same operation after one, two and four seconds, with a small
jitter; persistent failures require an explicit retry. Closing, changing points,
leaving annotation mode and application navigation wait for acknowledged saves.
Browser navigation warnings are best effort: terminating the process can still
lose text that was never acknowledged.

Saves patch only owned reference tokens and definitions in a fresh source
snapshot. A private sibling temporary file is flushed and atomically renamed,
then its directory is synchronized before success. Owner and mode are preserved;
unsupported ACLs, extended attributes or flags prevent replacement rather than
being silently removed. File identity, source content and permissions are
rechecked before publication. Uncertain acknowledgement retains retry recovery.

An unrelated editor can still save an old buffer over newer comments. Filesystem
rename is not cross-process compare-and-swap. External editor buffers need
reloading when another tool has changed the file. A source rename requires
reopening its new path. Restarting the service revokes URLs and write grants;
open the document again through the command.

## Storage and ownership

Org uses `[fn:tadg-001]` references and named definitions. Markdown uses
`[^tadg-001]` references and `[^tadg-001]:` definitions. Each owned definition
contains readable current text, visible author/creation attribution, and one
hidden native comment block carrying its stable UUID, editable label association,
state and latest retry identity. There is no persisted autosave history. Comment
text that resembles markup is encoded as literal text for native export.

Sidecars use the complete filename: `notes.org-annotations.org` or
`notes.md-annotations.org`. They contain Org footnotes and readable point context.
New sidecars are private, mode 0600. An unrecognized existing sidecar is never
overwritten. A read-only source receives no physical marker; HTML Preview adds
verified virtual references. Other readers require the separate sidecar.

When source permissions change, an existing composer keeps its selected store;
a new composer may use the writable source. Both stores are read together and
reconciled by annotation UUID and revision. Saving migrates valid legacy records
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
64 active composers and 256 document write grants. Closing releases a composer;
restart releases grants and leaves saved drafts read-only.

## Verification

`make test` runs Go protocol, filesystem and rendering regressions. Generated
HTML is checked with `htmltest`, `htmlq` and `tidy`; JavaScript with `oxlint`,
and CSS with native `biome`. These checks do not execute browser interactions.
Current keyboard, responsive layout, save feedback and accessibility checks use
paired human validation. The historical browser-fixture runner is not used for
this iteration. Linux/WSL qualification remains pending.

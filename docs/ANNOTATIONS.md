# Document annotations

Service previews of genuine Org and Markdown files offer an **Annotations**
button beside the outline controls. Comments autosave into the source file.
When the source is read-only, comments use an adjacent Org sidecar instead.
File previews, code, plain text, binary documents and native HTML remain
reading-only. Reading a page never creates annotation records.

This is the W007 delivery candidate. Automated evidence and pending browser,
accessibility and filesystem qualification are recorded in
[annotation validation](../specs/007-service-annotations/validation.org).

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

1. Select a passage, or leave the selection empty for a whole-document comment.
2. Activate **Annotations**, then **Add comment**. The composer shows its target.
3. Type the comment. **Autosaved draft** means the latest text was acknowledged.
4. Activate **Close comment**, or press Escape, to flush and freeze the comment.

Comments have no edit or delete action after closing. Corrections become new
comments. Reopening a page exposes the latest saved draft as a read-only recovered
comment. Autosave appends successive events rather than rewriting earlier text.
The first acknowledged draft therefore survives loss of the browser session.

Autosave waits 300 milliseconds after typing pauses and submits within two
seconds during continuous typing. Input-method composition suspends submission.
Comments accept up to 4,000 Unicode characters and 16 KiB. An empty edit does
not erase a previous autosave; **Restore last autosave** restores that text.

## Source changes and failures

Visible pages check for changes once per second. Authored source changes refresh
the preview without replacing its active composer. Passage comments reattach
only when the exact quote and available context identify one match. Missing or
ambiguous passages keep their original excerpt and pause new saves. There is no
fuzzy matching or automatic retargeting to different words.

**Not saved** retains the draft and offers Retry and Copy draft. Temporary
failures retry the same event after one, two and four seconds, with a small
jitter; persistent failures require an explicit retry. Close and application
navigation wait for successful saving. Browser navigation warnings are best
effort: terminating the process can still lose text that was never acknowledged.

The service checks source bytes, file identity and permissions before appending.
A replaced file or incomplete annotation tail prevents further writes. Recovery
must preserve the current file and valid earlier records; the application never
truncates a damaged tail or rolls back an interrupted append. A source rename
requires reopening its new path. Restarting the service revokes browser URLs and
write grants; open the document again through the command.

An unrelated editor can save an old buffer over comments appended since it
opened the file. Autosave and refresh do not prevent that external overwrite.
Reload external editor buffers before saving when other tools have changed the
file. Advisory locks coordinate cooperating writers, not arbitrary editors.

## Storage and ownership

The source prefix, including its byte order mark, line endings and permissions,
is preserved. Versioned comment frames are appended at the end. Org uses comment
blocks; Markdown uses HTML comments. Structured records are hidden from preview
content, while literal examples of those records remain literal.

Sidecars use the complete filename: `notes.org-annotations.org` or
`notes.md-annotations.org`. New sidecars are private, mode 0600. An existing
unrecognized sidecar is never overwritten. When source permissions change,
existing composers keep their chosen store; a new composer can use the now
writable source. Both stores are read together without moving history.

Comments, names, timestamps, selected excerpts and superseded draft text travel
with shared or committed files. A display name is a label, not authentication.
Restart-scoped read URLs and separate document write grants remain subject to
the service's loopback, permitted-root and filesystem restrictions. There is no
general editor, upload endpoint or remote annotation service.

Each source and sidecar is limited to 10 MiB. Their combined annotation history
is limited to 10 MiB and 10,000 events. History is never compacted automatically.
The service permits 64 active composers and 256 document write grants; excess
capacity produces a visible refusal. Closing a composer releases its slot;
service restart releases grants and makes saved drafts read-only.

Printing includes an appendix of the latest acknowledged comments with author,
date, excerpt and unresolved status. Pending text and controls are excluded.

## Verification

`make test` runs Go protocol, filesystem and rendering regressions. The separate
`make test-browser` command opens the system default browser against a temporary
loopback fixture and runs the packaged JavaScript with controlled clocks and
transport responses. It requires a working browser and fails if assertions do
not return within 120 seconds. It adds no JavaScript command-line runtime.

A result proves only the engine actually used. Native Safari, Firefox, Edge,
VoiceOver and NVDA qualification remains distinct from these automated checks.

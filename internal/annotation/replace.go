// ABOUTME: Saves the active annotation as one current native footnote value.
// ABOUTME: Serializes composer writes and atomically replaces a freshly checked source.
package annotation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
)

type Replacement struct {
	Receipt                  Receipt
	PreviousInfo, SourceInfo os.FileInfo
	Retry                    bool
}

type PointVerifier func(context.Context, Snapshot, *Target) (int, error)

// Keep only the current creation request and acknowledgement in a bounded slot.
// Native definitions carry no operation history or hidden session metadata.
type currentSave struct {
	request            Request
	event              Event
	definitionRevision string
}

func validateCurrentRequest(r Request) error {
	if r.Action == "edit" {
		return validateFootnoteEdit(r)
	}
	if len(r.Text) > 16384 || utf8.RuneCountInString(r.Text) > 4000 || len(r.Target.Run) > 8192 || utf8.RuneCountInString(r.Target.Prefix) > 64 || utf8.RuneCountInString(r.Target.Suffix) > 64 || len(r.Target.HeadingID) > 4096 {
		return fail("body_limit")
	}
	if !ValidLabel(r.Label) {
		return fail("invalid_label")
	}
	if !ValidID(r.OperationID) || !ValidID(r.AnnotationID) || !ValidID(r.ComposerID) || r.Sequence < 1 || (r.Action != "upsert" && r.Action != "close") || r.Kind != "" || (r.Text != "" && !ValidText(r.Text)) {
		return fail("invalid_event")
	}
	if len(r.SourceRevision) != 64 || len(r.BodyRevision) != 64 || len(r.Revision) != 64 {
		return fail("invalid_event")
	}
	for _, revision := range []string{r.SourceRevision, r.BodyRevision, r.Revision} {
		if _, err := hex.DecodeString(revision); err != nil {
			return fail("invalid_event")
		}
	}
	for _, value := range []string{r.Target.Run, r.Target.Prefix, r.Target.Suffix, r.Target.HeadingID} {
		if !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
			return fail("invalid_event")
		}
	}
	if r.Target.Type != "point" || len(r.Target.BlockID) > 100 || len(r.Target.Run) > 8192 || r.Target.Position < 0 || r.Target.RunOffset < 0 {
		return fail("point_unmappable")
	}
	return nil
}

func (r Request) ValidateCurrent() error { return validateCurrentRequest(r) }

func (w *Writer) Replace(ctx context.Context, loc Location, expected os.FileInfo, author, secret string, r Request, verify PointVerifier) (Replacement, error) {
	if err := validateCurrentRequest(r); err != nil {
		return Replacement{}, err
	}
	// Native definitions use terminal line breaks as separators, not note text.
	// Normalize after validation so whitespace-only or oversized requests cannot
	// become valid clears, and use the same value for saves, retries and closes.
	r.Text = strings.TrimRight(r.Text, "\r\n")
	if r.Action == "edit" {
		return w.editFootnote(ctx, loc, expected, author, r)
	}
	incoming := r
	if author == "" || ValidateName(author) != nil {
		return Replacement{}, fail("invalid_event")
	}
	if err := w.begin(loc.Path); err != nil {
		return Replacement{}, err
	}
	defer w.end(loc.Path)
	snap, err := Read(loc)
	if err != nil {
		return Replacement{}, err
	}
	if expected == nil || !os.SameFile(expected, snap.SourceInfo) {
		return Replacement{}, fail("source_replaced")
	}
	if snap.Reason != "" {
		return Replacement{}, fail(snap.Reason)
	}
	key := loc.Path + "\x00" + r.ComposerID
	w.mu.Lock()
	owner, active := w.composers[key]
	w.mu.Unlock()
	if active && (owner.secret != sha256.Sum256([]byte(secret)) || owner.author != author || owner.annotationID != r.AnnotationID || !os.SameFile(owner.info, snap.SourceInfo)) {
		return Replacement{}, fail("composer_conflict")
	}
	var previous *Event
	for i := range snap.Events {
		if snap.Events[i].AnnotationID == r.AnnotationID {
			previous = &snap.Events[i]
		}
	}
	missing := previous == nil
	if active && owner.current != nil {
		previous = &owner.current.event
		missing = previous.Text == ""
		if previous.OperationID == r.OperationID && owner.current.request != r {
			return Replacement{}, fail("operation_conflict")
		}
	}
	edited := false
	if active && owner.current != nil && previous.Text != "" {
		data, format := snap.RawSource, loc.Format
		if owner.storage == "sidecar" {
			data, format = snap.RawSidecar, "org"
		}
		found := false
		for _, note := range EditableFootnotes(data, format, owner.storage) {
			if note.Label == previous.Label {
				found = note.Revision == owner.current.definitionRevision
				edited = strings.Contains(note.attribution, "; Edited: ")
				break
			}
		}
		if !found {
			return Replacement{}, fail("footnote_conflict")
		}
	}
	if previous != nil && previous.OperationID == r.OperationID {

		kind := "draft"
		if r.Action == "close" {
			kind = "close"
		}
		if previous.Text != r.Text || previous.Label != r.Label || previous.Sequence != r.Sequence || previous.Author != author || previous.Kind != kind {
			return Replacement{}, fail("operation_conflict")
		}
		storage := currentStorage(snap, r.AnnotationID)
		if active {
			storage = owner.storage
		}
		if err := w.syncReplacement(ctx, loc, snap, storage); err != nil {
			return Replacement{}, err
		}
		return Replacement{Receipt: receipt(snap, *previous, r.BodyRevision, storage), PreviousInfo: snap.SourceInfo, SourceInfo: snap.SourceInfo, Retry: true}, nil
	}
	if previous != nil && (!active || previous.Kind == "close") {
		return Replacement{}, fail("closed_comment")
	}
	if r.SourceRevision != snap.SourceRevision {
		return Replacement{}, fail("stale_source")
	}
	if previous == nil && r.Text == "" {
		return Replacement{}, fail("invalid_event")
	}
	label := strings.ToLower(r.Label)
	if (snap.Embedded.Labels[label] || snap.Sidecar.Labels[label]) && (previous == nil || !strings.EqualFold(previous.Label, r.Label)) {
		return Replacement{}, fail("label_conflict")
	}
	if previous == nil && (r.Sequence != 1 || r.Action != "upsert") || previous != nil && r.Sequence != previous.Sequence+1 {
		return Replacement{}, fail("sequence_conflict")
	}
	storage, err := Destination(loc, snap)
	if err != nil {
		return Replacement{}, err
	}
	if active {
		storage = owner.storage
	}
	position := -1
	if missing && r.Text != "" {
		if r.Target.BodyRevision != "" && r.Target.BodyRevision != r.BodyRevision {
			return Replacement{}, fail("stale_body")
		}
		if verify == nil {
			return Replacement{}, fail("point_unmappable")
		}
		position, err = verify(ctx, snap, &r.Target)
		if err != nil {
			return Replacement{}, err
		}
	}
	header := snap.Header
	if header == nil {
		id, err := NewID()
		if err != nil {
			return Replacement{}, err
		}
		header = &Header{1, id, loc.Format}
	}
	now := w.now().UTC().Format(time.RFC3339Nano)
	event := Event{Schema: 2, OperationID: r.OperationID, AnnotationID: r.AnnotationID, Sequence: r.Sequence, Kind: "draft", Author: author, CreatedAt: now, RecordedAt: now, Target: r.Target, Text: r.Text, Label: r.Label}
	if previous != nil {
		if previous.Text == r.Text {
			event.CreatedAt = previous.CreatedAt
		} else if !missing {
			edited = true
		}
		if !missing {
			event.Target = previous.Target
		}
	}
	if r.Action == "close" {
		event.Kind = "close"
	}
	if previous != nil && r.Action == "close" && (previous.Text != r.Text || previous.Label != r.Label) {
		return Replacement{}, fail("sequence_conflict")
	}
	data, format := snap.RawSource, loc.Format
	if storage == "sidecar" {
		data, format = snap.RawSidecar, "org"
	}
	if active && owner.current != nil {
		data, err = restoreCurrentFrame(data, format, *header, owner.current)
		if err != nil {
			return Replacement{}, err
		}
	}
	data, position, err = migrateCurrent(ctx, snap, data, format, r.Label, position, storage == "sidecar", verify)
	if err != nil {
		return Replacement{}, err
	}
	data, err = updateFootnote(data, format, *header, event, r.Label, position, storage == "sidecar")
	if err != nil {
		return Replacement{}, err
	}
	editedLabel := ""
	if edited {
		editedLabel = r.Label
	}
	data, err = readableFootnotes(data, format, editedLabel)
	if err != nil {
		return Replacement{}, err
	}
	candidate := snap
	if storage == "embedded" {
		candidate.RawSource = data
		candidate.Embedded = Parse(data, format)
	} else {
		candidate.RawSidecar = data
		candidate.SideExists = true
		candidate.Sidecar = Parse(data, format)
		if candidate.Sidecar.Header != nil {
			candidate.Sidecar.Header.SourceFormat = loc.Format
		}
	}
	validateSnapshot(&candidate, loc.Format)
	if candidate.Reason != "" {
		return Replacement{}, fail(candidate.Reason)
	}
	if !active {
		w.mu.Lock()
		if len(w.composers) >= 64 {
			for oldKey, old := range w.composers {
				if old.current != nil && old.current.event.Kind == "close" {
					delete(w.composers, oldKey)
					break
				}
			}
		}
		if len(w.composers) >= 64 {
			w.mu.Unlock()
			return Replacement{}, fail("composer_capacity")
		}
		w.composers[key] = composer{secret: sha256.Sum256([]byte(secret)), storage: storage, author: author, annotationID: r.AnnotationID, info: snap.SourceInfo}
		w.mu.Unlock()
	}
	updated, err := w.replaceFile(ctx, loc, snap, storage, data)
	if err != nil {
		// Rename may have succeeded before a directory synchronization failed.
		// Retain only a byte-for-byte verified current value and its new inode;
		// an identical retry must synchronize it before acknowledging it.
		uncertain, readErr := Read(loc)
		written := uncertain.RawSource
		if storage == "sidecar" {
			written = uncertain.RawSidecar
		}
		if readErr != nil || uncertain.Reason != "" || !bytes.Equal(written, data) {
			if !active {
				w.mu.Lock()
				delete(w.composers, key)
				w.mu.Unlock()
			}
			return Replacement{}, err
		}
		updated = uncertain
	}
	w.rememberReplacement(loc.Path, snap.SourceInfo, updated.SourceInfo)
	w.mu.Lock()
	item := w.composers[key]
	item.current = &currentSave{request: incoming, event: event}
	for _, note := range EditableFootnotes(data, format, storage) {
		if note.Label == r.Label {
			item.current.definitionRevision = note.Revision
		}
	}
	w.composers[key] = item
	w.mu.Unlock()
	return Replacement{Receipt: receipt(updated, event, r.BodyRevision, storage), PreviousInfo: snap.SourceInfo, SourceInfo: updated.SourceInfo}, err
}

func (w *Writer) syncReplacement(ctx context.Context, loc Location, snap Snapshot, storage string) (err error) {
	f, root, _, err := openDestination(loc, storage, false)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, f.Close(), root.Close()) }()
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return fail("busy")
	}
	defer func() { err = errors.Join(err, syscall.Flock(int(f.Fd()), syscall.LOCK_UN)) }()
	if err = destinationIdentity(loc, storage, root, f, snap); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = w.operations.Sync(f); err != nil {
		return err
	}
	rel, err := loc.relative()
	if err != nil {
		return err
	}
	parent, err := root.Open(filepath.Dir(rel))
	if err != nil {
		return err
	}
	if err = errors.Join(w.operations.Sync(parent), parent.Close()); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	current, err := Read(loc)
	if err != nil {
		return err
	}
	if current.Reason != "" || current.Revision != snap.Revision {
		return fail("store_changed")
	}
	return destinationIdentity(loc, storage, root, f, current)
}

func currentStorage(snap Snapshot, id string) string {
	for _, note := range snap.Embedded.Notes {
		if note.Event.AnnotationID == id {
			return "embedded"
		}
	}
	return "sidecar"
}

func (w *Writer) replaceFile(ctx context.Context, loc Location, snap Snapshot, storage string, data []byte) (updated Snapshot, err error) {
	if len(data) > MaxStore || storage == "embedded" && loc.Limit > 0 && int64(len(data)) > loc.Limit {
		return updated, fail("store_limit")
	}
	rel, err := loc.relative()
	if err != nil {
		return updated, err
	}
	info := snap.SourceInfo
	if storage == "sidecar" {
		rel += "-annotations.org"
		info = snap.SideInfo
	}
	root, err := os.OpenRoot(loc.Root)
	if err != nil {
		return updated, err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	var original *os.File
	var attributes fileAttributes
	if info != nil {
		var openErr error
		original, openErr = root.OpenFile(rel, os.O_WRONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if openErr != nil {
			return updated, openErr
		}
		defer func() { err = errors.Join(err, original.Close()) }()
		if err = destinationIdentity(loc, storage, root, original, snap); err != nil {
			return updated, err
		}
		if err = replacementMetadata(original); err != nil {
			return updated, err
		}
		attributes, err = readFileAttributes(original)
		if err != nil {
			return updated, err
		}
	}
	id, err := NewID()
	if err != nil {
		return updated, err
	}
	temp := filepath.Join(filepath.Dir(rel), ".htmlpreview-"+id)
	f, err := root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return updated, err
	}
	published := false
	defer func() {
		err = errors.Join(err, f.Close())
		if !published {
			err = errors.Join(err, root.Remove(temp))
		}
	}()
	if err = replacementMetadata(f); err != nil {
		return updated, err
	}
	n, err := w.operations.Append(f, data)
	if err != nil {
		return updated, err
	}
	if n != len(data) {
		return updated, io.ErrShortWrite
	}
	if info != nil {
		stat := info.Sys().(*syscall.Stat_t)
		current, statErr := f.Stat()
		if statErr != nil {
			return updated, statErr
		}
		fresh := current.Sys().(*syscall.Stat_t)
		if fresh.Uid != stat.Uid || fresh.Gid != stat.Gid {
			if err = f.Chown(int(stat.Uid), int(stat.Gid)); err != nil {
				return updated, err
			}
		}
		if err = f.Chmod(info.Mode()); err != nil {
			return updated, err
		}
	}
	if original != nil {
		if err = attributes.apply(f); err != nil {
			return updated, err
		}
	}
	if err = w.operations.Sync(f); err != nil {
		return updated, err
	}
	current, err := Read(loc)
	if err != nil {
		return updated, err
	}
	if current.Revision != snap.Revision || !os.SameFile(current.SourceInfo, snap.SourceInfo) || current.SourceInfo.Mode() != snap.SourceInfo.Mode() {
		return updated, fail("source_changed")
	}
	if !writableFile(current.SourceInfo) {
		return updated, fail("unsafe_source")
	}
	if snap.SideInfo != nil && (current.SideInfo == nil || !os.SameFile(snap.SideInfo, current.SideInfo) || snap.SideInfo.Mode() != current.SideInfo.Mode() || !writableFile(current.SideInfo)) {
		return updated, fail("store_changed")
	}
	if original != nil {
		if err = destinationIdentity(loc, storage, root, original, snap); err != nil {
			return updated, err
		}
		if err = replacementMetadata(original); err != nil {
			return updated, err
		}
		if err = attributes.verify(original); err != nil {
			return updated, err
		}
	}
	tempInfo, err := f.Stat()
	if err != nil {
		return updated, err
	}
	tempPath, err := root.Lstat(temp)
	if err != nil {
		return updated, err
	}
	if !writableFile(tempInfo) || !os.SameFile(tempInfo, tempPath) || tempPath.Mode()&os.ModeSymlink != 0 {
		return updated, fail("unsafe_source")
	}
	if err = replacementMetadata(f); err != nil {
		return updated, err
	}
	if original != nil {
		if err = attributes.verify(f); err != nil {
			return updated, err
		}
	}
	if err = ctx.Err(); err != nil {
		return updated, err
	}
	if err = root.Rename(temp, rel); err != nil {
		return updated, err
	}
	published = true
	parent, err := root.Open(filepath.Dir(rel))
	if err != nil {
		return updated, err
	}
	if err = errors.Join(w.operations.Sync(parent), parent.Close()); err != nil {
		return updated, err
	}
	if err = ctx.Err(); err != nil {
		return updated, err
	}
	updated, err = Read(loc)
	if err != nil {
		return updated, err
	}
	written := updated.RawSource
	if storage == "sidecar" {
		written = updated.RawSidecar
	}
	if updated.Reason != "" || !bytes.Equal(written, data) {
		return updated, fail("store_changed")
	}
	return updated, nil
}

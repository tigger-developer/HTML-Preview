// ABOUTME: Serializes bounded append-only annotation writes and composer ownership.
// ABOUTME: Revalidates identity, revisions and stores before synchronizing any acknowledgement.
package annotation

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

type FileOperations struct {
	Append func(*os.File, []byte) (int, error)
	Sync   func(*os.File) error
}

type composer struct {
	secret                        [32]byte
	storage, author, annotationID string
	info                          os.FileInfo
}

type Writer struct {
	mu         sync.Mutex
	active     map[string]bool
	composers  map[string]composer
	operations FileOperations
	now        func() time.Time
}

func NewWriter(ops FileOperations) *Writer {
	if ops.Append == nil {
		ops.Append = func(f *os.File, b []byte) (int, error) { return f.Write(b) }
	}
	if ops.Sync == nil {
		ops.Sync = func(f *os.File) error { return f.Sync() }
	}
	return &Writer{active: make(map[string]bool), composers: make(map[string]composer), operations: ops, now: time.Now}
}

type Receipt struct {
	AnnotationID   string `json:"annotation_id"`
	Sequence       int    `json:"sequence"`
	Revision       string `json:"revision"`
	SourceRevision string `json:"source_revision"`
	BodyRevision   string `json:"body_revision"`
	StoredAt       string `json:"stored_at"`
	Storage        string `json:"storage"`
	Closed         bool   `json:"closed"`
}

func (w *Writer) begin(path string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.active[path] || len(w.active) >= 2 {
		return fail("busy")
	}
	w.active[path] = true
	return nil
}
func (w *Writer) end(path string) { w.mu.Lock(); delete(w.active, path); w.mu.Unlock() }

func (w *Writer) Append(ctx context.Context, loc Location, expected os.FileInfo, author, secret string, request Request) (Receipt, bool, error) {
	if request.Validate() != nil || author == "" || ValidateName(author) != nil {
		return Receipt{}, false, fail("invalid_event")
	}
	if err := w.begin(loc.Path); err != nil {
		return Receipt{}, false, err
	}
	defer w.end(loc.Path)
	if err := ctx.Err(); err != nil {
		return Receipt{}, false, err
	}
	snap, err := Read(loc)
	if err != nil {
		return Receipt{}, false, err
	}
	if expected == nil || !os.SameFile(expected, snap.SourceInfo) {
		return Receipt{}, false, fail("source_replaced")
	}
	if snap.Reason != "" {
		return Receipt{}, false, fail(snap.Reason)
	}
	key := loc.Path + "\x00" + request.ComposerID
	w.mu.Lock()
	session, active := w.composers[key]
	w.mu.Unlock()
	if active && (session.secret != sha256.Sum256([]byte(secret)) || session.author != author || session.annotationID != request.AnnotationID || !os.SameFile(session.info, snap.SourceInfo)) {
		return Receipt{}, false, fail("composer_conflict")
	}
	if event, exists := snap.Operations[request.OperationID]; exists {
		if !RequestMatches(request, event) || event.Author != author {
			return Receipt{}, false, fail("operation_conflict")
		}
		receipt, retry, err := w.retry(ctx, loc, snap, event, request.BodyRevision)
		if err == nil && event.Kind == "close" {
			w.mu.Lock()
			delete(w.composers, key)
			w.mu.Unlock()
		}
		return receipt, retry, err
	}
	if request.SourceRevision != snap.SourceRevision {
		return Receipt{}, false, fail("stale_source")
	}
	var previous *Event
	for i := range snap.Events {
		if snap.Events[i].AnnotationID == request.AnnotationID {
			previous = &snap.Events[i]
		}
	}
	if previous != nil && (!active || previous.Kind == "close") {
		return Receipt{}, false, fail("closed_comment")
	}
	storage := session.storage
	if !active {
		storage, err = Destination(loc, snap)
		if err != nil {
			return Receipt{}, false, err
		}
		w.mu.Lock()
		if len(w.composers) >= 64 {
			w.mu.Unlock()
			return Receipt{}, false, fail("composer_capacity")
		}
		w.composers[key] = composer{sha256.Sum256([]byte(secret)), storage, author, request.AnnotationID, snap.SourceInfo}
		w.mu.Unlock()
		// Failed initial writes do not retain an unacknowledged composer slot.
		defer func() {
			if !active {
				w.mu.Lock()
				delete(w.composers, key)
				w.mu.Unlock()
			}
		}()
	}
	now := w.now().UTC().Format(time.RFC3339Nano)
	event := Event{Schema: 1, OperationID: request.OperationID, AnnotationID: request.AnnotationID, ComposerID: request.ComposerID, Sequence: request.Sequence, Kind: request.Kind, Author: author, CreatedAt: now, RecordedAt: now, Target: request.Target, Text: request.Text}
	if previous != nil {
		event.CreatedAt = previous.CreatedAt
	}
	if ValidateNext(previous, event) != nil {
		return Receipt{}, false, fail("sequence_conflict")
	}
	header := snap.Header
	if header == nil {
		id, err := NewID()
		if err != nil {
			return Receipt{}, false, err
		}
		header = &Header{1, id, loc.Format}
	}
	updated, err := w.commit(ctx, loc, snap, storage, header, event)
	if err != nil {
		// A complete append may have failed synchronization. Retain its live
		// composer so an identical, synchronized retry can continue editing.
		if uncertain, readErr := Read(loc); readErr == nil && uncertain.Reason == "" {
			if stored, exists := uncertain.Operations[event.OperationID]; exists && stored == event {
				active = true
			}
		}
		return Receipt{}, false, err
	}
	active = true
	if event.Kind == "close" {
		w.mu.Lock()
		delete(w.composers, key)
		w.mu.Unlock()
	}
	return receipt(updated, event, request.BodyRevision, storage), false, nil
}

func receipt(s Snapshot, event Event, body, storage string) Receipt {
	return Receipt{event.AnnotationID, event.Sequence, s.Revision, s.SourceRevision, body, event.RecordedAt, storage, event.Kind == "close"}
}

func (w *Writer) retry(ctx context.Context, loc Location, snap Snapshot, event Event, body string) (result Receipt, retried bool, err error) {
	storage := "sidecar"
	for _, candidate := range snap.Embedded.Events {
		if candidate.OperationID == event.OperationID {
			storage = "embedded"
			break
		}
	}
	f, root, _, err := openDestination(loc, storage, false)
	if err != nil {
		return Receipt{}, false, err
	}
	defer func() { err = errors.Join(err, f.Close(), root.Close()) }()
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return Receipt{}, false, fail("busy")
	}
	defer func() { err = errors.Join(err, syscall.Flock(int(f.Fd()), syscall.LOCK_UN)) }()
	if err = destinationIdentity(loc, storage, root, f, snap); err != nil {
		return Receipt{}, false, err
	}
	if err = ctx.Err(); err != nil {
		return Receipt{}, false, err
	}
	if err = w.operations.Sync(f); err != nil {
		return Receipt{}, false, err
	}
	if storage == "sidecar" {
		rel, relErr := loc.relative()
		if relErr != nil {
			return Receipt{}, false, relErr
		}
		parent, openErr := root.Open(filepath.Dir(rel))
		if openErr != nil {
			return Receipt{}, false, openErr
		}
		if err = errors.Join(w.operations.Sync(parent), parent.Close()); err != nil {
			return Receipt{}, false, err
		}
	}
	current, err := Read(loc)
	if err != nil {
		return Receipt{}, false, err
	}
	if got, ok := current.Operations[event.OperationID]; !ok || got != event || current.Reason != "" {
		return Receipt{}, false, fail("store_changed")
	}
	if !os.SameFile(snap.SourceInfo, current.SourceInfo) {
		return Receipt{}, false, fail("source_replaced")
	}
	if err = destinationIdentity(loc, storage, root, f, current); err != nil {
		return Receipt{}, false, err
	}
	return receipt(current, event, body, storage), true, nil
}

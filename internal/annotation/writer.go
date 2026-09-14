// ABOUTME: Serializes bounded current annotation writes and composer ownership.
// ABOUTME: Revalidates identity, revisions and stores before synchronizing any acknowledgement.
package annotation

import (
	"os"
	"strings"
	"sync"
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
	current                       *currentSave
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
	FootnoteRevision string `json:"footnote_revision,omitempty"`
	AnnotationID     string `json:"annotation_id"`
	Sequence         int    `json:"sequence"`
	Revision         string `json:"revision"`
	SourceRevision   string `json:"source_revision"`
	BodyRevision     string `json:"body_revision"`
	StoredAt         string `json:"stored_at"`
	Storage          string `json:"storage"`
	Closed           bool   `json:"closed"`
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

func receipt(s Snapshot, event Event, body, storage string) Receipt {
	return Receipt{"", event.AnnotationID, event.Sequence, s.Revision, s.SourceRevision, body, event.RecordedAt, storage, event.Kind == "close"}
}

// Both creation and generic edits replace the source inode. Keep other live
// composers on that document attached to the verified replacement as well.
func (w *Writer) rememberReplacement(path string, before, after os.FileInfo) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for name, item := range w.composers {
		if strings.HasPrefix(name, path+"\x00") && os.SameFile(item.info, before) {
			item.info = after
			w.composers[name] = item
		}
	}
}

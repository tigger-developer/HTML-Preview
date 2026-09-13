// ABOUTME: Exercises storage transitions and uncertain writes on real temporary files.
// ABOUTME: Injects faults only at the owned append and synchronization boundary.
package annotation

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func sourceFixture(t *testing.T, name string) (Location, Snapshot) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	loc := Location{Root: root, Path: filepath.Join(root, name), Format: "org", Limit: MaxStore}
	if err := os.WriteFile(loc.Path, []byte("* Source\nPreserved prose.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	snap, err := Read(loc)
	if err != nil {
		t.Fatal(err)
	}
	return loc, snap
}

func requestFixture(t *testing.T, snap Snapshot) Request {
	t.Helper()
	id := func() string {
		value, err := NewID()
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	return Request{OperationID: id(), AnnotationID: id(), ComposerID: id(), Sequence: 1, Revision: snap.Revision, SourceRevision: snap.SourceRevision, BodyRevision: Digest([]byte("Source Preserved prose.")), Kind: "draft", Target: Target{Type: "document"}, Text: "A draft"}
}

func TestRT007_4_ReadOnlySidecarAndPermissionTransition(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses ordinary file permission denial")
	}
	loc, initial := sourceFixture(t, "work.org")
	if err := os.Chmod(loc.Path, 0400); err != nil {
		t.Fatal(err)
	}
	w := NewWriter(FileOperations{})
	first := requestFixture(t, initial)
	saved, _, err := w.Append(t.Context(), loc, initial.SourceInfo, "Reviewer", "first", first)
	if err != nil || saved.Storage != "sidecar" {
		t.Fatalf("sidecar append: %+v %v", saved, err)
	}
	current, err := Read(loc)
	if err != nil || !bytes.Equal(initial.RawSource, current.RawSource) || len(current.Events) != 1 {
		t.Fatalf("sidecar/source: %v", err)
	}
	if current.SideInfo.Mode().Perm() != 0600 {
		t.Fatal("sidecar permissions")
	}
	if err := os.Chmod(loc.Path, 0600); err != nil {
		t.Fatal(err)
	}
	second := requestFixture(t, current)
	saved, _, err = w.Append(t.Context(), loc, current.SourceInfo, "Another reviewer", "second", second)
	if err != nil || saved.Storage != "embedded" {
		t.Fatalf("embedded transition: %+v %v", saved, err)
	}
	union, err := Read(loc)
	if err != nil || union.Reason != "" || len(union.Events) != 2 || union.Embedded.Header.DocumentID != union.Sidecar.Header.DocumentID {
		t.Fatalf("union: %+v %v", union, err)
	}
}

func TestRT007_6_ShortAppendRetainsEarlierEvents(t *testing.T) {
	loc, initial := sourceFixture(t, "work.org")
	w := NewWriter(FileOperations{})
	first := requestFixture(t, initial)
	if _, _, err := w.Append(t.Context(), loc, initial.SourceInfo, "Reviewer", "secret", first); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(loc.Path)
	if err != nil {
		t.Fatal(err)
	}
	w.operations.Append = func(f *os.File, b []byte) (int, error) { return f.Write(b[:len(b)/2]) }
	second := first
	second.Sequence = 2
	second.OperationID = "10000000-0000-4000-8000-000000000002"
	second.Text = "Updated"
	if _, _, err := w.Append(t.Context(), loc, initial.SourceInfo, "Reviewer", "secret", second); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("short append=%v", err)
	}
	current, err := Read(loc)
	if err != nil || current.Reason == "" || !bytes.HasPrefix(current.RawSource, before) || len(current.Events) != 1 {
		t.Fatalf("partial preservation=%+v %v", current, err)
	}
	if _, _, err := w.Append(t.Context(), loc, initial.SourceInfo, "Reviewer", "secret", second); err == nil {
		t.Fatal("appended after partial tail")
	}
}

func TestRT007_6_SyncFailureRetriesAndContinuesComposer(t *testing.T) {
	loc, initial := sourceFixture(t, "work.org")
	failSync := true
	w := NewWriter(FileOperations{Sync: func(f *os.File) error {
		if failSync {
			return errors.New("injected synchronization failure")
		}
		return f.Sync()
	}})
	first := requestFixture(t, initial)
	if _, _, err := w.Append(t.Context(), loc, initial.SourceInfo, "Reviewer", "secret", first); err == nil {
		t.Fatal("sync failure reported success")
	}
	failSync = false
	if _, retry, err := w.Append(t.Context(), loc, initial.SourceInfo, "Reviewer", "secret", first); err != nil || !retry {
		t.Fatalf("uncertain retry=%t %v", retry, err)
	}
	first.Sequence = 2
	first.OperationID = "10000000-0000-4000-8000-000000000002"
	first.Text = "Continuing after retry"
	if _, _, err := w.Append(t.Context(), loc, initial.SourceInfo, "Reviewer", "secret", first); err != nil {
		t.Fatalf("active composer lost after synchronization retry: %v", err)
	}
}

func TestRT007_5_RetrySecretAndRecoveredDraft(t *testing.T) {
	loc, initial := sourceFixture(t, "retry.org")
	w := NewWriter(FileOperations{})
	request := requestFixture(t, initial)
	if _, _, err := w.Append(t.Context(), loc, initial.SourceInfo, "Reviewer", "secret", request); err != nil {
		t.Fatal(err)
	}
	if _, _, err := w.Append(t.Context(), loc, initial.SourceInfo, "Reviewer", "other secret", request); err == nil {
		t.Fatal("active composer accepted a retry with another secret")
	}
	recovered := NewWriter(FileOperations{})
	if _, retry, err := recovered.Append(t.Context(), loc, initial.SourceInfo, "Reviewer", "secret", request); err != nil || !retry {
		t.Fatalf("recovered exact retry=%v %v", retry, err)
	}
	request.Sequence = 2
	request.OperationID = "10000000-0000-4000-8000-000000000002"
	if _, _, err := recovered.Append(t.Context(), loc, initial.SourceInfo, "Reviewer", "secret", request); err == nil {
		t.Fatal("recovered draft accepted a new event")
	}
}

func TestRT007_6_RetryHonoursFilesystemLock(t *testing.T) {
	loc, initial := sourceFixture(t, "lock.org")
	w := NewWriter(FileOperations{})
	request := requestFixture(t, initial)
	if _, _, err := w.Append(t.Context(), loc, initial.SourceInfo, "Reviewer", "secret", request); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(loc.Path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			t.Error(err)
		}
	}()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); err != nil {
			t.Error(err)
		}
	}()
	if _, _, err := w.Append(t.Context(), loc, initial.SourceInfo, "Reviewer", "secret", request); err == nil {
		t.Fatal("retry bypassed another writer's advisory lock")
	}
}

func TestRT007_6_CancelledSyncDoesNotAcknowledge(t *testing.T) {
	loc, initial := sourceFixture(t, "cancel.org")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	w := NewWriter(FileOperations{Sync: func(f *os.File) error { cancel(); return f.Sync() }})
	request := requestFixture(t, initial)
	if _, _, err := w.Append(ctx, loc, initial.SourceInfo, "Reviewer", "secret", request); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled operation acknowledged: %v", err)
	}
	w.operations.Sync = func(f *os.File) error { return f.Sync() }
	if _, retry, err := w.Append(t.Context(), loc, initial.SourceInfo, "Reviewer", "secret", request); err != nil || !retry {
		t.Fatalf("cancelled sync retry=%v %v", retry, err)
	}
}

func TestRT007_7_IdentityAliasesAndStaleSource(t *testing.T) {
	for _, change := range []string{"hard link", "symlink sidecar", "replacement", "edit", "foreign sidecar"} {
		t.Run(change, func(t *testing.T) {
			loc, initial := sourceFixture(t, "work.org")
			request := requestFixture(t, initial)
			switch change {
			case "hard link":
				if err := os.Link(loc.Path, filepath.Join(loc.Root, "alias.org")); err != nil {
					t.Fatal(err)
				}
			case "symlink sidecar":
				if err := os.Symlink(loc.Path, loc.Path+"-annotations.org"); err != nil {
					t.Fatal(err)
				}
			case "replacement":
				if err := os.Rename(loc.Path, loc.Path+".old"); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(loc.Path, initial.RawSource, 0600); err != nil {
					t.Fatal(err)
				}
			case "edit":
				if err := os.WriteFile(loc.Path, []byte("External replacement prose"), 0600); err != nil {
					t.Fatal(err)
				}
			case "foreign sidecar":
				if err := os.WriteFile(loc.Path+"-annotations.org", []byte("Personal document"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.ReadFile(loc.Path)
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := NewWriter(FileOperations{}).Append(t.Context(), loc, initial.SourceInfo, "Reviewer", "secret", request); err == nil {
				t.Fatal("unsafe mutation accepted")
			}
			after, err := os.ReadFile(loc.Path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("denied append changed source")
			}
		})
	}
}

func TestRT007_10_ComposerCapacityAndClosedRelease(t *testing.T) {
	loc, initial := sourceFixture(t, "work.org")
	w := NewWriter(FileOperations{})
	var first Request
	for i := 0; i < 64; i++ {
		request := requestFixture(t, initial)
		if i == 0 {
			first = request
		}
		if _, _, err := w.Append(context.Background(), loc, initial.SourceInfo, "Reviewer", "secret", request); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := w.Append(t.Context(), loc, initial.SourceInfo, "Reviewer", "secret", requestFixture(t, initial)); err == nil || !strings.Contains(err.Error(), "composer_capacity") {
		t.Fatalf("capacity=%v", err)
	}
	first.Sequence = 2
	first.Kind = "close"
	first.OperationID = "10000000-0000-4000-8000-000000000002"
	if _, _, err := w.Append(t.Context(), loc, initial.SourceInfo, "Reviewer", "secret", first); err != nil {
		t.Fatal(err)
	}
	if _, _, err := w.Append(t.Context(), loc, initial.SourceInfo, "Reviewer", "secret", requestFixture(t, initial)); err != nil {
		t.Fatal("closed composer did not release capacity")
	}
}

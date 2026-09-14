// ABOUTME: Exercises atomic current-value writes through the annotation writer.
// ABOUTME: Protects W009 draft replacement, identity, permission and failure semantics.
package annotation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRT009_7_AtomicCurrentValues(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "notes.org")
	original := "A sentence.\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	loc := Location{Root: root, Path: path, Format: "org"}
	snap, err := Read(loc)
	if err != nil {
		t.Fatal(err)
	}
	w := NewWriter(FileOperations{})
	r := Request{OperationID: "10000000-0000-4000-8000-000000000001", AnnotationID: "20000000-0000-4000-8000-000000000001", ComposerID: "30000000-0000-4000-8000-000000000001", Sequence: 1, Revision: snap.Revision, SourceRevision: snap.SourceRevision, BodyRevision: Digest([]byte("A sentence.")), Action: "upsert", Label: "tadg-001", Target: Target{Type: "point", Position: 10, Run: "A sentence.", RunOffset: 10}, Text: "First draft"}
	verify := func(context.Context, Snapshot, Target) (int, error) { return 10, nil }
	result, err := w.Replace(context.Background(), loc, snap.SourceInfo, "Taḋg", "secret", r, verify)
	if err != nil {
		t.Fatal(err)
	}
	r.OperationID = "10000000-0000-4000-8000-000000000002"
	r.Sequence, r.Text, r.Label = 2, "Current value", "tadg-002"
	result, err = w.Replace(context.Background(), loc, result.SourceInfo, "Taḋg", "secret", r, verify)
	if err != nil {
		t.Fatal(err)
	}
	current, err := Read(loc)
	if err != nil || current.Reason != "" || len(current.Events) != 1 || current.Events[0].Text != "Current value" {
		t.Fatalf("latest value not readable: %v %s", err, current.Reason)
	}
	if string(current.Embedded.Source) != original || current.SourceInfo.Mode().Perm() != 0600 {
		t.Fatal("authored bytes or mode changed")
	}
	_, err = w.Replace(context.Background(), loc, result.SourceInfo, "Taḋg", "secret", r, verify)
	if err != nil {
		t.Fatalf("idempotent retry failed: %v", err)
	}
	r.OperationID = "10000000-0000-4000-8000-000000000003"
	r.Sequence, r.Text = 3, ""
	result, err = w.Replace(context.Background(), loc, result.SourceInfo, "Taḋg", "secret", r, verify)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != original {
		t.Fatal("clearing did not restore authored bytes")
	}
	retry, err := w.Replace(context.Background(), loc, result.SourceInfo, "Taḋg", "secret", r, verify)
	if err != nil || !retry.Retry {
		t.Fatalf("clearing retry failed: %v", err)
	}
	r.OperationID = "10000000-0000-4000-8000-000000000004"
	r.Sequence, r.Text = 4, "Restored current comment"
	result, err = w.Replace(context.Background(), loc, result.SourceInfo, "Taḋg", "secret", r, verify)
	if err != nil {
		t.Fatalf("typing after clearing failed: %v", err)
	}
}

func TestRT009_7_FailedReplacementRetainsSource(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "notes.org")
	original := []byte("A sentence.\n")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	loc := Location{Root: root, Path: path, Format: "org"}
	snap, err := Read(loc)
	if err != nil {
		t.Fatal(err)
	}
	w := NewWriter(FileOperations{Append: func(*os.File, []byte) (int, error) { return 0, errors.New("injected write failure") }})
	r := Request{OperationID: "10000000-0000-4000-8000-000000000001", AnnotationID: "20000000-0000-4000-8000-000000000001", ComposerID: "30000000-0000-4000-8000-000000000001", Sequence: 1, Revision: snap.Revision, SourceRevision: snap.SourceRevision, BodyRevision: Digest([]byte("A sentence.")), Action: "upsert", Label: "tadg-001", Target: Target{Type: "point", Run: "A sentence.", Position: 10, RunOffset: 10}, Text: "Must not be partially saved"}
	_, err = w.Replace(context.Background(), loc, snap.SourceInfo, "Taḋg", "secret", r, func(context.Context, Snapshot, Target) (int, error) { return 10, nil })
	if err == nil {
		t.Fatal("injected write failure was acknowledged")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != string(original) {
		t.Fatal("partial write replaced the original")
	}
	if len(w.composers) != 0 {
		t.Fatal("failed first write consumed composer capacity")
	}
}

func TestRT009_7_RetrySynchronizesPublishedReplacement(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "notes.org")
	if err := os.WriteFile(path, []byte("A sentence.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	loc := Location{Root: root, Path: path, Format: "org"}
	snap, err := Read(loc)
	if err != nil {
		t.Fatal(err)
	}
	failed, syncs := false, 0
	w := NewWriter(FileOperations{Sync: func(f *os.File) error {
		info, err := f.Stat()
		if err != nil {
			return err
		}
		if info.IsDir() {
			syncs++
			if !failed {
				failed = true
				return errors.New("injected directory sync failure")
			}
		}
		return f.Sync()
	}})
	r := Request{OperationID: "10000000-0000-4000-8000-000000000001", AnnotationID: "20000000-0000-4000-8000-000000000001", ComposerID: "30000000-0000-4000-8000-000000000001", Sequence: 1, Revision: snap.Revision, SourceRevision: snap.SourceRevision, BodyRevision: Digest([]byte("A sentence.")), Action: "upsert", Label: "tadg-001", Target: Target{Type: "point", Position: 10, Run: "A sentence.", RunOffset: 10}, Text: "Retain this current value"}
	verify := func(context.Context, Snapshot, Target) (int, error) { return 10, nil }
	result, err := w.Replace(context.Background(), loc, snap.SourceInfo, "Taḋg", "secret", r, verify)
	if err == nil {
		t.Fatal("uncertain durability was acknowledged")
	}
	if result.SourceInfo == nil {
		t.Fatal("published identity was lost after sync failure")
	}
	result, err = w.Replace(context.Background(), loc, result.SourceInfo, "Taḋg", "secret", r, verify)
	if err != nil || !result.Retry || syncs != 2 {
		t.Fatalf("retry did not synchronize published value: %v, syncs=%d", err, syncs)
	}
}

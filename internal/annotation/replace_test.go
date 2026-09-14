// ABOUTME: Exercises atomic current-value writes through the annotation writer.
// ABOUTME: Protects W009 draft replacement, identity, permission and failure semantics.
package annotation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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
	verify := func(context.Context, Snapshot, *Target) (int, error) { return 10, nil }
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
	if err != nil || current.Reason != "" || len(EditableFootnotes(current.RawSource, "org", "embedded")) != 1 || EditableFootnotes(current.RawSource, "org", "embedded")[0].Text != "Current value" {
		t.Fatalf("latest value not readable: %v %s", err, current.Reason)
	}
	if string(FootnoteBodySource(current.RawSource, "org")) != original || current.SourceInfo.Mode().Perm() != 0600 {
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
	// #nosec G304 -- Reads only the synthetic source allocated in this test's temporary root.
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
	_, err = w.Replace(context.Background(), loc, snap.SourceInfo, "Taḋg", "secret", r, func(context.Context, Snapshot, *Target) (int, error) { return 10, nil })
	if err == nil {
		t.Fatal("injected write failure was acknowledged")
	}
	// #nosec G304 -- Reads only the synthetic source allocated in this test's temporary root.
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
	verify := func(context.Context, Snapshot, *Target) (int, error) { return 10, nil }
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

func TestRT009_6_CurrentSidecarPreservesReadOnlySource(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires native unprivileged permission checks")
	}
	root := t.TempDir()
	path := filepath.Join(root, "notes.md")
	if err := os.WriteFile(path, []byte("A sentence.\n"), 0400); err != nil {
		t.Fatal(err)
	}
	loc := Location{Root: root, Path: path, Format: "markdown"}
	snap, err := Read(loc)
	if err != nil {
		t.Fatal(err)
	}
	w := NewWriter(FileOperations{})
	r := Request{OperationID: "10000000-0000-4000-8000-000000000001", AnnotationID: "20000000-0000-4000-8000-000000000001", ComposerID: "30000000-0000-4000-8000-000000000001", Sequence: 1, Revision: snap.Revision, SourceRevision: snap.SourceRevision, BodyRevision: Digest([]byte("A sentence.")), Action: "upsert", Label: "tadg-001", Target: Target{Type: "point", Position: 10, Run: "A sentence.", RunOffset: 10, Prefix: "A sentence", Suffix: "."}, Text: "Sidecar comment"}
	result, err := w.Replace(context.Background(), loc, snap.SourceInfo, "Taḋg", "secret", r, func(context.Context, Snapshot, *Target) (int, error) { return 10, nil })
	if err != nil {
		t.Fatal(err)
	}
	current, err := Read(loc)
	if err != nil || current.Reason != "" || len(EditableFootnotes(current.RawSidecar, "org", "sidecar")) != 1 {
		t.Fatalf("sidecar not readable: %v %s", err, current.Reason)
	}
	if string(current.RawSource) != "A sentence.\n" || !os.SameFile(snap.SourceInfo, current.SourceInfo) {
		t.Fatal("read-only source changed")
	}
	if !strings.Contains(string(current.RawSidecar), "[fn:tadg-001]") || !strings.Contains(string(current.RawSidecar), "A sentence") {
		t.Fatal("sidecar lacks native reference and readable context")
	}
	r.OperationID = "10000000-0000-4000-8000-000000000002"
	r.Sequence = 2
	r.Text = ""
	_, err = w.Replace(context.Background(), loc, result.SourceInfo, "Taḋg", "secret", r, nil)
	if err != nil {
		t.Fatalf("clearing sidecar failed: %v", err)
	}
	current, err = Read(loc)
	if err != nil || current.Reason != "" || len(EditableFootnotes(current.RawSidecar, "org", "sidecar")) != 0 {
		t.Fatalf("cleared sidecar unavailable: %v %s", err, current.Reason)
	}
}

func TestRT009_7_ReplacementRechecksSourceAliasesBeforeRename(t *testing.T) {
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
	linked := false
	w := NewWriter(FileOperations{Sync: func(f *os.File) error {
		if !linked {
			if err := os.Link(path, filepath.Join(root, "alias.org")); err != nil {
				return err
			}
			linked = true
		}
		return f.Sync()
	}})
	r := Request{OperationID: "10000000-0000-4000-8000-000000000001", AnnotationID: "20000000-0000-4000-8000-000000000001", ComposerID: "30000000-0000-4000-8000-000000000001", Sequence: 1, Revision: snap.Revision, SourceRevision: snap.SourceRevision, BodyRevision: Digest([]byte("A sentence.")), Action: "upsert", Label: "tadg-001", Target: Target{Type: "point", Position: 10, Run: "A sentence.", RunOffset: 10}, Text: "Must not replace a newly aliased source"}
	_, err = w.Replace(t.Context(), loc, snap.SourceInfo, "Taḋg", "secret", r, func(context.Context, Snapshot, *Target) (int, error) { return 10, nil })
	if err == nil {
		t.Fatal("source alias introduced during staging was ignored")
	}
	// #nosec G304 -- Reads only the synthetic source allocated in this test's temporary root.
	data, readErr := os.ReadFile(path)
	if readErr != nil || string(data) != "A sentence.\n" {
		t.Fatal("unsafe replacement changed source", readErr)
	}
}

func TestRT009_7_NeighbouringEditRetainsOwnedReference(t *testing.T) {
	loc, initial := sourceFixture(t, "notes.org")
	w := NewWriter(FileOperations{})
	r := requestFixture(t, initial)
	r.Target.BodyRevision = r.BodyRevision
	if _, _, err := saveFixture(w, t.Context(), loc, &initial.SourceInfo, "Reviewer", "secret", r); err != nil {
		t.Fatal(err)
	}
	current, err := Read(loc)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(current.RawSource), "prose.", "prose has changed.", 1)
	if err := os.WriteFile(loc.Path, []byte(edited), 0600); err != nil {
		t.Fatal(err)
	}
	current, err = Read(loc)
	if err != nil {
		t.Fatal(err)
	}
	r.Sequence, r.OperationID = 2, "10000000-0000-4000-8000-000000000002"
	r.Revision, r.SourceRevision = current.Revision, current.SourceRevision
	r.BodyRevision = Digest([]byte("Source Preserved prose has changed."))
	r.Text = "Keep this current annotation at its existing marker"
	if _, _, err := saveFixture(w, t.Context(), loc, &initial.SourceInfo, "Reviewer", "secret", r); err != nil {
		t.Fatalf("owned reference was invalidated by neighbouring prose: %v", err)
	}
	current, err = Read(loc)
	if err != nil || len(EditableFootnotes(current.RawSource, "org", "embedded")) != 1 || !strings.Contains(string(current.RawSource), footnoteReference("org", r.Label)) || EditableFootnotes(current.RawSource, "org", "embedded")[0].Text != r.Text {
		t.Fatal("updated note lost its native reference", err)
	}
}

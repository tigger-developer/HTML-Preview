// ABOUTME: Exercises legacy import through the current annotation save boundary.
// ABOUTME: Protects latest values, attribution, destination isolation and authored bytes.
package annotation

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRT009_7_SaveMigratesOnlyCurrentLegacyValues(t *testing.T) {
	for _, format := range []string{"org", "markdown"} {
		t.Run(format, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "notes")
			original := []byte("A sentence.\r\n")
			header := Header{1, "40000000-0000-4000-8000-000000000001", format}
			legacy := fixtureEvent()
			legacy.Text = "Old draft"
			data := append([]byte(nil), original...)
			for i, text := range []string{"Old draft", "Current legacy comment"} {
				legacy.Sequence = i + 1
				legacy.OperationID = []string{"10000000-0000-4000-8000-000000000001", "10000000-0000-4000-8000-000000000002"}[i]
				legacy.Text = text
				addition, err := AppendBytes(Parse(data, format), &header, legacy, format)
				if err != nil {
					t.Fatal(err)
				}
				data = append(data, addition...)
			}
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			loc := Location{Root: root, Path: path, Format: format}
			snap, err := Read(loc)
			if err != nil {
				t.Fatal(err)
			}
			request := Request{OperationID: "50000000-0000-4000-8000-000000000001", AnnotationID: "60000000-0000-4000-8000-000000000001", ComposerID: "70000000-0000-4000-8000-000000000001", Sequence: 1, Revision: snap.Revision, SourceRevision: snap.SourceRevision, BodyRevision: Digest([]byte("A sentence.")), Action: "upsert", Label: "tadg-001", Target: Target{Type: "point", Position: 10, Run: "A sentence.", RunOffset: 10}, Text: "New comment"}
			_, err = NewWriter(FileOperations{}).Replace(context.Background(), loc, snap.SourceInfo, "Taḋg", "secret", request, func(context.Context, Snapshot, *Target) (int, error) { return 10, nil })
			if err != nil {
				t.Fatal(err)
			}
			current, err := Read(loc)
			if err != nil || current.Reason != "" {
				t.Fatalf("unreadable migration: %v %s", err, current.Reason)
			}
			if string(FootnoteBodySource(current.RawSource, format)) != string(original) || len(EditableFootnotes(current.RawSource, format, "embedded")) != 2 || strings.Contains(string(current.RawSource), headerMarker) || strings.Contains(string(current.RawSource), "Old draft") {
				t.Fatal("migration retained history or changed authored bytes")
			}
			for _, note := range EditableFootnotes(current.RawSource, format, "embedded") {
				if note.Label == "annotation-"+legacy.AnnotationID && (!strings.Contains(note.Text, legacy.Text) || note.Author != legacy.Author || note.CreatedAt != orgTimestamp(legacy.CreatedAt)) {
					t.Fatalf("legacy value or attribution changed: %+v", note)
				}
			}

		})
	}
}

func TestRT009_7_MigratedValueReconcilesWithUnwrittenLegacyStore(t *testing.T) {
	legacy := fixtureEvent()
	native := legacy
	native.Schema, native.ComposerID, native.Label = 2, "", "annotation-example"
	values, _, err := Project([]Event{legacy, native})
	if err != nil || len(values) != 1 || values[0].Schema != 2 {
		t.Fatalf("same migrated operation conflicts with retained legacy store: %v %+v", err, values)
	}
	native.Text = "Conflicting value"
	if _, _, err := Project([]Event{legacy, native}); err == nil {
		t.Fatal("equal-version conflicting values accepted")
	}
}

func TestRT009_7_ReadOnlyLegacySourceRemainsUnmodifiedWhenSavingSidecar(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires native unprivileged permission checks")
	}
	root := t.TempDir()
	path := filepath.Join(root, "notes.org")
	header := Header{1, "40000000-0000-4000-8000-000000000001", "org"}
	original := []byte("A sentence.\n")
	addition, err := AppendBytes(Parse(original, "org"), &header, fixtureEvent(), "org")
	if err != nil {
		t.Fatal(err)
	}
	original = append(original, addition...)
	if err := os.WriteFile(path, original, 0400); err != nil {
		t.Fatal(err)
	}
	loc := Location{Root: root, Path: path, Format: "org"}
	snap, err := Read(loc)
	if err != nil {
		t.Fatal(err)
	}
	request := Request{OperationID: "50000000-0000-4000-8000-000000000001", AnnotationID: "60000000-0000-4000-8000-000000000001", ComposerID: "70000000-0000-4000-8000-000000000001", Sequence: 1, Revision: snap.Revision, SourceRevision: snap.SourceRevision, BodyRevision: Digest([]byte("A sentence.")), Action: "upsert", Label: "tadg-001", Target: Target{Type: "point", Position: 10, Run: "A sentence.", RunOffset: 10}, Text: "New sidecar comment"}
	_, err = NewWriter(FileOperations{}).Replace(t.Context(), loc, snap.SourceInfo, "Taḋg", "secret", request, func(context.Context, Snapshot, *Target) (int, error) { return 10, nil })
	if err != nil {
		t.Fatal(err)
	}
	current, err := Read(loc)
	if err != nil || current.Reason != "" || string(current.RawSource) != string(original) || len(current.Events) != 1 || len(EditableFootnotes(current.RawSidecar, "org", "sidecar")) != 1 {
		t.Fatalf("save crossed destination boundary: %v %s", err, current.Reason)
	}
}

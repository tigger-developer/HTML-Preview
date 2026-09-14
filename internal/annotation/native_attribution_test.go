// ABOUTME: Checks native footnotes without persisted autosave metadata.
// ABOUTME: Covers current values, optional attribution and service-independent editing.
package annotation

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRT009_6_NoCommentMetadata(t *testing.T) {
	loc, snap := sourceFixture(t, "notes.org")
	w := NewWriter(FileOperations{})
	r := requestFixture(t, snap)
	result, err := w.Replace(t.Context(), loc, snap.SourceInfo, "Taḋg", "secret", r, func(context.Context, Snapshot, *Target) (int, error) { return 10, nil })
	if err != nil {
		t.Fatal(err)
	}
	current, err := Read(loc)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(current.RawSource), "#+BEGIN_COMMENT") || strings.Contains(string(current.RawSource), nativeMarker) {
		t.Fatal("saved native footnote contains hidden metadata")
	}
	notes := EditableFootnotes(current.RawSource, "org", "embedded")
	if len(notes) != 1 || notes[0].Text != r.Text {
		t.Fatalf("current footnote: %#v", notes)
	}
	if !strings.Contains(string(current.RawSource), "Author: Taḋg; Created: [") {
		t.Fatal("native attribution missing")
	}
	r.Sequence++
	r.OperationID = "10000000-0000-4000-8000-000000000009"
	r.Text = "Latest value"
	r.Revision, r.SourceRevision = current.Revision, current.SourceRevision
	result, err = w.Replace(t.Context(), loc, result.SourceInfo, "Taḋg", "secret", r, nil)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := w.Replace(t.Context(), loc, result.SourceInfo, "Taḋg", "secret", r, nil)
	if err != nil || !retry.Retry {
		t.Fatal("retry", err)
	}
	current, err = Read(loc)
	if err != nil {
		t.Fatal(err)
	}
	notes = EditableFootnotes(current.RawSource, "org", "embedded")
	if len(notes) != 1 || notes[0].Text != "Latest value" {
		t.Fatal("latest value missing")
	}
	if strings.Contains(string(current.RawSource), nativeMarker) {
		t.Fatal("metadata returned")
	}
	before := string(current.RawSource)
	current.RawSource = []byte(strings.Replace(before, "Latest value", "Disk edit", 1))
	if err := os.WriteFile(loc.Path, current.RawSource, 0600); err != nil {
		t.Fatal(err)
	}
	fresh, err := Read(loc)
	if err != nil {
		t.Fatal(err)
	}
	r.Sequence++
	r.OperationID = "10000000-0000-4000-8000-000000000010"
	r.Text = "Conflicting draft"
	r.Revision, r.SourceRevision = fresh.Revision, fresh.SourceRevision
	if _, err = w.Replace(t.Context(), loc, result.SourceInfo, "Taḋg", "secret", r, nil); err == nil {
		t.Fatal("competing footnote edit overwritten")
	}
}

func TestRT009_4_OptionalNativeAttribution(t *testing.T) {
	for _, format := range []string{"org", "markdown"} {
		for _, date := range []string{"", "[2026-09-14 Mon]", "[2026-09-14 Mon 03:12]"} {
			t.Run(format+date, func(t *testing.T) {
				loc, snap := sourceFixture(t, "note")
				loc.Format = format
				label, definition, indent := "[fn:ordinary]", "[fn:ordinary]", ""
				if format != "org" {
					label, definition, indent = "[^ordinary]", "[^ordinary]:", "    "
				}
				original := "Source" + label + ".\n\n" + definition + " Original text.\n"
				if date != "" {
					original += "\n" + indent + "Author: Original author; Created: " + date + "\n"
				}
				if err := os.WriteFile(loc.Path, []byte(original), 0600); err != nil {
					t.Fatal(err)
				}
				snap, err := Read(loc)
				if err != nil {
					t.Fatal(err)
				}
				notes := EditableFootnotes(snap.RawSource, format, "embedded")
				if len(notes) != 1 || notes[0].Text != "Original text." || notes[0].CreatedAt != date {
					t.Fatal(notes)
				}
				r := editRequest(snap, notes[0], "Revised text.")
				writer := NewWriter(FileOperations{})
				writer.now = func() time.Time { return time.Date(2026, 9, 14, 14, 30, 0, 0, time.Local) }
				if _, err := writer.Replace(t.Context(), loc, snap.SourceInfo, "Another reviewer", "secret", r, nil); err != nil {
					t.Fatal(err)
				}
				after, err := Read(loc)
				if err != nil {
					t.Fatal(err)
				}
				expected := strings.Replace(original, "Original text.", "Revised text.", 1)
				if date != "" {
					expected = strings.Replace(expected, "Author: Original author; Created: "+date, "Author: Another reviewer; Created: [2026-09-14 Mon 14:30]", 1)
				}
				if string(after.RawSource) != expected {
					t.Fatalf("native attribution or unrelated bytes changed: %s", after.RawSource)
				}
			})
		}
	}
}

func TestRT009_7_EditOtherNoteRetainsActiveComposer(t *testing.T) {
	loc, _ := sourceFixture(t, "notes.org")
	if err := os.WriteFile(loc.Path, []byte("A sentence[fn:ordinary].\n\n[fn:ordinary] Existing note.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	snap, err := Read(loc)
	if err != nil {
		t.Fatal(err)
	}
	w := NewWriter(FileOperations{})
	r := requestFixture(t, snap)
	r.Label = "new-note"
	r.Text = "Draft one"
	r.Target = Target{Type: "point", Position: 1, Run: "A sentence", RunOffset: 1}
	r.BodyRevision = Digest([]byte("A sentence."))
	first, err := w.Replace(t.Context(), loc, snap.SourceInfo, "Reviewer", "secret", r, func(context.Context, Snapshot, *Target) (int, error) { return 1, nil })
	if err != nil {
		t.Fatal(err)
	}
	snap, err = Read(loc)
	if err != nil {
		t.Fatal(err)
	}
	var ordinary EditableFootnote
	for _, note := range EditableFootnotes(snap.RawSource, "org", "embedded") {
		if note.Label == "ordinary" {
			ordinary = note
		}
	}
	edited, err := w.Replace(t.Context(), loc, first.SourceInfo, "Reviewer", "other", editRequest(snap, ordinary, "An edited ordinary note."), nil)
	if err != nil {
		t.Fatal(err)
	}
	snap, err = Read(loc)
	if err != nil {
		t.Fatal(err)
	}
	r.OperationID = "10000000-0000-4000-8000-000000000009"
	r.Sequence = 2
	r.Text = "Draft two"
	r.Revision, r.SourceRevision = snap.Revision, snap.SourceRevision
	if _, err = w.Replace(t.Context(), loc, edited.SourceInfo, "Reviewer", "secret", r, nil); err != nil {
		t.Fatal("unrelated edit interrupted active draft", err)
	}
	snap, err = Read(loc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(snap.RawSource), "An edited ordinary note.") || !strings.Contains(string(snap.RawSource), "Draft two") {
		t.Fatal("current notes missing")
	}
}

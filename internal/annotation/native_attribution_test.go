// ABOUTME: Checks native footnotes without persisted autosave metadata.
// ABOUTME: Covers current values, optional attribution and service-independent editing.
package annotation

import (
	"context"
	"fmt"
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
					expected = strings.Replace(expected, "Author: Original author; Created: "+date, "Author: Another reviewer; Edited: [2026-09-14 Mon 14:30]", 1)
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

func TestRT009_4_AttributionLifecycle(t *testing.T) {
	for _, format := range []string{"org", "markdown"} {
		for _, changeDraft := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/changed=%v", format, changeDraft), func(t *testing.T) {
				loc, _ := sourceFixture(t, "note")
				loc.Format = format
				snap, err := Read(loc)
				if err != nil {
					t.Fatal(err)
				}
				w := NewWriter(FileOperations{})
				now := time.Date(2026, 9, 14, 14, 30, 0, 0, time.Local)
				w.now = func() time.Time { return now }
				r := requestFixture(t, snap)
				r.Text = "First value"
				result, err := w.Replace(t.Context(), loc, snap.SourceInfo, "Reviewer", "secret", r, func(context.Context, Snapshot, *Target) (int, error) { return 10, nil })
				if err != nil {
					t.Fatal(err)
				}
				check := func(want string) (Snapshot, EditableFootnote) {
					t.Helper()
					current, err := Read(loc)
					if err != nil {
						t.Fatal(err)
					}
					notes := EditableFootnotes(current.RawSource, format, "embedded")
					if len(notes) != 1 || notes[0].attribution != want {
						t.Fatalf("attribution: %#v, want %s", notes, want)
					}
					return current, notes[0]
				}
				want := "Author: Reviewer; Created: [2026-09-14 Mon 14:30]"
				snap, _ = check(want)
				if changeDraft {
					now = now.Add(time.Minute)
					r.Sequence++
					r.OperationID = "10000000-0000-4000-8000-000000000009"
					r.Text = "Changed value"
					r.Revision, r.SourceRevision = snap.Revision, snap.SourceRevision
					result, err = w.Replace(t.Context(), loc, result.SourceInfo, "Reviewer", "secret", r, nil)
					if err != nil {
						t.Fatal(err)
					}
					want = "Author: Reviewer; Edited: [2026-09-14 Mon 14:31]"
					snap, _ = check(want)
				}
				now = now.Add(time.Hour)
				r.Sequence++
				r.OperationID = "10000000-0000-4000-8000-000000000010"
				r.Action = "close"
				r.Revision, r.SourceRevision = snap.Revision, snap.SourceRevision
				result, err = w.Replace(t.Context(), loc, result.SourceInfo, "Reviewer", "secret", r, nil)
				if err != nil {
					t.Fatal(err)
				}
				snap, note := check(want)
				// Reopening and editing again must read Edited without including the attribution in the textarea.
				for i := 0; i < 2; i++ {
					w = NewWriter(FileOperations{})
					now = now.Add(time.Minute)
					w.now = func() time.Time { return now }
					edit := editRequest(snap, note, fmt.Sprintf("Reopened edit %d", i))
					result, err = w.Replace(t.Context(), loc, result.SourceInfo, "Other reviewer", "new", edit, nil)
					if err != nil {
						t.Fatal(err)
					}
					want = "Author: Other reviewer; Edited: " + now.Format("[2006-01-02 Mon 15:04]")
					snap, note = check(want)
					if note.Text != edit.Text {
						t.Fatal("attribution leaked into editor", note.Text)
					}
					before := string(snap.RawSource)
					now = now.Add(time.Hour)
					if _, err = w.Replace(t.Context(), loc, result.SourceInfo, "Other reviewer", "new", edit, nil); err != nil {
						t.Fatal(err)
					}
					snap, note = check(want)
					if string(snap.RawSource) != before {
						t.Fatal("retry changed source")
					}
				}
			})
		}
	}
}

func TestRT009_6_NativeSidecarKeepsAuthoredExample(t *testing.T) {
	loc, _ := sourceFixture(t, "note.org")
	if err := os.WriteFile(loc.Path, []byte("Body.\n"), 0400); err != nil {
		t.Fatal(err)
	}
	body := "\n#+BEGIN_EXAMPLE\n*literal* <text>\n#+END_EXAMPLE"
	sidecar := "Context: Body. | [fn:ordinary]\n\n[fn:ordinary] " + body + "\n\nAuthor: Reviewer; Edited: [2026-09-14 Mon 03:12]\n"
	if err := os.WriteFile(loc.Path+"-annotations.org", []byte(sidecar), 0600); err != nil {
		t.Fatal(err)
	}
	snap, err := Read(loc)
	if err != nil {
		t.Fatal(err)
	}
	preview, _, err := PreviewFootnotes(t.Context(), snap, "org", "Body.", nil, "review")
	if err != nil || !strings.Contains(string(preview), body) {
		t.Fatalf("ordinary sidecar block changed: %v\n%s", err, preview)
	}
	if string(snap.RawSource) != "Body.\n" || string(snap.RawSidecar) != sidecar {
		t.Fatal("reading changed source or sidecar")
	}
}

func TestRT009_4_TrailingNewlinesPreserveAttribution(t *testing.T) {
	for _, format := range []string{"org", "markdown"} {
		for _, sidecar := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/sidecar=%v", format, sidecar), func(t *testing.T) {
				loc, _ := sourceFixture(t, "note")
				loc.Format = format
				if sidecar {
					if os.Geteuid() == 0 {
						t.Skip("requires permission enforcement")
					}
					if err := os.Chmod(loc.Path, 0400); err != nil {
						t.Fatal(err)
					}
				}
				snap, err := Read(loc)
				if err != nil {
					t.Fatal(err)
				}
				original := string(snap.RawSource)
				w := NewWriter(FileOperations{})
				now := time.Date(2026, 9, 14, 14, 30, 0, 0, time.Local)
				w.now = func() time.Time { return now }
				r := requestFixture(t, snap)
				r.Text = "A draft  \n\n"
				result, err := w.Replace(t.Context(), loc, snap.SourceInfo, "Reviewer", "secret", r, func(context.Context, Snapshot, *Target) (int, error) { return 10, nil })
				if err != nil {
					t.Fatal("save with trailing newlines", err)
				}
				readNote := func(want string) (Snapshot, EditableFootnote, string) {
					t.Helper()
					current, err := Read(loc)
					if err != nil {
						t.Fatal(err)
					}
					data, noteFormat, storage := current.RawSource, format, "embedded"
					if sidecar {
						data, noteFormat, storage = current.RawSidecar, "org", "sidecar"
						if string(current.RawSource) != original {
							t.Fatal("read-only original changed")
						}
					}
					notes := EditableFootnotes(data, noteFormat, storage)
					if len(notes) != 1 || notes[0].Text != want {
						t.Fatalf("saved text: %#v, want %q", notes, want)
					}
					return current, notes[0], string(data)
				}
				snap, _, before := readNote("A draft  ")
				if retry, err := w.Replace(t.Context(), loc, result.SourceInfo, "Reviewer", "secret", r, nil); err != nil || !retry.Retry {
					t.Fatal("retry with trailing newlines", err)
				}
				now = now.Add(time.Hour)
				r.Sequence++
				r.OperationID = "10000000-0000-4000-8000-000000000009"
				r.Text = "A draft  \r\n"
				r.Revision, r.SourceRevision = snap.Revision, snap.SourceRevision
				result, err = w.Replace(t.Context(), loc, result.SourceInfo, "Reviewer", "secret", r, nil)
				if err != nil {
					t.Fatal("save differing only in trailing line endings", err)
				}
				snap, note, after := readNote("A draft  ")
				if after != before {
					t.Fatal("trailing-newline change altered source or attribution")
				}
				r.Sequence++
				r.OperationID = "10000000-0000-4000-8000-000000000010"
				r.Action = "close"
				r.Revision, r.SourceRevision = snap.Revision, snap.SourceRevision
				result, err = w.Replace(t.Context(), loc, result.SourceInfo, "Reviewer", "secret", r, nil)
				if err != nil || !result.Receipt.Closed {
					t.Fatal("close with trailing newlines", err)
				}
				snap, note, after = readNote("A draft  ")
				if after != before {
					t.Fatal("close changed source or attribution")
				}
				edit := editRequest(snap, note, "Edited *body*.\n\nParagraph.  \r\n")
				result, err = w.Replace(t.Context(), loc, result.SourceInfo, "Other reviewer", "other", edit, nil)
				if err != nil {
					t.Fatal("edit with trailing newlines", err)
				}
				_, note, before = readNote("Edited *body*.\n\nParagraph.  ")
				if note.attribution != "Author: Other reviewer; Edited: [2026-09-14 Mon 15:30]" {
					t.Fatal("edited attribution", note.attribution)
				}
				now = now.Add(time.Hour)
				if retry, err := w.Replace(t.Context(), loc, result.SourceInfo, "Other reviewer", "other", edit, nil); err != nil || !retry.Retry {
					t.Fatal("edit retry with trailing newlines", err)
				}
				_, _, after = readNote("Edited *body*.\n\nParagraph.  ")
				if after != before {
					t.Fatal("edit retry changed source or attribution")
				}
			})
		}
	}
}

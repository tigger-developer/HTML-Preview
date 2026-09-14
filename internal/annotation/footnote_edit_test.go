// ABOUTME: Verifies native footnote edits against real source files and atomic writes.
// ABOUTME: Covers attribution, native markup, concurrent edits and read-only boundaries.
package annotation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func editRequest(s Snapshot, n EditableFootnote, text string) Request {
	return Request{OperationID: "10000000-0000-4000-8000-000000000009", AnnotationID: "20000000-0000-4000-8000-000000000009", ComposerID: "30000000-0000-4000-8000-000000000009", Sequence: 1, Revision: s.Revision, SourceRevision: s.SourceRevision, BodyRevision: Digest([]byte("body")), Action: "edit", Label: n.Label, Target: Target{Type: "footnote", Exact: n.Revision, Run: n.Storage}, Text: text}
}

func TestRT009_7_NativeFootnoteMarkup(t *testing.T) {
	cases := []struct{ name, format, source, text string }{
		{"org paragraphs", "org", "Body[fn:one].\n\n[fn:one] /First/ paragraph.\n\nSecond paragraph.\n\n\nOutside.\n", "/First/ paragraph.\n\nSecond paragraph."},
		{"markdown CRLF", "markdown", "Body[^1].\r\n\r\n[^1]: **First** paragraph.\r\n\r\n    Second paragraph.\r\n\r\nOutside.\r\n", "**First** paragraph.\n\nSecond paragraph."},
		{"org literal block", "org", "Body[fn:one].\n\n[fn:one]\n#+BEGIN_EXAMPLE\nFirst <text>.\n#+END_EXAMPLE\n", "\n#+BEGIN_EXAMPLE\nFirst <text>.\n#+END_EXAMPLE"},
		{"markdown literal block", "markdown", "Body[^one].\n\n[^one]:\n    ```\n    First <text>.\n    ```\n", "\n```\nFirst <text>.\n```"},
		{"unicode ID", "org", "Body[fn:réf].\n\n[fn:réf] First note.", "First note."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "note")
			if err := os.WriteFile(path, []byte(tc.source), 0600); err != nil {
				t.Fatal(err)
			}
			loc := Location{Root: root, Path: path, Format: tc.format}
			snap, err := Read(loc)
			if err != nil {
				t.Fatal(err)
			}
			notes := EditableFootnotes(snap.RawSource, tc.format, "embedded")
			if len(notes) != 1 || notes[0].Text != tc.text {
				t.Fatalf("native definition changed: %#v", notes)
			}
			request := editRequest(snap, notes[0], strings.Replace(tc.text, "First", "Updated", 1))
			result, err := NewWriter(FileOperations{}).Replace(context.Background(), loc, snap.SourceInfo, "Reviewer", "secret", request, nil)
			if err != nil {
				t.Fatal(err)
			}
			current, err := Read(loc)
			if err != nil {
				t.Fatal(err)
			}
			if string(current.RawSource) != strings.Replace(tc.source, "First", "Updated", 1) || result.Receipt.FootnoteRevision == notes[0].Revision {
				t.Fatalf("unrelated bytes or digest changed incorrectly: %s", current.RawSource)
			}
		})
	}
}

func TestRT009_7_EditClosedRefreshesAttribution(t *testing.T) {
	for _, format := range []string{"org", "markdown"} {
		t.Run(format, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "note")
			header := Header{1, "40000000-0000-4000-8000-000000000009", format}
			event := Event{Schema: 2, AnnotationID: "20000000-0000-4000-8000-000000000001", OperationID: "10000000-0000-4000-8000-000000000001", Author: "Original author", CreatedAt: "2026-09-14T10:00:00Z", RecordedAt: "2026-09-14T10:00:01Z", Sequence: 2, Kind: "close", Text: "First comment", Target: Target{Type: "point"}}
			data, err := UpdateFootnote([]byte("Body.\n"), format, header, event, "original-001", 4)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			loc := Location{Root: root, Path: path, Format: format}
			snap, err := Read(loc)
			if err != nil {
				t.Fatal(err)
			}
			note := EditableFootnotes(data, format, "embedded")[0]
			request := editRequest(snap, note, "Revised comment")
			writer := NewWriter(FileOperations{})
			writer.now = func() time.Time { return time.Date(2026, 9, 14, 14, 30, 0, 0, time.Local) }
			_, err = writer.Replace(context.Background(), loc, snap.SourceInfo, "Different reviewer", "secret", request, nil)
			if err != nil {
				t.Fatal(err)
			}
			current, err := Read(loc)
			if err != nil || current.Reason != "" || len(EditableFootnotes(current.RawSource, format, "embedded")) != 1 {
				t.Fatalf("invalid edit: %v %#v", err, current.Events)
			}
			got := EditableFootnotes(current.RawSource, format, "embedded")[0]
			if got.Author != "Different reviewer" || got.CreatedAt != "[2026-09-14 Mon 14:30]" || got.Label != "original-001" || strings.Contains(string(current.RawSource), nativeMarker) || got.Text != "Revised comment" {
				t.Fatal("attribution or state changed", got)
			}
			// A new writer has no browser/session memory but can reopen the persisted note.
			note = EditableFootnotes(current.RawSource, format, "embedded")[0]
			request = editRequest(current, note, "Later correction")
			if _, err = NewWriter(FileOperations{}).Replace(context.Background(), loc, current.SourceInfo, "Reviewer", "another", request, nil); err != nil {
				t.Fatal("reopen after restart", err)
			}
		})
	}
}

func TestRT009_7_EditRefusals(t *testing.T) {
	for _, tc := range []string{"stale note", "stale document", "duplicate label", "read only", "breakout", "failed write"} {
		t.Run(tc, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "note.org")
			original := "Body[fn:one].\n\n[fn:one] First.\n"
			if err := os.WriteFile(path, []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
			loc := Location{Root: root, Path: path, Format: "org"}
			snap, err := Read(loc)
			if err != nil {
				t.Fatal(err)
			}
			note := EditableFootnotes(snap.RawSource, "org", "embedded")[0]
			r := editRequest(snap, note, "Replacement.")
			w := NewWriter(FileOperations{})
			want := "footnote_conflict"
			switch tc {
			case "stale note":
				original = strings.Replace(original, "First.", "External change.", 1)
			case "stale document":
				original = "Added outside.\n" + original
				want = "stale_source"
			case "duplicate label":
				original += "\n[fn:one] Duplicate.\n"
			case "read only":
				if os.Geteuid() == 0 {
					t.Skip("requires permission enforcement")
				}
				if err := os.Chmod(path, 0400); err != nil {
					t.Fatal(err)
				}
				want = "read_only_footnote"
			case "breakout":
				r.Text = "Paragraph\n\n\n* New heading"
				want = "invalid_footnote"
			case "failed write":
				w = NewWriter(FileOperations{Append: func(*os.File, []byte) (int, error) { return 0, errors.New("test failure") }})
				want = "test failure"
			}
			if tc == "stale note" || tc == "stale document" || tc == "duplicate label" {
				if err := os.WriteFile(path, []byte(original), 0600); err != nil {
					t.Fatal(err)
				}
			}
			_, err = w.Replace(context.Background(), loc, snap.SourceInfo, "Reviewer", "secret", r, nil)
			if err == nil || err.Error() != want {
				t.Fatalf("refusal=%v want %s", err, want)
			}
			current, err := Read(loc)
			if err != nil || string(current.RawSource) != original {
				t.Fatal("refused edit changed source", err)
			}
		})
	}
}

func TestRT009_6_EditExistingSidecar(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires permission enforcement")
	}
	root := t.TempDir()
	path := filepath.Join(root, "note.org")
	original := []byte("Body.\n")
	if err := os.WriteFile(path, original, 0400); err != nil {
		t.Fatal(err)
	}
	loc := Location{Root: root, Path: path, Format: "org"}
	snap, err := Read(loc)
	if err != nil {
		t.Fatal(err)
	}
	w := NewWriter(FileOperations{})
	request := Request{OperationID: "10000000-0000-4000-8000-000000000001", AnnotationID: "20000000-0000-4000-8000-000000000001", ComposerID: "30000000-0000-4000-8000-000000000001", Sequence: 1, Revision: snap.Revision, SourceRevision: snap.SourceRevision, BodyRevision: Digest([]byte("Body.")), Action: "upsert", Label: "reviewer-001", Target: Target{Type: "point", Position: 4, Run: "Body.", RunOffset: 4}, Text: "First comment"}
	_, err = w.Replace(context.Background(), loc, snap.SourceInfo, "Original author", "secret", request, func(context.Context, Snapshot, *Target) (int, error) { return 4, nil })
	if err != nil {
		t.Fatal(err)
	}
	snap, err = Read(loc)
	if err != nil {
		t.Fatal(err)
	}
	note := EditableFootnotes(snap.RawSidecar, "org", "sidecar")[0]
	_, err = w.Replace(context.Background(), loc, snap.SourceInfo, "Reviewer", "other", editRequest(snap, note, "Edited sidecar"), nil)
	if err != nil {
		t.Fatal(err)
	}
	current, err := Read(loc)
	if err != nil || string(current.RawSource) != string(original) || len(EditableFootnotes(current.RawSidecar, "org", "sidecar")) != 1 || EditableFootnotes(current.RawSidecar, "org", "sidecar")[0].Text != "Edited sidecar" || EditableFootnotes(current.RawSidecar, "org", "sidecar")[0].Author != "Reviewer" {
		t.Fatal("sidecar edit changed source or attribution", err)
	}
}
